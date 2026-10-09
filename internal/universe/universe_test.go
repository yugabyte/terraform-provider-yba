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
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// The universe runs on a yba_ybdb_release that the test registers. Only this test
// may use tfManagedReleaseVersion: YBA allows one release per version.
//
// The third step covers an apply that ended between the DB upgrade task and its
// finalize (Ctrl-C, timeout, network error): the universe is in PreFinalize
// while state holds the new version and finalize = true. PreConfig runs that
// upgrade outside Terraform, to the fixture's own build (preview, like
// tfManagedReleaseVersion: YBA refuses an upgrade across the stable and preview
// tracks), and the apply must finalize it.
//
// The config declares an empty per_process block and a per_az entry with no
// flags, which never reach YBA. The plan after each apply checks that state
// keeps both. Import cannot recover the per_az entry, so the import step
// ignores per_az.
func TestAccLong_Universe_GCP_UpdatePrimaryNodes(t *testing.T) {
	var universe client.UniverseResp

	rName := acctest.RandomName("gcp-universe")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckGCP(t)
			acctest.TestAccPreCheckCloudYBA(t, "GCP")
			deleteLeftoverRelease(t, "GCP", tfManagedReleaseVersion)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse("GCP"),
		Steps: []resource.TestStep{
			{
				Config: universeGcpConfigWithTFRelease(rName, 3, tfManagedReleaseExpr),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckNumNodes(&universe, 3),
					testAccCheckCreateTaskRanHooks("GCP", &universe),
				),
			},
			{
				Config: universeGcpConfigWithTFRelease(rName, 4, tfManagedReleaseExpr),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
			},
			{
				PreConfig: func() { upgradeUniverseOutOfBand(t, "GCP", &universe) },
				Config: universeGcpConfigWithTFRelease(rName, 4,
					"data.yba_release_version.release_version.id"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					resource.TestCheckResourceAttr("yba_universe.gcp",
						"db_version_upgrade_state", "Ready"),
					func(_ *terraform.State) error {
						if s := universe.UniverseDetails.GetSoftwareUpgradeState(); s != "Ready" {
							return fmt.Errorf("YBA db_version_upgrade_state is %q, want Ready", s)
						}
						return nil
					},
				),
			},
			{
				// Read on import has no raw config and no prior state, and must
				// still write the per_process block. db_version_upgrade_options
				// is config-only, YBA returns the passwords redacted, and the
				// per_az entry sets no flags, so it never reaches YBA.
				ResourceName:      "yba_universe.gcp",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"db_version_upgrade_options",
					"clusters.0.user_intent.0.ysql_password",
					"clusters.0.user_intent.0.ycql_password",
					"clusters.0.user_intent.0.specific_gflags.0.per_az",
				},
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					k := "clusters.0.user_intent.0.specific_gflags.0.per_process.#"
					if n := s[0].Attributes[k]; n != "1" {
						return fmt.Errorf("imported %s = %q, want 1", k, n)
					}
					return nil
				},
			},
		},
	})
}

func TestAccLong_Universe_AWS_UpdatePrimaryNodes(t *testing.T) {
	var universe client.UniverseResp

	rName := acctest.RandomName("aws-universe")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckAWS(t)
			acctest.TestAccPreCheckCloudYBA(t, "AWS")
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse("AWS"),
		Steps: []resource.TestStep{
			{
				Config: universeAwsConfigWithNodes(rName, 3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AWS", "yba_universe.aws", &universe),
					testAccCheckNumNodes(&universe, 3),
					testAccCheckCreateTaskRanHooks("AWS", &universe),
				),
			},
			{
				Config: universeAwsConfigWithNodes(rName, 4),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AWS", "yba_universe.aws", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
			},
		},
	})
}

