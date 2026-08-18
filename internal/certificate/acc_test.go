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

package certificate_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// The certificate resources need a live YBA but no universe or cloud
// resources, so these run in the short acceptance tier against the GCP
// fixture YBA (same pattern as the storage-config tests). The write-only
// key arguments (private_key, server_key) additionally require the test
// runner's terraform binary to be 1.11 or later.

func testAccPreCheckCertificate(t *testing.T) {
	acctest.TestAccPreCheckCloudYBA(t, "GCP")
}

// TestAccSelfSignedCertificate_MintAndImport covers mint mode round-tripping:
// YBA generates the root certificate, the resource exports it, and a
// subsequent import reproduces the state exactly.
func TestAccSelfSignedCertificate_MintAndImport(t *testing.T) {
	label := acctest.RandomName("cert-mint")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckCertificate(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckCertificatesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: selfSignedMintConfig("mint", label),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"yba_self_signed_certificate.mint", "uuid"),
					resource.TestCheckResourceAttrSet(
						"yba_self_signed_certificate.mint", "certificate"),
					resource.TestCheckResourceAttrSet(
						"yba_self_signed_certificate.mint", "expiry_date"),
					resource.TestCheckResourceAttr(
						"yba_self_signed_certificate.mint", "label", label),
					resource.TestCheckResourceAttr(
						"yba_self_signed_certificate.mint", "in_use", "false"),
				),
			},
			{
				ResourceName:      "yba_self_signed_certificate.mint",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccSelfSignedCertificate_BringYourOwnPEMStability uploads a
// bring-your-own root certificate whose PEM is deliberately mangled (CRLF
// endings, 76-column base64). YBA re-encodes uploads through its own writer,
// so the read-back never matches the config textually; the framework's
// post-apply empty-plan check proves the semantic PEM comparison suppresses
// the would-be destroy-and-recreate diff. The import step then verifies the
// adopted state matches, with the write-only private_key excluded.
func TestAccSelfSignedCertificate_BringYourOwnPEMStability(t *testing.T) {
	label := acctest.RandomName("cert-byo")
	ca := acctest.NewTestCA(t, "tf-acc-byo-root")
	mangled := acctest.MangledPEM(t, ca.CertPEM)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckCertificate(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckCertificatesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: selfSignedByoConfig("byo", label, mangled, ca.KeyPEM),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"yba_self_signed_certificate.byo", "uuid"),
					resource.TestCheckResourceAttr(
						"yba_self_signed_certificate.byo", "label", label),
				),
			},
			{
				ResourceName:            "yba_self_signed_certificate.byo",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"private_key"},
			},
		},
	})
}

// TestAccCustomServerCertificate_ImportAdoption exercises the documented
// adoption story for certificates created outside Terraform: the API never
// returns server_certificate, so the imported state carries it empty, the
// documented `ignore_changes = [server_certificate]` escape hatch yields a
// clean plan, and managing the resource without it replaces the
// configuration (delete + re-upload) as the docs warn.
func TestAccCustomServerCertificate_ImportAdoption(t *testing.T) {
	label := acctest.RandomName("cert-adopt")
	ca := acctest.NewTestCA(t, "tf-acc-adopt-root")
	serverCert, serverKey := ca.IssueServerCert(t, "node.acctest.local")

	var importedUUID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckCertificate(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckCertificatesDestroyed,
		Steps: []resource.TestStep{
			{
				// Adopt a certificate that was uploaded outside Terraform.
				PreConfig: func() {
					importedUUID = uploadCustomServerCert(t, label, ca.CertPEM,
						serverCert, serverKey)
				},
				Config: customServerCertConfig("adopted", label, ca.CertPEM, serverCert,
					serverKey, false),
				ResourceName: "yba_custom_server_certificate.adopted",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return importedUUID, nil
				},
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported instance, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["server_certificate"] != "" {
						return errors.New("server_certificate must be empty after " +
							"import: the API never returns it")
					}
					if attrs["root_certificate"] == "" {
						return errors.New("root_certificate must be restored on import")
					}
					if attrs["label"] != label {
						return fmt.Errorf("imported label = %q, want %q",
							attrs["label"], label)
					}
					return nil
				},
			},
			{
				// The documented escape hatch: ignoring server_certificate
				// adopts the imported certificate as-is with a clean plan.
				Config: customServerCertConfig("adopted", label, ca.CertPEM, serverCert,
					serverKey, true),
				PlanOnly: true,
			},
			{
				// Without the escape hatch the lost server_certificate plans a
				// replacement; applying it deletes the imported configuration
				// and uploads a fresh one.
				Config: customServerCertConfig("adopted", label, ca.CertPEM, serverCert,
					serverKey, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"yba_custom_server_certificate.adopted", "uuid"),
					resource.TestCheckResourceAttr(
						"yba_custom_server_certificate.adopted", "label", label),
					func(s *terraform.State) error {
						r := s.RootModule().Resources["yba_custom_server_certificate.adopted"]
						if r == nil {
							return errors.New("yba_custom_server_certificate.adopted not in state")
						}
						if r.Primary.ID == importedUUID {
							return errors.New("apply without ignore_changes must replace " +
								"the imported certificate configuration")
						}
						return nil
					},
				),
			},
		},
	})
}

