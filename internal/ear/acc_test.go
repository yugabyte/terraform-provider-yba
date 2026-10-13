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

package ear_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/ear"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// The GCP EAR tests point at a pre-existing key ring and crypto key so no test
// run creates KMS resources in the project. The service account behind
// TF_VAR_GCP_CREDENTIALS must hold the Cloud KMS permissions YBA checks at
// create time.
const (
	envGCPEARLocationID  = "TF_VAR_GCP_EAR_LOCATION_ID"
	envGCPEARKeyRingID   = "TF_VAR_GCP_EAR_KEY_RING_ID"
	envGCPEARCryptoKeyID = "TF_VAR_GCP_EAR_CRYPTO_KEY_ID"
)

func testAccPreCheckGCPEAR(t *testing.T) {
	// EAR configs live on the GCP fixture YBA, where the universe tests can
	// attach them.
	acctest.TestAccPreCheckCloudYBA(t, "GCP")
	for _, v := range []string{"TF_VAR_GCP_CREDENTIALS", envGCPEARLocationID,
		envGCPEARKeyRingID, envGCPEARCryptoKeyID} {
		if os.Getenv(v) == "" {
			t.Skipf("%s not set; skipping GCP encryption at rest acceptance tests", v)
		}
	}
}

func TestAccGCPEARConfig_ServiceAccount(t *testing.T) {
	rName := acctest.RandomName("gcp-ear")
	res := "yba_gcp_ear_config.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckGCPEAR(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckEARConfigDestroy("GCP"),
		Steps: append(append([]resource.TestStep{
			{
				Config: testAccGCPEARConfigServiceAccount(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEARConfigExists("GCP", res),
					resource.TestCheckResourceAttr(res, "name", rName),
					resource.TestCheckResourceAttr(res, "use_gcp_iam", "false"),
					resource.TestCheckResourceAttr(res, "in_use", "false"),
					resource.TestCheckResourceAttrSet(res, "uuid"),
					// YBA records the existing key's actual protection level.
					resource.TestCheckResourceAttrSet(res, "protection_level"),
				),
			},
		}, acctest.ImportPlanSteps(t, res, testAccGCPEARConfigServiceAccount(rName),
			acctest.ImportPlanOptions{
				// YBA lists the key masked (see testAccGCPEARConfigServiceAccount).
				VerifyIgnore: []string{"credentials"},
			})...),
			resource.TestStep{
				Config: testAccGCPEARConfigServiceAccount(rName) +
					testAccEARConfigDataSource("yba_gcp_ear_config"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.yba_ear_config.test", "uuid", res, "uuid"),
					resource.TestCheckResourceAttr(
						"data.yba_ear_config.test",
						"key_provider",
						"GCP",
					),
					resource.TestCheckResourceAttr("data.yba_ear_config.test", "in_use", "false"),
				),
			}),
	})
}

// TestAccGCPEARConfig_HostIdentity needs a YBA build with host-identity support
// for GCP KMS. It skips while the GCP fixture YBA fails the same version gate
// that the plan applies, and runs once the fixture is upgraded. The fixture's
// attached service account holds the KMS role (acctest/gcp/iam.tf).
func TestAccGCPEARConfig_HostIdentity(t *testing.T) {
	rName := acctest.RandomName("gcp-ear-iam")
	res := "yba_gcp_ear_config.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckGCPEAR(t)
			c, err := acctest.APIClientForCloud("GCP")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.AppVersion(context.Background()); err != nil {
				t.Fatalf("read the GCP fixture YBA version: %v", err)
			}
			if err := ear.GCPRequireHostIdentity(
				context.Background(), c, "use_gcp_iam"); err != nil {
				t.Skipf("skipping the host identity test: %v", err)
			}
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckEARConfigDestroy("GCP"),
		Steps: append([]resource.TestStep{
			{
				Config: testAccGCPEARConfigHostIdentity(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEARConfigExists("GCP", res),
					resource.TestCheckResourceAttr(res, "use_gcp_iam", "true"),
					resource.TestCheckNoResourceAttr(res, "credentials"),
				),
			},
		}, acctest.ImportPlanSteps(t, res, testAccGCPEARConfigHostIdentity(rName),
			acctest.ImportPlanOptions{})...),
	})
}

// The AWS EAR test points at the fixture's KMS key (acctest/aws/kms.tf): a
// configuration without cmk_id makes YBA create a KMS key that nothing
// deletes.
const envAWSEARKeyARN = "TF_VAR_AWS_EAR_KEY_ARN"