func TestAccLong_Universe_Azure_UpdatePrimaryNodes(t *testing.T) {
	var universe client.UniverseResp

	rName := acctest.RandomName("azu-universe")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckAzure(t)
			acctest.TestAccPreCheckCloudYBA(t, "AZURE")
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse("AZURE"),
		Steps: []resource.TestStep{
			{
				Config: universeAzureConfigWithNodes(rName, 3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AZURE", "yba_universe.azu", &universe),
					testAccCheckNumNodes(&universe, 3),
					testAccCheckCreateTaskRanHooks("AZURE", &universe),
				),
			},
			{
				Config: universeAzureConfigWithNodes(rName, 4),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AZURE", "yba_universe.azu", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
			},
		},
	})
}

func testAccCheckDestroyProviderAndUniverse(cloud string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		apiClient, err := acctest.APIClientForCloud(cloud)
		if err != nil {
			return err
		}
		conn := apiClient.YugawareClient
		cUUID := apiClient.CustomerID

		for _, r := range s.RootModule().Resources {
			switch r.Type {
			case "yba_universe":
				_, _, err := conn.UniverseManagementAPI.GetUniverse(context.Background(), cUUID,
					r.Primary.ID).Execute()
				// A 404 means the universe is gone (destroyed) — that is the success
				// case. Only a successful GET means it still exists.
				if err == nil {
					return errors.New("Universe resource is not destroyed")
				}
			case "yba_hook":
				_, err := apiClient.VanillaClient.GetHook(context.Background(), cUUID,
					r.Primary.ID, apiClient.APIKey)
				if err == nil {
					return fmt.Errorf("hook %s is not destroyed", r.Primary.ID)
				}
				if !errors.Is(err, api.ErrHookMissing) {
					return fmt.Errorf("checking destroyed hook %s: %w", r.Primary.ID, err)
				}
			case "yba_ybdb_release":
				_, response, err := conn.NewReleaseManagementAPI.GetNewRelease(
					context.Background(), cUUID, r.Primary.ID).Execute()
				if err == nil {
					return fmt.Errorf("release %s is not destroyed", r.Primary.ID)
				}
				if !utils.IsReleaseNotFound(response, err) {
					return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
						r.Primary.ID, "Destroy Check")
				}
			case "yba_cloud_provider":
				// Provider deletion is async; poll until it disappears rather than
				// sleeping a fixed interval (which can false-fail and leak the
				// provider if deletion runs long).
				deadline := time.Now().Add(5 * time.Minute)
				for {
					res, response, err := conn.CloudProvidersAPI.GetListOfProviders(
						context.Background(), cUUID).Execute()
					if err != nil {
						return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
							"Universe", "Read - Cloud Provider")
					}
					found := false
					for _, p := range res {
						if *p.Uuid == r.Primary.ID {
							found = true
							break
						}
					}
					if !found {
						break
					}
					if time.Now().After(deadline) {
						return errors.New("Cloud provider is not destroyed")
					}
					time.Sleep(10 * time.Second)
				}
			}
		}

		return nil
	}
}

// testAccCheckCreateTaskRanHooks asserts from the task record on YBA that the
// universe's Create task ran its "Running Hooks" subtask group to Success: the
// provider-scoped PostNodeProvision hook in the config executed on the new
// nodes. A disabled flag or an unbound hook leaves the group out of the task
// entirely; a failing script fails the create itself.
func testAccCheckCreateTaskRanHooks(
	cloud string, universe *client.UniverseResp) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		apiClient, err := acctest.APIClientForCloud(cloud)
		if err != nil {
			return err
		}
		conn := apiClient.YugawareClient
		cUUID := apiClient.CustomerID
		tasks, response, err := conn.CustomerTasksAPI.
			TasksList(context.Background(), cUUID).
			UUUID(universe.GetUniverseUUID()).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Universe", "Read - Tasks")
		}
		createTask := ""
		for i := range tasks {
			if tasks[i].GetType() == "Create" {
				createTask = tasks[i].GetId()
				break
			}
		}
		if createTask == "" {
			return fmt.Errorf("no Create task found for universe %s",
				universe.GetUniverseUUID())
		}
		status, response, err := conn.CustomerTasksAPI.
			TaskStatus(context.Background(), cUUID, createTask).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Universe", "Read - Task Status")
		}
		details, _ := status["details"].(map[string]interface{})
		groups, _ := details["taskDetails"].([]interface{})
		seen := make([]string, 0, len(groups))
		for _, g := range groups {
			group, _ := g.(map[string]interface{})
			title, _ := group["title"].(string)
			state, _ := group["state"].(string)
			seen = append(seen, title+"="+state)
			if title != "Running Hooks" {
				continue
			}
			if state != "Success" {
				return fmt.Errorf("Create task %s ran hooks but the group is %q, want Success",
					createTask, state)
			}
			return nil
		}
		return fmt.Errorf("Create task %s has no \"Running Hooks\" subtask group, so the "+
			"PostNodeProvision hook did not run (groups: %v)", createTask, seen)
	}
}

