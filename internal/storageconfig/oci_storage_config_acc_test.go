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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// TestAccOCIStorageConfig covers create, a region remove and a region add,
// import, and a follow-up plan. The create name has surrounding whitespace
// because YBA stores it trimmed and the provider must plan clean. The keys
// come from the environment because YBA checks them against the bucket.
func TestAccOCIStorageConfig(t *testing.T) {
	rName := acctest.RandomName("oci")
	rNamePadded := "  " + rName + "  "
	rNameUpdated := acctest.RandomName("oci-upd")
	createCfg := testAccOCIStorageConfig(rNamePadded, true)

	steps := []resource.TestStep{{
		Config: createCfg,
		Check: resource.ComposeTestCheckFunc(
			testAccCheckStorageConfigExists("yba_oci_storage_config.test"),
			resource.TestCheckResourceAttr("yba_oci_storage_config.test", "name", rName),
			resource.TestCheckResourceAttr(
				"yba_oci_storage_config.test",
				"use_oci_iam",
				"false",
			),
			resource.TestCheckResourceAttr(
				"yba_oci_storage_config.test",
				"region_locations.#",
				"2",
			),
			resource.TestCheckNoResourceAttr(
				"yba_oci_storage_config.test",
				"access_key_id",
			),
			resource.TestCheckNoResourceAttr(
				"yba_oci_storage_config.test",
				"secret_access_key",
			),
			resource.TestCheckResourceAttrSet("yba_oci_storage_config.test", "config_uuid"),
		),
	}}
	steps = append(steps, acctest.ImportPlanSteps(
		t, "yba_oci_storage_config.test", createCfg, acctest.ImportPlanOptions{})...)
	steps = append(steps,
		resource.TestStep{
			Config: testAccOCIStorageConfig(rNameUpdated, false),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr(
					"yba_oci_storage_config.test",
					"name",
					rNameUpdated,
				),
				resource.TestCheckResourceAttr(
					"yba_oci_storage_config.test",
					"region_locations.#",
					"1",
				),
			),
		},
		resource.TestStep{
			Config: testAccOCIStorageConfig(rNameUpdated, true),
			Check: resource.TestCheckResourceAttr(
				"yba_oci_storage_config.test",
				"region_locations.#",
				"2",
			),
		},
	)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckOCIStorage(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps:             steps,
	})
}

func testAccPreCheckOCIStorage(t *testing.T) {
	acctest.TestAccPreCheckCloudYBA(t, "OCI")
	required := []string{
		"TF_VAR_OCI_BACKUP_LOCATION",
		"TF_VAR_OCI_REGION",
		"TF_VAR_OCI_NAMESPACE",
		"TF_VAR_OCI_ACCESS_KEY_ID",
		"TF_VAR_OCI_SECRET_ACCESS_KEY",
		"TF_VAR_OCI_HOST_BASE",
		"TF_VAR_OCI_BACKUP_LOCATION_2",
		"TF_VAR_OCI_REGION_2",
		"TF_VAR_OCI_HOST_BASE_2",
	}
	for _, v := range required {
		if os.Getenv(v) == "" {
			t.Skipf("%s not set; skipping OCI storage config acceptance tests", v)
		}
	}
}

func testAccOCIStorageConfig(name string, twoRegions bool) string {
	regions := ""
	if twoRegions {
		regions = `
  region_locations {
    region    = var.OCI_REGION
    location  = var.OCI_BACKUP_LOCATION
    host_base = var.OCI_HOST_BASE
  }
  region_locations {
    region    = var.OCI_REGION_2
    location  = var.OCI_BACKUP_LOCATION_2
    host_base = var.OCI_HOST_BASE_2
  }`
	} else {
		regions = `
  region_locations {
    region    = var.OCI_REGION
    location  = var.OCI_BACKUP_LOCATION
    host_base = var.OCI_HOST_BASE
  }`
	}
	return acctest.YBAProviderBlock("OCI") + fmt.Sprintf(`
variable "OCI_BACKUP_LOCATION" { type = string }
variable "OCI_REGION" { type = string }
variable "OCI_NAMESPACE" { type = string }
variable "OCI_ACCESS_KEY_ID" {
  type      = string
  sensitive = true
}
variable "OCI_SECRET_ACCESS_KEY" {
  type      = string
  sensitive = true
}
variable "OCI_HOST_BASE" { type = string }
variable "OCI_BACKUP_LOCATION_2" { type = string }
variable "OCI_REGION_2" { type = string }
variable "OCI_HOST_BASE_2" { type = string }

resource "yba_oci_storage_config" "test" {
  name            = "%s"
  backup_location = var.OCI_BACKUP_LOCATION
  region          = var.OCI_REGION
  namespace       = var.OCI_NAMESPACE
  access_key_id     = var.OCI_ACCESS_KEY_ID
  secret_access_key = var.OCI_SECRET_ACCESS_KEY
  host_base         = var.OCI_HOST_BASE
%s
}
`, name, regions)
}
