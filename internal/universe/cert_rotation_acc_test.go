// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package universe_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// The rotation test asserts two things per step: the universe's certificate
// state (rootCA, clientRootCA, rootAndClientRootCASame) as YBA reports it,
// and the exact cumulative number of successful CertsRotate tasks recorded
// for the universe. The count catches both silent no-ops (a rotation the
// provider decided not to dispatch) and redundant double-dispatches (an
// extra rolling restart the user never asked for) — apply succeeding is not
// evidence either way.
//
// Named *Long so the short tier's `-skip '^TestAccLong'` skips it; it deploys
// a real 1-node universe and every rotation is a rolling restart. All
// scenarios chain on that one universe: rotations exercise universe
// properties, not universe shape, so paying a universe create per scenario
// buys nothing. The universe shrinks the node_restart_settings sleeps to
// 30 s — nothing runs on it, and the platform-default 3 minutes per restart
// would more than double each rotation.

// TestAccLong_Universe_GCP_CertRotation walks one universe through the
// certificate lifecycle:
//
//  1. create with root_ca only and a cert_rotation trigger — YBA shares the
//     CA across both TLS channels, and a trigger set at creation records
//     without rotating;
//  2. change root_ca — BOTH channels must move to the new CA (the channel
//     split regression from review: the state echo of clientRootCA must not
//     pin client-to-node to the old certificate);
//  3. remove the cert_rotation block — clearing a trigger must never fire;
//  4. re-add the block with a new value — adding a trigger to an
//     already-managed universe counts as a change and fires a same-CA
//     server-certificate rotation;
//  5. replace the root certificate via create_before_destroy (the documented
//     SelfSigned rotation pattern) — new config minted first, universe
//     rotated to it, old configuration deleted once out of use;
//  6. split the channels: point client_root_ca at an org-issued
//     yba_custom_server_certificate (fed in with CRLF endings and 76-column
//     base64 — the post-apply empty-plan check proves the semantic PEM
//     comparison against YBA's re-encoded read-back);
//  7. re-issue the server certificate from the same org CA (new configuration
//     with identical root content) and repoint client_root_ca — the
//     documented CustomServerCert rotation flow. The old configuration stays
//     in config until the rotation lands: Terraform destroys removed
//     resources before updating their former referrers, so dropping it in
//     the same apply trips the in-use guard;
//  8. drop the now-unused old configuration and change root_ca while
//     client_root_ca is explicitly pinned — explicit config wins: only the
//     node-to-node side moves and the split is preserved;
//  9. taint the in-use client certificate — the delete guard must fail the
//     replacement with an error naming the referencing universe instead of
//     corrupting state.
func TestAccLong_Universe_GCP_CertRotation(t *testing.T) {
	var universe client.UniverseResp
	var oldCertBUUID, certOneUUID string

	rName := acctest.RandomName("cert-uni")
	certA := mintedCertConfig("a", rName+"-ca-a")
	certB := mintedCertConfig("b", rName+"-ca-b")
	certB2 := mintedCertConfig("b", rName+"-ca-b2")

	orgCA := acctest.NewTestCA(t, "tf-acc-c2n-root")
	serverOne, keyOne := orgCA.IssueServerCert(t, "one.acctest.local")
	serverTwo, keyTwo := orgCA.IssueServerCert(t, "two.acctest.local")
	certOne := customCertConfig("one", rName+"-c2n-1",
		acctest.MangledPEM(t, orgCA.CertPEM), serverOne, keyOne)
	certTwo := customCertConfig("two", rName+"-c2n-2", orgCA.CertPEM, serverTwo, keyTwo)

	// The trigger stays at this value from step 4 on so later applies never
	// re-fire it.
	trigger := `
					cert_rotation {
						server_cert_trigger = "epoch-2"
					}`
	splitAttrs := func(rootRes, clientRes string) string {
		return fmt.Sprintf(`
					root_ca        = yba_self_signed_certificate.%s.uuid
					client_root_ca = yba_custom_server_certificate.%s.uuid
`, rootRes, clientRes) + trigger
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckGCP(t)
			acctest.TestAccPreCheckCloudYBA(t, "GCP")
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyCertRotationFixtures,
		Steps: []resource.TestStep{
			{
				// 1: shared CA at create; trigger set at create records only.
				Config: certRotationUniverseConfig(rName, certA+certB, `
					root_ca = yba_self_signed_certificate.a.uuid

					cert_rotation {
						server_cert_trigger = "epoch-1"
					}`),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckUniverseCertState(&universe,
						"yba_self_signed_certificate.a",
						"yba_self_signed_certificate.a", true),
					testAccCheckCertsRotateTaskCount(&universe, 0),
					testAccCheckCertificateInUseBy(
						"yba_self_signed_certificate.a", rName),
					testAccCaptureResourceID("yba_self_signed_certificate.b",
						&oldCertBUUID),
				),
			},
			{
				// 2: root_ca change on a shared-CA universe moves BOTH channels.
				Config: certRotationUniverseConfig(rName, certA+certB, `
					root_ca = yba_self_signed_certificate.b.uuid

					cert_rotation {
						server_cert_trigger = "epoch-1"
					}`),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckUniverseCertState(&universe,
						"yba_self_signed_certificate.b",
						"yba_self_signed_certificate.b", true),
					testAccCheckCertsRotateTaskCount(&universe, 1),
				),
			},
			{
				// 3: removing the cert_rotation block never fires.
				Config: certRotationUniverseConfig(rName, certA+certB,
					`root_ca = yba_self_signed_certificate.b.uuid`),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckCertsRotateTaskCount(&universe, 1),
				),
			},
			{
				// 4: adding a trigger to a managed universe fires a rotation.
				Config: certRotationUniverseConfig(rName, certA+certB,
					`root_ca = yba_self_signed_certificate.b.uuid`+trigger),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckUniverseCertState(&universe,
						"yba_self_signed_certificate.b",
						"yba_self_signed_certificate.b", true),
					testAccCheckCertsRotateTaskCount(&universe, 2),
				),
			},
			{
				// 5: create_before_destroy replacement — new label mints a new
				// config, the universe rotates to it, the old one is deleted.
				Config: certRotationUniverseConfig(rName, certA+certB2,
					`root_ca = yba_self_signed_certificate.b.uuid`+trigger),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckUniverseCertState(&universe,
						"yba_self_signed_certificate.b",
						"yba_self_signed_certificate.b", true),
					testAccCheckCertsRotateTaskCount(&universe, 3),
					testAccCheckResourceIDChanged("yba_self_signed_certificate.b",
						&oldCertBUUID),
					testAccCheckCertificateGone(&oldCertBUUID),
				),
			},
			{
				// 6: split the channels onto an org-issued CustomServerCert.
				Config: certRotationUniverseConfig(rName, certA+certB2+certOne,
					splitAttrs("b", "one")),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckUniverseCertState(&universe,
						"yba_self_signed_certificate.b",
						"yba_custom_server_certificate.one", false),
					testAccCheckCertsRotateTaskCount(&universe, 4),
					testAccCheckCertificateInUseBy(
						"yba_custom_server_certificate.one", rName),
					testAccCaptureResourceID("yba_custom_server_certificate.one",
						&certOneUUID),
				),
			},
			{
				// 7: repoint to a re-issued server certificate (same org CA).
				// The old configuration stays until the rotation lands.
				Config: certRotationUniverseConfig(rName, certA+certB2+certOne+certTwo,
					splitAttrs("b", "two")),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckUniverseCertState(&universe,
						"yba_self_signed_certificate.b",
						"yba_custom_server_certificate.two", false),
					testAccCheckCertsRotateTaskCount(&universe, 5),
				),
			},
			{
				// 8: drop the unused old configuration; rotate root_ca with the
				// client explicitly pinned — the split must be preserved.
				Config: certRotationUniverseConfig(rName, certA+certB2+certTwo,
					splitAttrs("a", "two")),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckUniverseCertState(&universe,
						"yba_self_signed_certificate.a",
						"yba_custom_server_certificate.two", false),
					testAccCheckCertsRotateTaskCount(&universe, 6),
					testAccCheckCertificateGone(&certOneUUID),
				),
			},
			{
				// 9: deleting an in-use certificate must fail with an error
				// naming the referencing universe.
				Config: certRotationUniverseConfig(rName, certA+certB2+certTwo,
					splitAttrs("a", "two")),
				Taint:       []string{"yba_custom_server_certificate.two"},
				ExpectError: regexp.MustCompile("still referenced by universe"),
			},
		},
	})
}