func testAccCheckUniverseExists(
	cloud, name string, universe *client.UniverseResp) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for universe resource")
		}

		apiClient, err := acctest.APIClientForCloud(cloud)
		if err != nil {
			return err
		}
		conn := apiClient.YugawareClient
		cUUID := apiClient.CustomerID
		res, response, err := conn.UniverseManagementAPI.GetUniverse(context.Background(), cUUID,
			r.Primary.ID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Universe", "Read - Universe")
			return errMessage
		}
		*universe = *res
		return nil
	}
}

func testAccCheckNumNodes(universe *client.UniverseResp, expected int32) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		found := universe.UniverseDetails.Clusters[0].UserIntent.GetNumNodes()
		if found != expected {
			return fmt.Errorf("expected %d nodes; found %d", expected, found)
		}
		return nil
	}
}

func universeGcpConfigWithNodes(name string, nodes int) string {
	return acctest.YBAProviderBlock("GCP") + cloudProviderGCPConfig(name+"-provider") +
		universeConfigWithProviderWithNodes("gcp", name, nodes)
}

func universeAwsConfigWithNodes(name string, nodes int) string {
	return acctest.YBAProviderBlock("AWS") + cloudProviderAWSConfig(name+"-provider") +
		universeConfigWithProviderWithNodes("aws", name, nodes)
}

func universeAzureConfigWithNodes(name string, nodes int) string {
	return acctest.YBAProviderBlock("AZURE") + cloudProviderAzureConfig(name+"-provider") +
		universeConfigWithProviderWithNodes("azu", name, nodes)
}

// An older preview build than the fixture's own, so tests that read
// yba_release_version without a filter still pick the fixture build.
const (
	tfManagedReleaseVersion = "2.25.2.0-b359"
	tfManagedReleaseURL     = "https://software.yugabyte.com/releases/2.25.2.0/" +
		"yugabyte-2.25.2.0-b359-linux-x86_64.tar.gz"
)

const tfManagedReleaseExpr = "yba_ybdb_release.gcp.version"

// universeGcpConfigWithTFRelease is universeGcpConfigWithNodes with the
// yba_ybdb_release registered and the universe on softwareVersion, with
// finalize = true, an empty per_process block, and a per_az entry with no flags.
// The nodes download package_url themselves.
func universeGcpConfigWithTFRelease(name string, nodes int, softwareVersion string) string {
	return acctest.YBAProviderBlock("GCP") + cloudProviderGCPConfig(name+"-provider") +
		fmt.Sprintf(`
	resource "yba_ybdb_release" "gcp" {
		version      = %q
		release_type = "PREVIEW"
		artifact {
			platform     = "LINUX"
			architecture = "x86_64"
			package_url  = %q
		}
	}
`, tfManagedReleaseVersion, tfManagedReleaseURL) +
		universeConfigWithSoftwareVersion("gcp", name, nodes, `
  		db_version_upgrade_options {
  			finalize = true
  		}
`, `
				specific_gflags {
					per_process {
						master_gflags  = {}
						tserver_gflags = {}
					}
					per_az {
						az_uuid = yba_cloud_provider.gcp.regions[0].zones[0].uuid
					}
				}`, softwareVersion)
}

