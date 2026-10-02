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

// Each step asserts the universe's certificate state as YBA reports it
// (rootCA, clientRootCA, rootAndClientRootCASame) and the exact cumulative
// count of successful CertsRotate tasks: a passing apply proves nothing about
// a rotation silently not dispatched, or dispatched twice (an extra rolling
// restart).
//
// *Long: deploys a real 1-node universe and every rotation is a rolling
// restart. All scenarios chain on that one universe (rotations exercise
// universe properties, not shape). node_restart_settings sleeps are cut to
// 30 s: nothing runs on it, and the 3-minute platform default would more
// than double each rotation.

// TestAccLong_Universe_GCP_CertRotation walks one universe through the
// certificate lifecycle; the step comments carry each scenario.
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

	// Constant from step 4 on so later applies never re-fire it.
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
				// 1: shared CA at create; a trigger set at create records without rotating.
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
				// 2: root_ca change on a shared-CA universe moves BOTH channels: the
				// state echo of clientRootCA must not pin client-to-node to the old CA.
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
				// 4: adding a trigger to a managed universe fires a same-CA server-cert
				// rotation.
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
				// 5: create_before_destroy replacement (the documented SelfSigned
				// rotation): new label mints a new config, the universe rotates to
				// it, the old one is deleted once out of use.
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
				// 6: split the channels onto an org-issued CustomServerCert. certOne
				// carries CRLF/76-column PEM: the post-apply empty-plan check proves
				// the semantic PEM diff against YBA's re-encoded read-back.
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
				// 7: repoint to a re-issued server cert from the same org CA (the
				// documented CustomServerCert rotation). certOne stays in config:
				// Terraform destroys removed resources before updating their
				// referrers, so dropping it here trips the in-use guard.
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
				// 8: drop the unused certOne; change root_ca with client_root_ca
				// pinned: explicit config wins, only node-to-node moves.
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
				// 9: tainting the in-use client cert must fail with an error naming
				// the referencing universe, not corrupt state.
				Config: certRotationUniverseConfig(rName, certA+certB2+certTwo,
					splitAttrs("a", "two")),
				Taint:       []string{"yba_custom_server_certificate.two"},
				ExpectError: regexp.MustCompile("still referenced by universe"),
			},
		},
	})
}

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

// create_before_destroy is the documented lifecycle for certificates
// referenced by universes: the replacement is minted under a new label before
// the old configuration is deleted.
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

// No create_before_destroy: this type rotates by a new resource plus
// repointing client_root_ca, and step 9 needs delete-first ordering to hit
// the in-use guard.
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

// Run testAccCheckUniverseExists first to populate universe.
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

// Run testAccCheckUniverseExists first to populate universe.
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

// Asserts the inUse/universeDetails data the delete guard depends on.
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

// findCertificateByUUID returns nil when YBA no longer has the certificate.
// YBA has no public by-UUID GET, so it filters the list.
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