// certRotationUniverseConfig assembles provider + certificates + a minimal
// 1-node universe. universeCertAttrs carries the per-step root_ca /
// client_root_ca / cert_rotation lines.
func certRotationUniverseConfig(name, certResources, universeCertAttrs string) string {
	return acctest.YBAProviderBlock("GCP") + cloudProviderGCPConfig(name+"-provider") +
		certResources + fmt.Sprintf(`
	data "yba_provider_key" "gcp_key" {
		provider_id = yba_cloud_provider.gcp.id
	}

	data "yba_release_version" "release_version" {
		depends_on = [
			data.yba_provider_key.gcp_key
		]
	}

	resource "yba_universe" "gcp" {
		%s

		node_restart_settings {
			sleep_after_master_restart_millis  = 30000
			sleep_after_tserver_restart_millis = 30000
		}

		clusters {
			cluster_type = "PRIMARY"
			user_intent {
				universe_name      = "%s"
				provider           = yba_cloud_provider.gcp.id
				region_list        = yba_cloud_provider.gcp.regions[*].uuid
				num_nodes          = 1
				replication_factor = 1
				instance_type      = "%s"
				device_info {
					num_volumes  = 1
					volume_size  = 375
					storage_type = "%s"
				}
				assign_public_ip              = true
				use_time_sync                 = true
				enable_ysql                   = true
				enable_node_to_node_encrypt   = true
				enable_client_to_node_encrypt = true
				yb_software_version           = data.yba_release_version.release_version.id
				access_key_code               = data.yba_provider_key.gcp_key.id
				instance_tags = {
					"yb_owner" = "terraform_acctest"
					"yb_task"  = "dev"
					"yb_dept"  = "dev"
				}
			}
		}
		communication_ports {}
	}
`, universeCertAttrs, name, getUniverseInstanceType("gcp"), getUniverseStorageType("gcp"))
}