// upgradeUniverseOutOfBand upgrades the universe to the fixture's own build
// outside Terraform and leaves it in PreFinalize, as an interrupted apply would.
func upgradeUniverseOutOfBand(t *testing.T, cloud string, universe *client.UniverseResp) {
	t.Helper()
	apiClient, err := acctest.APIClientForCloud(cloud)
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
	for i := range clusters {
		clusters[i].UserIntent.YbSoftwareVersion = utils.GetStringPointer(target)
	}
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
			"(the upgrade needs no finalize, so the step proves nothing)", target, s)
	}
}

// deleteLeftoverRelease is a best-effort cleanup of what an aborted run left
// on version: the universes that use it, then the release, which would
// otherwise fail the next create. A run in progress on the same fixture loses
// its universe and release too.
func deleteLeftoverRelease(t *testing.T, cloud, version string) {
	t.Helper()
	apiClient, err := acctest.APIClientForCloud(cloud)
	if err != nil {
		t.Logf("leftover release cleanup skipped: %v", err)
		return
	}
	ctx := context.Background()
	c := apiClient.YugawareClient
	cUUID := apiClient.CustomerID
	releases, _, err := c.NewReleaseManagementAPI.ListNewReleases(ctx, cUUID).Execute()
	if err != nil {
		t.Logf("leftover release cleanup skipped: %v", err)
		return
	}
	for _, r := range releases {
		if r.Version != version {
			continue
		}
		for _, u := range r.Universes {
			t.Logf("deleting leftover universe %s (%s) on release %s", u.Name, u.Uuid, version)
			diags := utils.DispatchAndWait(ctx, "Delete Universe", cUUID, c, 30*time.Minute,
				utils.TestEntity, "Universe", "Delete",
				func() (string, *http.Response, error) {
					task, resp, err := c.UniverseManagementAPI.DeleteUniverse(ctx, cUUID, u.Uuid).
						IsForceDelete(true).Execute()
					if err != nil {
						return "", resp, err
					}
					return task.GetTaskUUID(), resp, nil
				})
			if diags.HasError() {
				t.Logf("delete leftover universe %s: %v", u.Uuid, diags)
			}
		}
		t.Logf("deleting leftover release %s (%s)", version, r.ReleaseUuid)
		if _, _, err := c.NewReleaseManagementAPI.DeleteNewRelease(
			ctx, cUUID, r.ReleaseUuid).Execute(); err != nil {
			t.Logf("delete leftover release %s: %v", r.ReleaseUuid, err)
		}
	}
}

// universeConfigWithProviderWithNodes declares the universe plus a
// provider-scoped PostNodeProvision hook it depends on. The hook rides along so
// every long test that provisions nodes also proves custom hooks execute on
// them, without a universe of its own: a no-op Bash script, run as the yugabyte
// user (no sudo), scoped to this test's provider so it never fires on another
// test's nodes. depends_on makes Terraform create the hook before provisioning
// starts; testAccCheckCreateTaskRanHooks verifies it ran. Custom hooks are
// enabled on each fixture YBA by yba_runtime_config.enable_custom_hooks in
// acctest/<cloud>/yba.tf, not by the tests.
func universeConfigWithProviderWithNodes(p string, name string, nodes int) string {
	return universeConfigWithProviderWithNodesAndExtra(p, name, nodes, "")
}

// universeConfigWithProviderWithNodesAndExtra is universeConfigWithProviderWithNodes
// with extra top-level universe HCL placed before communication_ports.
func universeConfigWithProviderWithNodesAndExtra(
	p string, name string, nodes int, extra string,
) string {
	return universeConfigWithSoftwareVersion(p, name, nodes, extra, "",
		"data.yba_release_version.release_version.id")
}

