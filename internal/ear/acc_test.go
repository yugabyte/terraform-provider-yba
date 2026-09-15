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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// The GCP EAR tests point at a pre-existing key ring and crypto key so no test
// run creates KMS resources in the project. The service account behind
// TF_VAR_GCP_CREDENTIALS must hold the Cloud KMS permissions YBA checks at
// create time.
const (
	envGCPEARLocationID   = "TF_VAR_GCP_EAR_LOCATION_ID"
	envGCPEARKeyRingID    = "TF_VAR_GCP_EAR_KEY_RING_ID"
	envGCPEARCryptoKeyID  = "TF_VAR_GCP_EAR_CRYPTO_KEY_ID"
	envGCPEARHostIdentity = "TF_VAR_GCP_EAR_HOST_IDENTITY"
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

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckGCPEAR(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckEARConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGCPEARConfigServiceAccount(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEARConfigExists(res),
					resource.TestCheckResourceAttr(res, "name", rName),
					resource.TestCheckResourceAttr(res, "use_gcp_iam", "false"),
					resource.TestCheckResourceAttr(res, "in_use", "false"),
					resource.TestCheckResourceAttrSet(res, "uuid"),
					// YBA records the existing key's actual protection level.
					resource.TestCheckResourceAttrSet(res, "protection_level"),
				),
			},
			{
				// Credentials never come back from YBA, so they stay empty after
				// import; the first apply after import re-submits them.
				ResourceName:            res,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"credentials"},
			},
			{
				Config: testAccGCPEARConfigServiceAccount(rName) + testAccEARConfigDataSource(),
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
			},
		},
	})
}

// TestAccGCPEARConfig_HostIdentity needs a YBA build with host-identity support
// for GCP KMS and a fixture host whose attached service account can use the
// key ring, so it is opt-in.
func TestAccGCPEARConfig_HostIdentity(t *testing.T) {
	rName := acctest.RandomName("gcp-ear-iam")
	res := "yba_gcp_ear_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckGCPEAR(t)
			if os.Getenv(envGCPEARHostIdentity) != "true" {
				t.Skipf(
					"%s is not \"true\"; skipping the host identity test",
					envGCPEARHostIdentity,
				)
			}
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckEARConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGCPEARConfigHostIdentity(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckEARConfigExists(res),
					resource.TestCheckResourceAttr(res, "use_gcp_iam", "true"),
					resource.TestCheckNoResourceAttr(res, "credentials"),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckEARConfigExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for the encryption at rest config")
		}
		found, err := earConfigListed(r.Primary.ID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("encryption at rest config %s is not listed by YBA", r.Primary.ID)
		}
		return nil
	}
}

func testAccCheckEARConfigDestroy(s *terraform.State) error {
	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_gcp_ear_config" {
			continue
		}
		found, err := earConfigListed(r.Primary.ID)
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("encryption at rest config %s still exists", r.Primary.ID)
		}
	}
	return nil
}

func earConfigListed(configUUID string) (bool, error) {
	apiClient, err := acctest.APIClientForCloud("GCP")
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

func testAccEARConfigDataSource() string {
	return `
data "yba_ear_config" "test" {
  name = yba_gcp_ear_config.test.name
}
`
}