// TestAccAWSEARConfig creates the configuration with access keys and then
// switches it in place to the host identity, the AWS fixture YBA's instance
// profile. Both identities hold the KMS grants (acctest/aws/iam.tf).
func TestAccAWSEARConfig(t *testing.T) {
	rName := acctest.RandomName("aws-ear")
	res := "yba_aws_ear_config.test"
	var configUUID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckCloudYBA(t, "AWS")
			for _, v := range []string{"TF_VAR_AWS_ACCESS_KEY_ID",
				"TF_VAR_AWS_SECRET_ACCESS_KEY", envAWSEARKeyARN} {
				if os.Getenv(v) == "" {
					t.Skipf("%s not set; skipping AWS encryption at rest acceptance tests", v)
				}
			}
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckEARConfigDestroy("AWS"),
		Steps: append(append([]resource.TestStep{
			{
				Config: testAccAWSEARConfig(rName, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEARConfigExists("AWS", res),
					// YBA stores the endpoint as given and adds the scheme
					// only when it builds the client.
					resource.TestCheckResourceAttr(res, "kms_endpoint",
						"kms.us-west-2.amazonaws.com"),
					resource.TestCheckResourceAttr(res, "in_use", "false"),
					func(s *terraform.State) error {
						configUUID = s.RootModule().Resources[res].Primary.ID
						return nil
					},
				),
			},
		}, acctest.ImportPlanSteps(t, res, testAccAWSEARConfig(rName, true),
			acctest.ImportPlanOptions{
				// YBA lists the keys masked (see testAccAWSEARConfig).
				VerifyIgnore: []string{"access_key_id", "secret_access_key"},
			})...),
			resource.TestStep{
				Config: testAccAWSEARConfig(rName, false) +
					testAccEARConfigDataSource("yba_aws_ear_config"),
				Check: resource.ComposeTestCheckFunc(
					// Same UUID: the switch is an edit, not a replacement.
					resource.TestCheckResourceAttrPtr(res, "id", &configUUID),
					resource.TestCheckResourceAttr(res, "kms_endpoint", ""),
					resource.TestCheckResourceAttr(
						"data.yba_ear_config.test", "key_provider", "AWS"),
				),
			}),
	})
}

// The Azure EAR test points at the fixture's key vault key
// (acctest/azure/kms.tf), so no test run creates keys in the vault.
const (
	envAzureEARVaultURL         = "TF_VAR_AZURE_EAR_VAULT_URL"
	envAzureEARKeyName          = "TF_VAR_AZURE_EAR_KEY_NAME"
	envAzureYBAIdentityClientID = "TF_VAR_AZURE_YBA_IDENTITY_CLIENT_ID"
)

// TestAccAzureEARConfig creates the configuration with the fixture's service
// principal, imports it, switches it in place to the managed identity of the
// Azure fixture YBA's VM, and back. Both identities have the key permissions
// (acctest/azure/kms.tf). YBA checks the credentials with the KMS on every
// edit, so each edit step fails when YBA cannot authenticate. The switch runs
// on imported state, which has no client_secret: it must still remove the
// secret that YBA stores, or YBA authenticates with that secret and the
// managed identity's client ID. Last, a rotation to a wrong secret fails after
// YBA saved it, and a return to the right secret must send it again.
func TestAccAzureEARConfig(t *testing.T) {
	rName := acctest.RandomName("azure-ear")
	res := "yba_azure_ear_config.test"
	var configUUID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckCloudYBA(t, "AZURE")
			for _, v := range []string{"TF_VAR_AZURE_CLIENT_ID", "TF_VAR_AZURE_CLIENT_SECRET",
				"TF_VAR_AZURE_TENANT_ID", envAzureEARVaultURL, envAzureEARKeyName,
				envAzureYBAIdentityClientID} {
				if os.Getenv(v) == "" {
					t.Skipf("%s not set; skipping Azure encryption at rest acceptance tests", v)
				}
			}
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckEARConfigDestroy("AZURE"),
		Steps: append(append([]resource.TestStep{
			{
				Config: testAccAzureEARConfig(rName, azureSecret, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEARConfigExists("AZURE", res),
					resource.TestCheckResourceAttr(res, "key_size", "2048"),
					resource.TestCheckResourceAttr(res, "in_use", "false"),
					func(s *terraform.State) error {
						configUUID = s.RootModule().Resources[res].Primary.ID
						return nil
					},
				),
			},
		}, acctest.ImportPlanSteps(t, res, testAccAzureEARConfig(rName, azureSecret, true),
			acctest.ImportPlanOptions{
				// YBA lists the secret masked (see testAccAzureEARConfig).
				VerifyIgnore: []string{"client_secret"},
			})...),
			resource.TestStep{
				// Forget the configuration without deleting it, and import it
				// again into state.
				Config: testAccEARConfigRemoved("AZURE"),
			},
			resource.TestStep{
				Config:             testAccAzureEARConfig(rName, azureSecret, true),
				ResourceName:       res,
				ImportState:        true,
				ImportStateIdFunc:  func(*terraform.State) (string, error) { return configUUID, nil },
				ImportStatePersist: true,
			},
			resource.TestStep{
				Config: testAccAzureEARConfig(rName, "", false) +
					testAccEARConfigDataSource("yba_azure_ear_config"),
				Check: resource.ComposeTestCheckFunc(
					// Same UUID: the switch is an edit, not a replacement.
					resource.TestCheckResourceAttrPtr(res, "id", &configUUID),
					resource.TestCheckResourceAttr(res, "client_id",
						os.Getenv(envAzureYBAIdentityClientID)),
					resource.TestCheckResourceAttr(
						"data.yba_ear_config.test", "key_provider", "AZU"),
				),
			},
			resource.TestStep{
				Config: testAccAzureEARConfig(rName, azureSecret, false),
				Check:  resource.TestCheckResourceAttrPtr(res, "id", &configUUID),
			},
			resource.TestStep{
				// The edit task fails, after YBA saved the wrong secret.
				Config:      testAccAzureEARConfig(rName, azureWrongSecret, false),
				ExpectError: regexp.MustCompile(`State: Failure`),
			},
			resource.TestStep{
				// State holds the wrong secret, so the return to the right one
				// plans a change and sends it.
				Config: testAccAzureEARConfig(rName, azureSecret, false),
				Check:  testAccCheckAzureEARSecret(res),
			}),
	})
}