// universeConfigWithSoftwareVersion is universeConfigWithProviderWithNodesAndExtra
// with yb_software_version set to the HCL expression softwareVersion, and
// userIntentExtra placed inside user_intent.
func universeConfigWithSoftwareVersion(
	p string, name string, nodes int, extra string, userIntentExtra string,
	softwareVersion string,
) string {
	return fmt.Sprintf(`
	data "yba_provider_key" "%[1]s_key" {
  		provider_id = yba_cloud_provider.%[1]s.id
	}

	data "yba_release_version" "release_version"{
		depends_on = [
			data.yba_provider_key.%[1]s_key
  		]
	}

	resource "yba_hook" "%[1]s_post_provision" {
		name           = "%[2]s-post-provision.sh"
		execution_lang = "Bash"
		hook_text      = "#!/bin/bash\necho yba-acctest-post-node-provision\n"
		trigger_type   = "PostNodeProvision"
		provider_uuid  = yba_cloud_provider.%[1]s.id
	}

	resource "yba_universe" "%[1]s" {
		depends_on = [yba_hook.%[1]s_post_provision]

  		clusters {
    		cluster_type = "PRIMARY"
    		user_intent {
      			universe_name      = "%[2]s"
      			provider           = yba_cloud_provider.%[1]s.id
      			region_list        = yba_cloud_provider.%[1]s.regions[*].uuid
      			num_nodes          = %[3]d
      			replication_factor = 3
      			instance_type      = "%[4]s"
      			device_info {
        			num_volumes  = 1
        			volume_size  = 375
        			storage_type = "%[5]s"
      			}
				assign_public_ip              = true
				use_time_sync                 = true
				enable_ysql                   = true
				enable_node_to_node_encrypt   = true
				enable_client_to_node_encrypt = true
				yb_software_version           = %[7]s
				access_key_code               = data.yba_provider_key.%[1]s_key.id
				instance_tags = {
					"yb_owner"  = "terraform_acctest"
					"yb_task"   = "dev"
					"yb_dept"   = "dev"
				}
				%[8]s
    		}
  		}
  		%[6]s
  		communication_ports {}
	}
`, p, name, nodes, getUniverseInstanceType(p), getUniverseStorageType(p), extra,
		softwareVersion, userIntentExtra)
}

func getUniverseStorageType(p string) string {
	switch p {
	case "gcp":
		return "Persistent"
	case "aws":
		return "GP2"
	}
	return "Premium_LRS"
}

func getUniverseInstanceType(p string) string {
	// All clouds use a current-gen, 2-vCPU instance — YugabyteDB's documented
	// minimum is 2 cores / 2 GB RAM, and these tests only need a node to come up.
	switch p {
	case "gcp":
		return "n2-standard-2"
	case "aws":
		return "c6i.large"
	}
	return "Standard_D2s_v4"
}

func cloudProviderGCPConfig(name string) string {
	return fmt.Sprintf(`
	variable "GCP_VPC_NETWORK" {
		type        = string
		description = "GCP VPC network to run acceptance testing"
	}

	variable "GCP_REGION" {
		type        = string
		description = "GCP region to run acceptance testing"
	}

	variable "GCP_CREDENTIALS" {
		type        = string
		sensitive   = true
		description = "GCP service account credentials JSON"
	}

	variable "GCP_PROJECT_ID" {
		type        = string
		description = "GCP project ID"
	}

	variable "GCP_SUBNETWORK" {
		type        = string
		description = "GCP shared subnet for universe nodes"
	}

	resource "yba_cloud_provider" "gcp" {
  		code = "gcp"
  		name = "%s"
  		gcp_config_settings {
  			network      = var.GCP_VPC_NETWORK
  			use_host_vpc = false
  			project_id   = var.GCP_PROJECT_ID
  			credentials  = var.GCP_CREDENTIALS
  		}
  		regions {
    		code = var.GCP_REGION
    		name = var.GCP_REGION
    		zones {
    			subnet = var.GCP_SUBNETWORK
    		}
  		}
  		ssh_port        = 22
  		air_gap_install = false
	}
`, name)
}

