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

// TestAccGCSStorageConfig_Basic tests basic GCS storage config creation with credentials
func TestAccGCSStorageConfig_Basic(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-gcs-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckGCS(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGCSStorageConfigWithCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_gcs_storage_config.test"),
					resource.TestCheckResourceAttr("yba_gcs_storage_config.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_gcs_storage_config.test",
						"use_gcp_iam",
						"false",
					),
					resource.TestCheckResourceAttrSet("yba_gcs_storage_config.test", "config_uuid"),
				),
			},
		},
	})
}

// TestAccGCSStorageConfig_IAM tests GCS storage config with GCP IAM (workload identity)
func TestAccGCSStorageConfig_IAM(t *testing.T) {
	t.Skip("GCP IAM test requires running on a GKE cluster with workload identity")

	rName := fmt.Sprintf("tf-acctest-gcs-iam-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGCSStorageConfigWithIAM(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_gcs_storage_config.test"),
					resource.TestCheckResourceAttr("yba_gcs_storage_config.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_gcs_storage_config.test",
						"use_gcp_iam",
						"true",
					),
				),
			},
		},
	})
}

// TestAccGCSStorageConfig_Update tests updating a GCS storage config
func TestAccGCSStorageConfig_Update(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-gcs-%s", sdkacctest.RandString(8))
	rNameUpdated := fmt.Sprintf("tf-acctest-gcs-updated-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckGCS(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGCSStorageConfigWithCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_gcs_storage_config.test"),
					resource.TestCheckResourceAttr("yba_gcs_storage_config.test", "name", rName),
				),
			},
			{
				Config: testAccGCSStorageConfigWithCredentials(rNameUpdated),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_gcs_storage_config.test"),
					resource.TestCheckResourceAttr(
						"yba_gcs_storage_config.test",
						"name",
						rNameUpdated,
					),
				),
			},
		},
	})
}

func testAccPreCheckGCS(t *testing.T) {
	acctest.TestAccPreCheck(t)

	requiredVars := []string{
		"TF_VAR_GCS_BACKUP_LOCATION",
		"TF_VAR_GCP_CREDENTIALS",
	}

	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for GCS storage config acceptance tests", v)
		}
	}
}

func testAccGCSStorageConfigWithCredentials(name string) string {
	return fmt.Sprintf(`
variable "GCS_BACKUP_LOCATION" {
  type = string
}

variable "GCP_CREDENTIALS" {
  type      = string
  sensitive = true
}

resource "yba_gcs_storage_config" "test" {
  name            = "%s"
  backup_location = var.GCS_BACKUP_LOCATION
  credentials     = var.GCP_CREDENTIALS
  use_gcp_iam     = false
}
`, name)
}

func testAccGCSStorageConfigWithIAM(name string) string {
	return fmt.Sprintf(`
variable "GCS_BACKUP_LOCATION" {
  type = string
}

resource "yba_gcs_storage_config" "test" {
  name            = "%s"
  backup_location = var.GCS_BACKUP_LOCATION
  use_gcp_iam     = true
}
`, name)
}