func selfSignedMintConfig(res, label string) string {
	return acctest.YBAProviderBlock("GCP") + fmt.Sprintf(`
resource "yba_self_signed_certificate" "%s" {
  label = "%s"
}
`, res, label)
}

func selfSignedByoConfig(res, label, certPEM, keyPEM string) string {
	return acctest.YBAProviderBlock("GCP") + fmt.Sprintf(`
resource "yba_self_signed_certificate" "%s" {
  label       = "%s"
  certificate = %s
  private_key = %s
}
`, res, label, strconv.Quote(certPEM), strconv.Quote(keyPEM))
}

func customServerCertConfig(res, label, rootPEM, certPEM, keyPEM string,
	ignoreServerCert bool) string {
	lifecycle := ""
	if ignoreServerCert {
		lifecycle = `
  lifecycle {
    ignore_changes = [server_certificate]
  }`
	}
	return acctest.YBAProviderBlock("GCP") + fmt.Sprintf(`
resource "yba_custom_server_certificate" "%s" {
  label              = "%s"
  root_certificate   = %s
  server_certificate = %s
  server_key         = %s%s
}
`, res, label, strconv.Quote(rootPEM), strconv.Quote(certPEM), strconv.Quote(keyPEM),
		lifecycle)
}

// uploadCustomServerCert uploads a CustomServerCert configuration directly
// through the API — simulating a certificate created in the YBA UI — and
// returns its UUID for the import step.
func uploadCustomServerCert(t *testing.T, label, rootPEM, serverCert, serverKey string) string {
	t.Helper()
	apiClient, err := acctest.APIClientForCloud("GCP")
	if err != nil {
		t.Fatal(err)
	}
	params := client.CertificateParams{
		Label:       label,
		CertType:    "CustomServerCert",
		CertContent: rootPEM,
		CustomServerCertData: &client.CustomServerCertData{
			ServerCertContent: serverCert,
			ServerKeyContent:  serverKey,
		},
	}
	r, _, err := apiClient.YugawareClient.CertificateInfoAPI.
		Upload(context.Background(), apiClient.CustomerID).Certificate(params).Execute()
	if err != nil {
		t.Fatalf("out-of-band certificate upload failed: %v", err)
	}
	// YBA returns the UUID as a bare JSON string; the generated client keeps
	// the surrounding quotes.
	return strings.Trim(strings.TrimSpace(r), `"`)
}

// testAccCheckCertificatesDestroyed fails when any certificate resource left
// in state still exists in YBA.
func testAccCheckCertificatesDestroyed(s *terraform.State) error {
	apiClient, err := acctest.APIClientForCloud("GCP")
	if err != nil {
		return err
	}
	certs, _, err := apiClient.YugawareClient.CertificateInfoAPI.
		GetListOfCertificate(context.Background(), apiClient.CustomerID).Execute()
	if err != nil {
		return fmt.Errorf("listing certificates: %w", err)
	}
	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_self_signed_certificate" &&
			r.Type != "yba_custom_server_certificate" {
			continue
		}
		for i := range certs {
			if certs[i].GetUuid() == r.Primary.ID {
				return fmt.Errorf("certificate %s (%s) is not destroyed",
					certs[i].GetLabel(), r.Primary.ID)
			}
		}
	}
	return nil
}