func cloudProviderAWSConfig(name string) string {
	return fmt.Sprintf(`
	variable "AWS_SG_ID" {
		type        = string
		description = "AWS sg-id to run acceptance testing"
	}

	variable "AWS_VPC_ID" {
		type        = string
		description = "AWS VPC ID to run acceptance testing"
	}

	variable "AWS_ZONE_SUBNET_ID" {
		type        = string
		description = "AWS zonal subnet ID to run acceptance testing"
	}

	resource "yba_cloud_provider" "aws" {
		code = "aws"
		name = "%s"
		regions {
			code              = "us-west-2"
			name              = "us-west-2"
		  	security_group_id = var.AWS_SG_ID
		  	vnet_name         = var.AWS_VPC_ID
		  	zones {
				code   = "us-west-2a"
				name   = "us-west-2a"
				subnet = var.AWS_ZONE_SUBNET_ID
		  	}
		}
		air_gap_install = false
	}
`, name)
}

func cloudProviderAzureConfig(name string) string {
	return fmt.Sprintf(`
	variable "AZURE_SUBNET_ID" {
		type        = string
		description = "Azure subnet ID to run acceptance testing"
	}

	variable "AZURE_VNET_ID" {
		type        = string
		description = "Azure vnet ID to run acceptance testing"
	}

	resource "yba_cloud_provider" "azu" {
  		code = "azu"
  		name        = "%s"
  		regions {
    		code = "westus2"
    		name = "westus2"
			vnet_name = var.AZURE_VNET_ID
			zones {
      			name = "westus2-1"
	  			subnet = var.AZURE_SUBNET_ID
			}
  		}
	}
`, name)
}

func TestAccLong_Universe_AWS_VMImageUpgrade(t *testing.T) {
	var universeBefore, universeAfter client.UniverseResp

	rName := acctest.RandomName("aws-universe")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckAWS(t)
			acctest.TestAccPreCheckCloudYBA(t, "AWS")
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse("AWS"),
		Steps: []resource.TestStep{
			{
				Config: universeAwsConfigWithImageBundle(rName,
					"${yba_aws_provider.aws.image_bundles[0].uuid}"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AWS", "yba_universe.aws", &universeBefore),
				),
			},
			{
				Config: universeAwsConfigWithImageBundle(rName,
					"${yba_aws_provider.aws.image_bundles[1].uuid}"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("AWS", "yba_universe.aws", &universeAfter),
					testAccCheckImageBundleUpdated(&universeBefore, &universeAfter),
				),
			},
		},
	})
}

func testAccCheckImageBundleUpdated(before *client.UniverseResp,
	after *client.UniverseResp) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// Validate Primary Cluster (Index 0)
		oldBundleP := before.UniverseDetails.Clusters[0].UserIntent.GetImageBundleUUID()
		newBundleP := after.UniverseDetails.Clusters[0].UserIntent.GetImageBundleUUID()

		if oldBundleP == newBundleP {
			return fmt.Errorf("PRIMARY: expected image_bundle_uuid to change, but both are %s",
				oldBundleP)
		}

		// Validate Async Cluster (Index 1)
		if len(before.UniverseDetails.Clusters) < 2 || len(after.UniverseDetails.Clusters) < 2 {
			return errors.New(
				"universe must have at least 2 clusters (Primary and Async) for this test",
			)
		}

		oldBundleA := before.UniverseDetails.Clusters[1].UserIntent.GetImageBundleUUID()
		newBundleA := after.UniverseDetails.Clusters[1].UserIntent.GetImageBundleUUID()

		if oldBundleA == newBundleA {
			return fmt.Errorf("ASYNC: expected image_bundle_uuid to change, but both are %s",
				oldBundleA)
		}

		if newBundleP == "" || newBundleA == "" {
			return errors.New("image_bundle_uuid is empty after VM Image upgrade")
		}

		return nil
	}
}

func universeAwsConfigWithImageBundle(name string, imageBundleUUID string) string {
	return acctest.YBAProviderBlock("AWS") +
		cloudProviderAWSConfigForVMImageUpgrade(name+"-provider") +
		universeConfigWithProviderWithImageBundle("aws", name, imageBundleUUID)
}

