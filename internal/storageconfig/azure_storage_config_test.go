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

package storageconfig_test

import (
	"fmt"
	"os"
	"testing"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// TestAccAzureStorageConfig_Basic tests basic Azure storage config creation
func TestAccAzureStorageConfig_Basic(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-az-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckAzureStorage(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAzureStorageConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_azure_storage_config.test"),
					resource.TestCheckResourceAttr("yba_azure_storage_config.test", "name", rName),
					resource.TestCheckResourceAttrSet(
						"yba_azure_storage_config.test",
						"config_uuid",
					),
				),
			},
		},
	})
}

// TestAccAzureStorageConfig_Update tests updating an Azure storage config
func TestAccAzureStorageConfig_Update(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-az-%s", sdkacctest.RandString(8))
	rNameUpdated := fmt.Sprintf("tf-acctest-az-updated-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckAzureStorage(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAzureStorageConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_azure_storage_config.test"),
					resource.TestCheckResourceAttr("yba_azure_storage_config.test", "name", rName),
				),
			},
			{
				Config: testAccAzureStorageConfig(rNameUpdated),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_azure_storage_config.test"),
					resource.TestCheckResourceAttr(
						"yba_azure_storage_config.test",
						"name",
						rNameUpdated,
					),
				),
			},
		},
	})
}

func testAccPreCheckAzureStorage(t *testing.T) {
	acctest.TestAccPreCheck(t)

	requiredVars := []string{
		"TF_VAR_AZURE_BACKUP_LOCATION",
		"TF_VAR_AZURE_SAS_TOKEN",
	}

	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for Azure storage config acceptance tests", v)
		}
	}
}

func testAccAzureStorageConfig(name string) string {
	return fmt.Sprintf(`
variable "AZURE_BACKUP_LOCATION" {
  type = string
}

variable "AZURE_SAS_TOKEN" {
  type      = string
  sensitive = true
}

resource "yba_azure_storage_config" "test" {
  name            = "%s"
  backup_location = var.AZURE_BACKUP_LOCATION
  sas_token       = var.AZURE_SAS_TOKEN
}
`, name)
}