// mintedCertConfig returns a mint-mode SelfSigned certificate resource.
// create_before_destroy is the documented lifecycle for certificates
// referenced by universes: replacements are minted (under a new label)
// before the old configuration is deleted.
func mintedCertConfig(res, label string) string {
	return fmt.Sprintf(`
	resource "yba_self_signed_certificate" "%s" {
		label = "%s"

		lifecycle {
			create_before_destroy = true
		}
	}
`, res, label)
}

// customCertConfig returns a CustomServerCert resource. No
// create_before_destroy: the rotation flow for this type is a new resource
// plus repointing client_root_ca, and the taint step relies on the default
// delete-first ordering to exercise the in-use guard.
func customCertConfig(res, label, rootPEM, certPEM, keyPEM string) string {
	return fmt.Sprintf(`
	resource "yba_custom_server_certificate" "%s" {
		label              = "%s"
		root_certificate   = %s
		server_certificate = %s
		server_key         = %s
	}
`, res, label, strconv.Quote(rootPEM), strconv.Quote(certPEM), strconv.Quote(keyPEM))
}

// testAccCheckUniverseCertState asserts YBA's certificate state for the
// universe: rootCA and clientRootCA match the given certificate resources
// and rootAndClientRootCASame has the expected value. Run
// testAccCheckUniverseExists first to populate universe.
func testAccCheckUniverseCertState(universe *client.UniverseResp,
	rootRes, clientRes string, wantSame bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		wantRoot, err := resourceIDFromState(s, rootRes)
		if err != nil {
			return err
		}
		wantClient, err := resourceIDFromState(s, clientRes)
		if err != nil {
			return err
		}
		details := universe.UniverseDetails
		if details.GetRootCA() != wantRoot {
			return fmt.Errorf("universe rootCA = %q, want %q (%s)",
				details.GetRootCA(), wantRoot, rootRes)
		}
		if details.GetClientRootCA() != wantClient {
			return fmt.Errorf("universe clientRootCA = %q, want %q (%s)",
				details.GetClientRootCA(), wantClient, clientRes)
		}
		if details.GetRootAndClientRootCASame() != wantSame {
			return fmt.Errorf("universe rootAndClientRootCASame = %t, want %t",
				details.GetRootAndClientRootCASame(), wantSame)
		}
		return nil
	}
}