// testAccCheckAzureEARSecret checks that YBA holds the configured client
// secret. The list masks it, so the check reads the by-UUID GET. Its error
// never shows the secret.
func testAccCheckAzureEARSecret(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		apiClient, err := acctest.APIClientForCloud("AZURE")
		if err != nil {
			return err
		}
		id := s.RootModule().Resources[name].Primary.ID
		stored, err := apiClient.VanillaClient.GetKMSConfig(context.Background(),
			apiClient.CustomerID, id, apiClient.APIKey)
		if err != nil {
			return err
		}
		if stored["CLIENT_SECRET"] != os.Getenv("TF_VAR_AZURE_CLIENT_SECRET") {
			return fmt.Errorf("encryption at rest config %s: YBA holds another client secret", id)
		}
		return nil
	}
}

func testAccCheckEARConfigExists(cloud, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for the encryption at rest config")
		}
		found, err := earConfigListed(cloud, r.Primary.ID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("encryption at rest config %s is not listed by YBA", r.Primary.ID)
		}
		return nil
	}
}

func testAccCheckEARConfigDestroy(cloud string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, r := range s.RootModule().Resources {
			if r.Type != "yba_"+strings.ToLower(cloud)+"_ear_config" {
				continue
			}
			found, err := earConfigListed(cloud, r.Primary.ID)
			if err != nil {
				return err
			}
			if found {
				return fmt.Errorf("encryption at rest config %s still exists", r.Primary.ID)
			}
		}
		return nil
	}
}

func earConfigListed(cloud, configUUID string) (bool, error) {
	apiClient, err := acctest.APIClientForCloud(cloud)
	if err != nil {
		return false, err
	}
	configs, response, err := apiClient.YugawareClient.EncryptionAtRestAPI.
		ListKMSConfigs(context.Background(), apiClient.CustomerID).Execute()
	if err != nil {
		return false, utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
			"Encryption At Rest Config", "List")
	}
	for _, cfg := range configs {
		meta, _ := cfg["metadata"].(map[string]interface{})
		if meta["configUUID"] == configUUID {
			return true, nil
		}
	}
	return false, nil
}

func earKeyVariables() string {
	return `
variable "GCP_EAR_LOCATION_ID" {
  type = string
}

variable "GCP_EAR_KEY_RING_ID" {
  type = string
}

variable "GCP_EAR_CRYPTO_KEY_ID" {
  type = string
}
`
}

func testAccGCPEARConfigServiceAccount(name string) string {
	return acctest.YBAProviderBlock("GCP") + earKeyVariables() + fmt.Sprintf(`
variable "GCP_CREDENTIALS" {
  type      = string
  sensitive = true
}

resource "yba_gcp_ear_config" "test" {
  name          = "%s"
  credentials   = var.GCP_CREDENTIALS
  location_id   = var.GCP_EAR_LOCATION_ID
  key_ring_id   = var.GCP_EAR_KEY_RING_ID
  crypto_key_id = var.GCP_EAR_CRYPTO_KEY_ID

  # YBA lists the key file masked (EncryptionAtRestController.listKMSConfigs:
  # CommonUtils.maskConfig masks private_key), so an import cannot read it back.
  lifecycle {
    ignore_changes = [credentials]
  }
}
`, name)
}

