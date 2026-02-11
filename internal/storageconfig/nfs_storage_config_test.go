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
	"testing"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// TestAccNFSStorageConfig_Basic tests basic NFS storage config creation
func TestAccNFSStorageConfig_Basic(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-nfs-%s", sdkacctest.RandString(8))
	backupLocation := fmt.Sprintf("/mnt/nfs/backups/%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccNFSStorageConfig(rName, backupLocation),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_nfs_storage_config.test"),
					resource.TestCheckResourceAttr("yba_nfs_storage_config.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_nfs_storage_config.test",
						"backup_location",
						backupLocation,
					),
					resource.TestCheckResourceAttrSet("yba_nfs_storage_config.test", "config_uuid"),
				),
			},
		},
	})
}

// TestAccNFSStorageConfig_Update tests updating an NFS storage config name
func TestAccNFSStorageConfig_Update(t *testing.T) {
	rName := fmt.Sprintf("tf-acctest-nfs-%s", sdkacctest.RandString(8))
	rNameUpdated := fmt.Sprintf("tf-acctest-nfs-updated-%s", sdkacctest.RandString(8))
	backupLocation := fmt.Sprintf("/mnt/nfs/backups/%s", sdkacctest.RandString(8))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckStorageConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccNFSStorageConfig(rName, backupLocation),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_nfs_storage_config.test"),
					resource.TestCheckResourceAttr("yba_nfs_storage_config.test", "name", rName),
				),
			},
			{
				Config: testAccNFSStorageConfig(rNameUpdated, backupLocation),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckStorageConfigExists("yba_nfs_storage_config.test"),
					resource.TestCheckResourceAttr(
						"yba_nfs_storage_config.test",
						"name",
						rNameUpdated,
					),
				),
			},
		},
	})
}

func testAccNFSStorageConfig(name, backupLocation string) string {
	return fmt.Sprintf(`
resource "yba_nfs_storage_config" "test" {
  name            = "%s"
  backup_location = "%s"
}
`, name, backupLocation)
}