// testAccCheckCertsRotateTaskCount asserts the universe has exactly want
// successful CertsRotate customer tasks. Run testAccCheckUniverseExists
// first to populate universe.
func testAccCheckCertsRotateTaskCount(universe *client.UniverseResp,
	want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		apiClient, err := acctest.APIClientForCloud("GCP")
		if err != nil {
			return err
		}
		tasks, response, err := apiClient.YugawareClient.CustomerTasksAPI.
			TasksList(context.Background(), apiClient.CustomerID).
			UUUID(universe.GetUniverseUUID()).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Universe", "Read - Tasks")
		}
		count := 0
		for i := range tasks {
			if tasks[i].GetType() != "CertsRotate" {
				continue
			}
			if tasks[i].GetStatus() != "Success" {
				return fmt.Errorf("CertsRotate task %s is %q, want Success",
					tasks[i].GetId(), tasks[i].GetStatus())
			}
			count++
		}
		if count != want {
			return fmt.Errorf(
				"universe has %d successful CertsRotate tasks, want exactly %d",
				count, want)
		}
		return nil
	}
}

// testAccCheckCertificateInUseBy asserts YBA reports the certificate as in
// use by the named universe — the data the provider's delete guard depends on.
func testAccCheckCertificateInUseBy(certRes, universeName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		certUUID, err := resourceIDFromState(s, certRes)
		if err != nil {
			return err
		}
		cert, err := findCertificateByUUID(certUUID)
		if err != nil {
			return err
		}
		if cert == nil {
			return fmt.Errorf("certificate %s (%s) not found in YBA", certRes, certUUID)
		}
		if !cert.GetInUse() {
			return fmt.Errorf("certificate %s must be reported in use", certRes)
		}
		for _, u := range cert.GetUniverseDetails() {
			if u.Name == universeName {
				return nil
			}
		}
		return fmt.Errorf("certificate %s universeDetails does not name universe %q",
			certRes, universeName)
	}
}

// testAccCaptureResourceID stores the resource's current ID for comparison
// in a later step.
func testAccCaptureResourceID(name string, dst *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := resourceIDFromState(s, name)
		if err != nil {
			return err
		}
		*dst = id
		return nil
	}
}

// testAccCheckResourceIDChanged asserts the resource was replaced since its
// ID was captured.
func testAccCheckResourceIDChanged(name string, old *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := resourceIDFromState(s, name)
		if err != nil {
			return err
		}
		if *old == "" {
			return fmt.Errorf("no previous ID captured for %s", name)
		}
		if id == *old {
			return fmt.Errorf("%s still has ID %s, expected a replacement", name, id)
		}
		return nil
	}
}

// testAccCheckCertificateGone asserts YBA no longer has the certificate with
// the captured UUID.
func testAccCheckCertificateGone(certUUID *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if *certUUID == "" {
			return errors.New("no certificate UUID captured")
		}
		cert, err := findCertificateByUUID(*certUUID)
		if err != nil {
			return err
		}
		if cert != nil {
			return fmt.Errorf("certificate %s (%s) still exists in YBA",
				cert.GetLabel(), *certUUID)
		}
		return nil
	}
}

// testAccCheckDestroyCertRotationFixtures extends the shared universe/provider
// destroy check with the certificate resources these tests create.
func testAccCheckDestroyCertRotationFixtures(s *terraform.State) error {
	if err := testAccCheckDestroyProviderAndUniverse("GCP")(s); err != nil {
		return err
	}
	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_self_signed_certificate" &&
			r.Type != "yba_custom_server_certificate" {
			continue
		}
		cert, err := findCertificateByUUID(r.Primary.ID)
		if err != nil {
			return err
		}
		if cert != nil {
			return fmt.Errorf("certificate %s (%s) is not destroyed",
				cert.GetLabel(), r.Primary.ID)
		}
	}
	return nil
}

func resourceIDFromState(s *terraform.State, name string) (string, error) {
	r, ok := s.RootModule().Resources[name]
	if !ok {
		return "", fmt.Errorf("resource not found in state: %s", name)
	}
	if r.Primary.ID == "" {
		return "", fmt.Errorf("no ID set for %s", name)
	}
	return r.Primary.ID, nil
}

// findCertificateByUUID fetches the certificate from the GCP fixture YBA, or
// nil when YBA no longer has it. YBA has no public by-UUID GET, so this
// filters the list endpoint.
func findCertificateByUUID(certUUID string) (*client.CertificateInfoExt, error) {
	apiClient, err := acctest.APIClientForCloud("GCP")
	if err != nil {
		return nil, err
	}
	certs, response, err := apiClient.YugawareClient.CertificateInfoAPI.
		GetListOfCertificate(context.Background(), apiClient.CustomerID).Execute()
	if err != nil {
		return nil, utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
			"Certificate", "Read - List")
	}
	for i := range certs {
		if certs[i].GetUuid() == certUUID {
			return &certs[i], nil
		}
	}
	return nil, nil
}
