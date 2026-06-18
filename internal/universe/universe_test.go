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
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestAccLong_Universe_GCP_UpdatePrimaryNodes(t *testing.T) {
	var universe client.UniverseResp

	rName := acctest.RandomName("gcp-universe")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckGCP(t)
			acctest.TestAccPreCheckCloudYBA(t, "GCP")
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse("GCP"),
		Steps: []resource.TestStep{
			{
				Config: universeGcpConfigWithNodes(rName, 3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckNumNodes(&universe, 3),
				),
			},
			{
				Config: universeGcpConfigWithNodes(rName, 4),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("GCP", "yba_universe.gcp", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
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

func universeConfigWithProviderWithNodes(p string, name string, nodes int) string {
	return fmt.Sprintf(`
	data "yba_provider_key" "%s_key" {
  		provider_id = yba_cloud_provider.%s.id
	}

	data "yba_release_version" "release_version"{
		depends_on = [
			data.yba_provider_key.%s_key
  		]
	}

	resource "yba_universe" "%s" {
  		clusters {
    		cluster_type = "PRIMARY"
    		user_intent {
      			universe_name      = "%s"
      			provider           = yba_cloud_provider.%s.id
      			region_list        = yba_cloud_provider.%s.regions[*].uuid
      			num_nodes          = %d
      			replication_factor = 3
      			instance_type      = "%s"
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
				access_key_code               = data.yba_provider_key.%s_key.id
				instance_tags = {
					"yb_owner"  = "terraform_acctest"
					"yb_task"   = "dev"
					"yb_dept"   = "dev"
				}
    		}
  		}
  		communication_ports {}
	}
`, p, p, p, p, name, p, p, nodes, getUniverseInstanceType(p),
		getUniverseStorageType(p), p)
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