func testAccGCPEARConfigHostIdentity(name string) string {
	return acctest.YBAProviderBlock("GCP") + earKeyVariables() + fmt.Sprintf(`
resource "yba_gcp_ear_config" "test" {
  name          = "%s"
  use_gcp_iam   = true
  location_id   = var.GCP_EAR_LOCATION_ID
  key_ring_id   = var.GCP_EAR_KEY_RING_ID
  crypto_key_id = var.GCP_EAR_CRYPTO_KEY_ID
}
`, name)
}

// testAccAWSEARConfig authenticates with the fixture's access keys and a
// custom endpoint, or as the YBA host when withKeys is false.
func testAccAWSEARConfig(name string, withKeys bool) string {
	auth := ""
	if withKeys {
		auth = `
  access_key_id     = var.AWS_ACCESS_KEY_ID
  secret_access_key = var.AWS_SECRET_ACCESS_KEY
  kms_endpoint      = "kms.us-west-2.amazonaws.com"

  # YBA lists the keys masked (EncryptionAtRestController.listKMSConfigs:
  # CommonUtils.maskConfig), so an import cannot read them back.
  lifecycle {
    ignore_changes = [access_key_id, secret_access_key]
  }`
	}
	return acctest.YBAProviderBlock("AWS") + fmt.Sprintf(`
variable "AWS_ACCESS_KEY_ID" {
  type      = string
  sensitive = true
}

variable "AWS_SECRET_ACCESS_KEY" {
  type      = string
  sensitive = true
}

variable "AWS_EAR_KEY_ARN" {
  type = string
}

resource "yba_aws_ear_config" "test" {
  name   = "%s"
  region = "us-west-2"
  cmk_id = var.AWS_EAR_KEY_ARN%s
}
`, name, auth)
}

// HCL values for the client_secret of testAccAzureEARConfig: the service
// principal's secret, or a wrong one.
const (
	azureSecret      = "var.AZURE_CLIENT_SECRET"
	azureWrongSecret = `"not-the-client-secret"`
)

// testAccAzureEARConfig authenticates as the fixture's service principal with
// secret, or as the managed identity of the YBA VM when secret is "". The
// create and import steps set ignoreSecret; the steps that change the secret
// do not.
func testAccAzureEARConfig(name, secret string, ignoreSecret bool) string {
	auth := `
  client_id = var.AZURE_YBA_IDENTITY_CLIENT_ID`
	if secret != "" {
		auth = `
  client_id     = var.AZURE_CLIENT_ID
  client_secret = ` + secret
	}
	if ignoreSecret {
		auth += `

  # YBA lists the secret masked (EncryptionAtRestController.listKMSConfigs:
  # CommonUtils.maskConfig), so an import cannot read it back.
  lifecycle {
    ignore_changes = [client_secret]
  }`
	}
	return acctest.YBAProviderBlock("AZURE") + fmt.Sprintf(`
variable "AZURE_CLIENT_ID" {
  type = string
}

variable "AZURE_CLIENT_SECRET" {
  type      = string
  sensitive = true
}

variable "AZURE_TENANT_ID" {
  type = string
}

variable "AZURE_YBA_IDENTITY_CLIENT_ID" {
  type = string
}

variable "AZURE_EAR_VAULT_URL" {
  type = string
}

variable "AZURE_EAR_KEY_NAME" {
  type = string
}

resource "yba_azure_ear_config" "test" {
  name      = "%s"
  tenant_id = var.AZURE_TENANT_ID
  vault_url = var.AZURE_EAR_VAULT_URL
  key_name  = var.AZURE_EAR_KEY_NAME%s
}
`, name, auth)
}

// testAccEARConfigRemoved removes the cloud's test configuration from state
// and leaves it in YBA.
func testAccEARConfigRemoved(cloud string) string {
	return acctest.YBAProviderBlock(cloud) + fmt.Sprintf(`
removed {
  from = yba_%s_ear_config.test

  lifecycle {
    destroy = false
  }
}
`, strings.ToLower(cloud))
}

func testAccEARConfigDataSource(resourceType string) string {
	return fmt.Sprintf(`
data "yba_ear_config" "test" {
  name = %s.test.name
}
`, resourceType)
}
