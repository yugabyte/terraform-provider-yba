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
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// Short tier: needs a live YBA (the GCP fixture) but no universe or cloud
// resources. The write-only key arguments need terraform 1.11+ on the runner.

func testAccPreCheckCertificate(t *testing.T) {
	acctest.TestAccPreCheckCloudYBA(t, "GCP")
}

var pemCertificate = regexp.MustCompile(`^-----BEGIN CERTIFICATE-----\n`)

// Mint mode: YBA generates the root certificate, the resource exports it, and
// import reproduces the state exactly.
func TestAccSelfSignedCertificate_MintAndImport(t *testing.T) {
	label := acctest.RandomName("cert-mint")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckCertificate(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckCertificatesDestroyed,
		Steps: append([]resource.TestStep{
			{
				Config: selfSignedMintConfig("mint", label),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"yba_self_signed_certificate.mint", "uuid"),
					resource.TestMatchResourceAttr(
						"yba_self_signed_certificate.mint", "certificate", pemCertificate),
					resource.TestCheckResourceAttrSet(
						"yba_self_signed_certificate.mint", "expiry_date"),
					resource.TestCheckResourceAttr(
						"yba_self_signed_certificate.mint", "label", label),
					resource.TestCheckResourceAttr(
						"yba_self_signed_certificate.mint", "in_use", "false"),
				),
			},
		}, acctest.ImportPlanSteps(t, "yba_self_signed_certificate.mint",
			selfSignedMintConfig("mint", label), acctest.ImportPlanOptions{})...),
	})
}

// Bring-your-own with a chain that YBA re-encodes on upload: text before the
// first block, CRLF and 76 columns, the legacy X509 CERTIFICATE label, and no
// final newline. YBA stores LF, 64-column CERTIFICATE blocks
// (CertificateHelper.uploadRootCA), so the read-back never matches the config
// textually, and the plans after create and after import must still be empty.
func TestAccSelfSignedCertificate_BringYourOwnPEMStability(t *testing.T) {
	label := acctest.RandomName("cert-byo")
	root := acctest.NewTestCA(t, "tf-acc-byo-root")
	intermediate := root.IssueIntermediateCA(t, "tf-acc-byo-intermediate")
	certPEM := "Bag Attributes\n    friendlyName: tf-acc\n" +
		acctest.MangledPEM(t, intermediate.CertPEM) +
		strings.TrimRight(strings.ReplaceAll(root.CertPEM,
			" CERTIFICATE-----", " X509 CERTIFICATE-----"), "\n")
	config := selfSignedByoConfig("byo", label, certPEM, intermediate.KeyPEM)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckCertificate(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckCertificatesDestroyed,
		Steps: append([]resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"yba_self_signed_certificate.byo", "uuid"),
					resource.TestCheckResourceAttr(
						"yba_self_signed_certificate.byo", "label", label),
				),
			},
		}, acctest.ImportPlanSteps(t, "yba_self_signed_certificate.byo", config,
			acctest.ImportPlanOptions{})...),
	})
}

// YBA puts the server certificates in front of the CA chain in the file that
// the download returns. An intermediate CA can go in either argument, and both
// must plan clean after create and after import with the create config.
func TestAccCustomServerCertificate_IntermediateCA(t *testing.T) {
	root := acctest.NewTestCA(t, "tf-acc-chain-root")
	intermediate := root.IssueIntermediateCA(t, "tf-acc-chain-intermediate")
	serverCert, serverKey := intermediate.IssueServerCert(t, "node.acctest.local")

	cases := []struct {
		name, rootPEM, serverPEM string
		verifyIgnore             []string
	}{
		{
			name:      "intermediate in server_certificate",
			rootPEM:   root.CertPEM,
			serverPEM: serverCert + intermediate.CertPEM,
			// YBA stores the server certificates and the CA chain as one
			// file (CertificateHelper.verifyCertificateConfig:
			// x509CACerts.addAll(0, x509ServerCerts)), so import splits at
			// the first CA certificate. The plan step checks the chain.
			verifyIgnore: []string{"server_certificate", "root_certificate"},
		},
		{
			name:      "intermediate in root_certificate",
			rootPEM:   intermediate.CertPEM + root.CertPEM,
			serverPEM: serverCert,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label := acctest.RandomName("cert-chain")
			config := customServerCertConfig("chain", label, tc.rootPEM, tc.serverPEM, serverKey)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheckCertificate(t) },
				ProviderFactories: acctest.ProviderFactories,
				CheckDestroy:      testAccCheckCertificatesDestroyed,
				Steps: append([]resource.TestStep{{Config: config}},
					acctest.ImportPlanSteps(t, "yba_custom_server_certificate.chain", config,
						acctest.ImportPlanOptions{VerifyIgnore: tc.verifyIgnore})...),
			})
		})
	}
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

func customServerCertConfig(res, label, rootPEM, certPEM, keyPEM string) string {
	return acctest.YBAProviderBlock("GCP") + fmt.Sprintf(`
resource "yba_custom_server_certificate" "%s" {
  label              = "%s"
  root_certificate   = %s
  server_certificate = %s
  server_key         = %s
}
`, res, label, strconv.Quote(rootPEM), strconv.Quote(certPEM), strconv.Quote(keyPEM))
}

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
