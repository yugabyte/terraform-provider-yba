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
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// An apply that ends between the upgrade task and its finalize (Ctrl-C, timeout,
// network error) leaves the universe in PreFinalize while state already holds
// the new version and finalize = true. The next plan must finalize it. Step 2
// reaches that state by running the upgrade outside Terraform, then applies a
// config that matches the upgraded universe: only the pending finalize differs.
//
// The universe starts on tfManagedReleaseVersion (registered by the test) and
// upgrades to the fixture's own build, both preview: YBA refuses an upgrade
// across the stable and preview tracks.
func TestAccLong_Universe_AWS_FinalizeInterruptedUpgrade(t *testing.T) {
	var universe client.UniverseResp

	rName := acctest.RandomName("aws-finalize")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckAWS(t)
			acctest.TestAccPreCheckCloudYBA(t, "AWS")
			deleteLeftoverRelease(t, "AWS", tfManagedReleaseVersion)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse("AWS"),
		Steps: []resource.TestStep{
			{
				Config: universeAwsConfigForFinalize(rName,
					"yba_ybdb_release.aws.version"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AWS", "yba_universe.aws", &universe),
					resource.TestCheckResourceAttr("yba_universe.aws",
						"db_version_upgrade_state", "Ready"),
				),
			},
			{
				PreConfig: func() { upgradeUniverseOutOfBand(t, &universe) },
				Config: universeAwsConfigForFinalize(rName,
					"data.yba_release_version.release_version.id"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AWS", "yba_universe.aws", &universe),
					resource.TestCheckResourceAttr("yba_universe.aws",
						"db_version_upgrade_state", "Ready"),
					func(_ *terraform.State) error {
						if s := universe.UniverseDetails.GetSoftwareUpgradeState(); s != "Ready" {
							return fmt.Errorf("YBA db_version_upgrade_state is %q, want Ready", s)
						}
						return nil
					},
				),
			},
		},
	})
}

// upgradeUniverseOutOfBand upgrades the universe to the fixture's own build
// and leaves it in PreFinalize, as an interrupted apply would.
func upgradeUniverseOutOfBand(t *testing.T, universe *client.UniverseResp) {
	t.Helper()
	apiClient, err := acctest.APIClientForCloud("AWS")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c := apiClient.YugawareClient
	cUUID := apiClient.CustomerID
	target, err := apiClient.AppVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	uUUID := universe.GetUniverseUUID()
	clusters := universe.UniverseDetails.Clusters
	clusters[0].UserIntent.YbSoftwareVersion = utils.GetStringPointer(target)
	req := client.SoftwareUpgradeParams{
		YbSoftwareVersion:              target,
		Clusters:                       clusters,
		UpgradeOption:                  "Rolling",
		UpgradeSystemCatalog:           true,
		SleepAfterMasterRestartMillis:  30000,
		SleepAfterTServerRestartMillis: 30000,
	}
	if diags := utils.DispatchAndWait(ctx, "DB Version Upgrade", cUUID, c, time.Hour,
		utils.TestEntity, "Universe", "Upgrade out of band",
		func() (string, *http.Response, error) {
			r, resp, e := c.UniverseUpgradesManagementAPI.UpgradeDBVersion(
				ctx, cUUID, uUUID).SoftwareUpgradeParams(req).Execute()
			if e != nil {
				return "", resp, e
			}
			return r.GetTaskUUID(), resp, nil
		}); diags.HasError() {
		t.Fatalf("upgrade to %s: %v", target, diags)
	}
	u, _, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, uUUID).Execute()
	if err != nil {
		t.Fatal(err)
	}
	if s := u.UniverseDetails.GetSoftwareUpgradeState(); s != "PreFinalize" {
		t.Fatalf("after upgrade to %s the universe is %q, want PreFinalize "+
			"(the upgrade needs no finalize, so this test proves nothing)", target, s)
	}
}

func universeAwsConfigForFinalize(name, softwareVersion string) string {
	return acctest.YBAProviderBlock("AWS") + cloudProviderAWSConfig(name+"-provider") +
		fmt.Sprintf(`
	resource "yba_ybdb_release" "aws" {
		version      = %q
		release_type = "PREVIEW"
		artifact {
			platform     = "LINUX"
			architecture = "x86_64"
			package_url  = %q
		}
	}
`, tfManagedReleaseVersion, tfManagedReleaseURL) +
		universeConfigWithSoftwareVersion("aws", name, 3, `
  		db_version_upgrade_options {
  			finalize = true
  		}
`, softwareVersion)
}