func cloudProviderAWSConfigForVMImageUpgrade(name string) string {
	return fmt.Sprintf(`
	variable "AWS_ACCESS_KEY_ID" {
		type        = string
		description = "AWS access key ID"
	}

	variable "AWS_SECRET_ACCESS_KEY" {
		type        = string
		sensitive   = true
		description = "AWS secret access key"
	}

	variable "AWS_SG_ID" {
		type        = string
		description = "AWS sg-id to run acceptance testing"
	}

	variable "AWS_VPC_ID" {
		type        = string
		description = "AWS VPC ID to run acceptance testing"
	}

	variable "AWS_ZONE_SUBNET_ID" {
		type        = string
		description = "AWS zonal subnet ID to run acceptance testing"
	}

	variable "AWS_AMI_ID_OLD" {
		type        = string
		description = "AMI ID for the first image bundle"
	}

	variable "AWS_AMI_ID_NEW" {
		type        = string
		description = "AMI ID for the second image bundle"
	}

	resource "yba_aws_provider" "aws" {
		name              = "%s"
		access_key_id     = var.AWS_ACCESS_KEY_ID
		secret_access_key = var.AWS_SECRET_ACCESS_KEY
		regions {
			code              = "us-west-2"
			security_group_id = var.AWS_SG_ID
			vpc_id            = var.AWS_VPC_ID
			zones {
				code   = "us-west-2a"
				subnet = var.AWS_ZONE_SUBNET_ID
			}
		}
		image_bundles {
			name           = "test-bundle-old"
			use_as_default = true
			details {
				arch     = "x86_64"
				ssh_user = "ec2-user"
				ssh_port = 22
				region_overrides = {
					"us-west-2" = var.AWS_AMI_ID_OLD
				}
			}
		}
		image_bundles {
			name           = "test-bundle-new"
			use_as_default = false
			details {
				arch     = "x86_64"
				ssh_user = "ec2-user"
				ssh_port = 22
				region_overrides = {
					"us-west-2" = var.AWS_AMI_ID_NEW
				}
			}
		}
	}
`, name)
}

func universeConfigWithProviderWithImageBundle(p string, name string,
	imageBundleUUID string) string {
	return fmt.Sprintf(`
    data "yba_release_version" "release_version"{
        depends_on = [
            yba_aws_provider.%s
        ]
    }

    resource "yba_universe" "%s" {
        clusters {
            cluster_type = "PRIMARY"
            user_intent {
                universe_name      = "%s"
                provider           = yba_aws_provider.%s.id
                region_list        = yba_aws_provider.%s.regions[*].uuid
                num_nodes          = 1
                replication_factor = 1
                instance_type      = "%s"
                image_bundle_uuid  = "%s"
                device_info {
                    num_volumes  = 1
                    volume_size  = 375
                    storage_type = "%s"
                }
                assign_public_ip              = true
                use_time_sync                 = true
                enable_ysql                   = true
                enable_node_to_node_encrypt   = true
                enable_client_to_node_encrypt = true
                yb_software_version           = data.yba_release_version.release_version.id
                access_key_code               = yba_aws_provider.%s.access_key_code
                instance_tags = {
                    "yb_owner"  = "terraform_acctest"
                    "yb_task"   = "dev"
                    "yb_dept"   = "dev"
                }
            }
        }
        # Added ASYNC Cluster Block
        clusters {
            cluster_type = "ASYNC"
            user_intent {
                universe_name      = "%s"
                provider           = yba_aws_provider.%s.id
                region_list        = yba_aws_provider.%s.regions[*].uuid
                num_nodes          = 1
                replication_factor = 1
                instance_type      = "%s"
                image_bundle_uuid  = "%s"
                device_info {
                    num_volumes  = 1
                    volume_size  = 375
                    storage_type = "%s"
                }
                assign_public_ip              = true
                enable_ysql                   = true
                yb_software_version           = data.yba_release_version.release_version.id
                access_key_code               = yba_aws_provider.%s.access_key_code
            }
        }
        communication_ports {}
    }
`,
		// PRIMARY cluster
		p, p, name, p, p, getUniverseInstanceType(p), imageBundleUUID,
		getUniverseStorageType(p), p,
		// ASYNC cluster
		name, p, p, getUniverseInstanceType(p), imageBundleUUID,
		getUniverseStorageType(p), p)
}
