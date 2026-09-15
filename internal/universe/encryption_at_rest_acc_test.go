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
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// The encryption-at-rest tests point at a pre-existing Cloud KMS key ring and
// crypto key (same variables as the yba_gcp_ear_config tests), so no run
// creates KMS resources in the project.
const (
	envGCPEARLocationID  = "TF_VAR_GCP_EAR_LOCATION_ID"
	envGCPEARKeyRingID   = "TF_VAR_GCP_EAR_KEY_RING_ID"
	envGCPEARCryptoKeyID = "TF_VAR_GCP_EAR_CRYPTO_KEY_ID"
)

func testAccPreCheckGCPEncryptionAtRest(t *testing.T) {
	acctest.TestAccPreCheckGCP(t)
	acctest.TestAccPreCheckCloudYBA(t, "GCP")
	for _, v := range []string{envGCPEARLocationID, envGCPEARKeyRingID, envGCPEARCryptoKeyID} {
		if os.Getenv(v) == "" {
			t.Skipf("%s not set; skipping encryption at rest universe tests", v)
		}
	}
}

// TestAccLong_Universe_GCP_EncryptionAtRest walks one universe through every
// encryption-at-rest operation the block drives: enable in place, universe key
// rotation, master key rotation to a second configuration, and disable. The
// second configuration reuses the same crypto key: YBA treats it as a distinct
// master key and re-wraps every universe key, which is the path under test,
// without needing a second key in the project.
func TestAccLong_Universe_GCP_EncryptionAtRest(t *testing.T) {
	var universe client.UniverseResp
	rName := acctest.RandomName("gcp-ear")
	uni := "yba_universe.gcp"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckGCPEncryptionAtRest(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckDestroyProviderAndUniverse("GCP"),
			testAccCheckEARConfigsDestroyed,
		),
		Steps: []resource.TestStep{
			{
				// Plain universe; configuration A exists but is not attached.
				Config: universeGcpConfigWithEAR(rName, 1, ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", uni, &universe),
					testAccCheckUniverseEAR(&universe, false, ""),
				),
			},
			{
				// Enable in place.
				Config: universeGcpConfigWithEAR(rName, 1,
					earBlock("yba_gcp_ear_config.a.uuid", "", true)),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", uni, &universe),
					testAccCheckUniverseEAR(&universe, true, "yba_gcp_ear_config.a"),
					testAccCheckUniverseKeyCount(&universe, 1),
				),
			},
			{
				// Universe key rotation: a second key under the same master key.
				Config: universeGcpConfigWithEAR(rName, 1,
					earBlock("yba_gcp_ear_config.a.uuid", "rotate-1", true)),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", uni, &universe),
					testAccCheckUniverseEAR(&universe, true, "yba_gcp_ear_config.a"),
					testAccCheckUniverseKeyCount(&universe, 2),
				),
			},
			{
				// Master key rotation to configuration B; the trigger is unchanged.
				Config: universeGcpConfigWithEAR(rName, 2,
					earBlock("yba_gcp_ear_config.b.uuid", "rotate-1", true)),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", uni, &universe),
					testAccCheckUniverseEAR(&universe, true, "yba_gcp_ear_config.b"),
				),
			},
			{
				// Disable; YBA keeps reporting the last configuration.
				Config: universeGcpConfigWithEAR(rName, 2,
					earBlock("yba_gcp_ear_config.b.uuid", "rotate-1", false)),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", uni, &universe),
					testAccCheckUniverseEAR(&universe, false, "yba_gcp_ear_config.b"),
				),
			},
		},
	})
}

// testAccCheckUniverseEAR asserts the live encryption state and, when
// configRes is set, that the universe references that resource's configuration.
func testAccCheckUniverseEAR(
	universe *client.UniverseResp, enabled bool, configRes string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		cfg := universe.UniverseDetails.EncryptionAtRestConfig
		got := cfg != nil && cfg.GetEncryptionAtRestEnabled()
		if got != enabled {
			return fmt.Errorf("encryption at rest enabled = %t, want %t", got, enabled)
		}
		if configRes == "" {
			return nil
		}
		r, ok := s.RootModule().Resources[configRes]
		if !ok {
			return fmt.Errorf("resource not found: %s", configRes)
		}
		if cfg == nil || cfg.GetKmsConfigUUID() != r.Primary.ID {
			return fmt.Errorf("universe kms config = %q, want %s (%s)",
				cfg.GetKmsConfigUUID(), configRes, r.Primary.ID)
		}
		return nil
	}
}

// testAccCheckUniverseKeyCount counts the universe's key history entries: one
// per universe key generated under the same master key.
func testAccCheckUniverseKeyCount(
	universe *client.UniverseResp, want int,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		apiClient, err := acctest.APIClientForCloud("GCP")
		if err != nil {
			return err
		}
		history, response, err := apiClient.YugawareClient.EncryptionAtRestAPI.
			GetKeyRefHistory(context.Background(), apiClient.CustomerID,
				universe.GetUniverseUUID()).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Universe", "Read - Key History")
		}
		if len(history) != want {
			return fmt.Errorf("universe key history has %d entries, want %d", len(history), want)
		}
		return nil
	}
}

func testAccCheckEARConfigsDestroyed(s *terraform.State) error {
	apiClient, err := acctest.APIClientForCloud("GCP")
	if err != nil {
		return err
	}
	configs, response, err := apiClient.YugawareClient.EncryptionAtRestAPI.
		ListKMSConfigs(context.Background(), apiClient.CustomerID).Execute()
	if err != nil {
		return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
			"Encryption At Rest Config", "List")
	}
	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_gcp_ear_config" {
			continue
		}
		for _, cfg := range configs {
			meta, _ := cfg["metadata"].(map[string]interface{})
			if meta["configUUID"] == r.Primary.ID {
				return fmt.Errorf("encryption at rest config %s still exists", r.Primary.ID)
			}
		}
	}
	return nil
}

// earBlock renders the universe's encryption_at_rest block.
func earBlock(configRef, trigger string, enabled bool) string {
	block := fmt.Sprintf(`
		encryption_at_rest {
			enabled         = %t
			kms_config_uuid = %s
`, enabled, configRef)
	if trigger != "" {
		block += fmt.Sprintf("\t\t\tuniverse_key_rotation_trigger = %q\n", trigger)
	}
	return block + "\t\t}\n"
}

// universeGcpConfigWithEAR is the GCP universe config plus numConfigs
// encryption-at-rest configurations (a, b) on the shared key, and the given
// encryption_at_rest block inside the universe.
func universeGcpConfigWithEAR(name string, numConfigs int, block string) string {
	configs := `
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
	for i, label := range []string{"a", "b"}[:numConfigs] {
		configs += fmt.Sprintf(`
	resource "yba_gcp_ear_config" "%s" {
		name          = "%s-ear-%d"
		credentials   = var.GCP_CREDENTIALS
		location_id   = var.GCP_EAR_LOCATION_ID
		key_ring_id   = var.GCP_EAR_KEY_RING_ID
		crypto_key_id = var.GCP_EAR_CRYPTO_KEY_ID
	}
`, label, name, i)
	}
	return acctest.YBAProviderBlock("GCP") + cloudProviderGCPConfig(name+"-provider") +
		configs + universeConfigWithProviderWithNodesAndExtra("gcp", name, 3, block)
}
