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
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// TestAccS3StorageConfig_Basic tests basic S3 storage config creation with credentials
func TestAccS3StorageConfig_Basic(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-s3-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckS3(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3StorageConfigWithCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_s3_storage_config.test"),
					resource.TestCheckResourceAttr("yba_s3_storage_config.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_s3_storage_config.test",
						"use_iam_instance_profile",
						"false",
					),
					resource.TestCheckResourceAttrSet("yba_s3_storage_config.test", "config_uuid"),
				),
			},
		},
	})
}

// TestAccS3StorageConfig_IAM tests S3 storage config with IAM instance profile
func TestAccS3StorageConfig_IAM(t *testing.T) {
	t.Skip("IAM instance profile test requires running on an AWS instance with IAM role")

	rName := fmt.Sprintf("tf-acctest-s3-iam-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3StorageConfigWithIAM(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_s3_storage_config.test"),
					resource.TestCheckResourceAttr("yba_s3_storage_config.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_s3_storage_config.test",
						"use_iam_instance_profile",
						"true",
					),
				),
			},
		},
	})
}

// TestAccS3StorageConfig_Update tests updating an S3 storage config
func TestAccS3StorageConfig_Update(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-s3-%s", sdkacctest.RandString(8))
	rNameUpdated := fmt.Sprintf("tf-acctest-s3-updated-%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheckS3(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccS3StorageConfigWithCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_s3_storage_config.test"),
					resource.TestCheckResourceAttr("yba_s3_storage_config.test", "name", rName),
				),
			},
			{
				Config: testAccS3StorageConfigWithCredentials(rNameUpdated),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_s3_storage_config.test"),
					resource.TestCheckResourceAttr(
						"yba_s3_storage_config.test",
						"name",
						rNameUpdated,
					),
				),
			},
		},
	})
}

func testAccPreCheckS3(t *testing.T) {
	acctest.TestAccPreCheck(t)

	requiredVars := []string{
		"TF_VAR_S3_BACKUP_LOCATION",
		"TF_VAR_AWS_ACCESS_KEY_ID",
		"TF_VAR_AWS_SECRET_ACCESS_KEY",
	}

	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for S3 storage config acceptance tests", v)
		}
	}
}

func testAccCheckStorageConfigExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return fmt.Errorf("no ID is set for storage config resource")
		}
		return nil
	}
}

func testAccCheckStorageConfigDestroy(s *terraform.State) error {
	// Storage configs are deleted by the test framework
	// This is a placeholder for any additional cleanup verification
	return nil
}

func testAccS3StorageConfigWithCredentials(name string) string {
	return fmt.Sprintf(`
variable "S3_BACKUP_LOCATION" {
  type = string
}

variable "AWS_ACCESS_KEY_ID" {
  type      = string
  sensitive = true
}

variable "AWS_SECRET_ACCESS_KEY" {
  type      = string
  sensitive = true
}

resource "yba_s3_storage_config" "test" {
  name                     = "%s"
  backup_location          = var.S3_BACKUP_LOCATION
  access_key_id            = var.AWS_ACCESS_KEY_ID
  secret_access_key        = var.AWS_SECRET_ACCESS_KEY
  use_iam_instance_profile = false
}
`, name)
}

func testAccS3StorageConfigWithIAM(name string) string {
	return fmt.Sprintf(`
variable "S3_BACKUP_LOCATION" {
  type = string
}

resource "yba_s3_storage_config" "test" {
  name                     = "%s"
  backup_location          = var.S3_BACKUP_LOCATION
  use_iam_instance_profile = true
}
`, name)
}
