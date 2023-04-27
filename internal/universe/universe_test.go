package universe_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/acctest"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

func TestAccUniverse_GCP_UpdatePrimaryNodes(t *testing.T) {
	var universe client.UniverseResp

	rName := fmt.Sprintf("tf-acctest-gcp-universe-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse,
		Steps: []resource.TestStep{
			{
				Config: universeGcpConfigWithNodes(rName, 3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_universe.gcp", &universe),
					testAccCheckNumNodes(&universe, 3),
				),
			},
			{
				Config: universeGcpConfigWithNodes(rName, 4),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_universe.gcp", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
			},
		},
	})
}

func TestAccUniverse_AWS_UpdatePrimaryNodes(t *testing.T) {
	var universe client.UniverseResp

	rName := fmt.Sprintf("tf-acctest-aws-universe-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAWS(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse,
		Steps: []resource.TestStep{
			{
				Config: universeAwsConfigWithNodes(rName, 3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_universe.aws", &universe),
					testAccCheckNumNodes(&universe, 3),
				),
			},
			{
				Config: universeAwsConfigWithNodes(rName, 4),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_universe.aws", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
			},
		},
	})
}

func TestAccUniverse_Azure_UpdatePrimaryNodes(t *testing.T) {
	var universe client.UniverseResp

	rName := fmt.Sprintf("tf-acctest-azu-universe-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAzure(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyProviderAndUniverse,
		Steps: []resource.TestStep{
			{
				Config: universeAzureConfigWithNodes(rName, 3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_universe.azu", &universe),
					testAccCheckNumNodes(&universe, 3),
				),
			},
			{
				Config: universeAzureConfigWithNodes(rName, 4),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_universe.azu", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
			},
		},
	})
}

func testAccCheckDestroyProviderAndUniverse(s *terraform.State) error {
	conn := acctest.APIClient.YugawareClient

	for _, r := range s.RootModule().Resources {
		if r.Type == "yb_universe" {
			cUUID := acctest.APIClient.CustomerID
			_, _, err := conn.UniverseManagementApi.GetUniverse(context.Background(), cUUID,
				r.Primary.ID).Execute()
			if err == nil || acctest.IsResourceNotFoundError(err) {
				return errors.New("Universe resource is not destroyed")
			}
		} else if r.Type == "yb_cloud_provider" {
			time.Sleep(60 * time.Second)
			cUUID := acctest.APIClient.CustomerID
			res, response, err := conn.CloudProvidersApi.GetListOfProviders(context.Background(),
				cUUID).Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
					"Universe", "Read - Cloud Provider")
				return errMessage
			}
			for _, p := range res {
				if *p.Uuid == r.Primary.ID {
					return errors.New("Cloud provider is not destroyed")
				}
			}
		}
	}

	return nil
}

func testAccCheckUniverseExists(name string, universe *client.UniverseResp) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for universe resource")
		}

		conn := acctest.APIClient.YugawareClient
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.UniverseManagementApi.GetUniverse(context.Background(), cUUID,
			r.Primary.ID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Universe", "Read - Universe")
			return errMessage
		}
		*universe = res
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
	return cloudProviderGCPConfig(fmt.Sprintf(name+"-provider")) +
		universeConfigWithProviderWithNodes("gcp", name, nodes)
}

func universeAwsConfigWithNodes(name string, nodes int) string {
	return cloudProviderAWSConfig(fmt.Sprintf(name+"-provider")) +
		universeConfigWithProviderWithNodes("aws", name, nodes)
}

func universeAzureConfigWithNodes(name string, nodes int) string {
	return cloudProviderAzureConfig(fmt.Sprintf(name+"-provider")) +
		universeConfigWithProviderWithNodes("azu", name, nodes)
}

func universeConfigWithProviderWithNodes(p string, name string, nodes int) string {
	return fmt.Sprintf(`
data "yb_provider_key" "%s_key" {
  provider_id = yb_cloud_provider.%s.id
}

data "yb_release_version" "release_version"{
	depends_on = [
		data.yb_provider_key.%s_key
  ]
}

resource "yb_universe" "%s" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "%s"
      provider_type      = "%s"
      provider           = yb_cloud_provider.%s.id
      region_list        = yb_cloud_provider.%s.regions[*].uuid
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
      yb_software_version           = data.yb_release_version.release_version.id
      access_key_code               = data.yb_provider_key.%s_key.id
	  instance_tags = {
        "yb_owner"  = "terraform_acctest"
        "yb_task"   = "dev"
        "yb_dept"   = "dev"
      }
    }
  }
  communication_ports {}
}
`, p, p, p,p, name, p, p, p, nodes, getUniverseInstanceType(p),
		getUniverseStorageType(p), p)
}

func getUniverseStorageType(p string) string {
	if p == "gcp" {
		return "Persistent"
	} else if p == "aws" {
		return "GP2"
	}
	return "Premium_LRS"
}

func getUniverseInstanceType(p string) string {
	if p == "gcp" {
		return "n1-standard-1"
	} else if p == "aws" {
		return "c5.large"
	}
	return "Standard_D4s_v3"
}

func cloudProviderGCPConfig(name string) string {
	return fmt.Sprintf(`
resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  dest_vpc_id = "***REMOVED***"
  name        = "%s"
  regions {
    code = "us-west2"
    name = "us-west2"
  }
  ssh_port        = 22
  air_gap_install = false
}
`, name)
}

func cloudProviderAWSConfig(name string) string {
	// TODO: remove the lifecycle ignore_changes block. This is needed because the current API
	// is not returning vnet_name
	return fmt.Sprintf(`
	resource "yb_cloud_provider" "aws" {
		code = "aws"
		name = "%s"
		regions {
		  code              = "us-west-2"
		  name              = "us-west-2"
		  security_group_id = "***REMOVED***"
		  vnet_name         = "***REMOVED***"
		  zones {
			code   = "us-west-2a"
			name   = "us-west-2a"
			subnet = "***REMOVED***"
		  }
		}
		air_gap_install = false
	  }
`, name)
}

func cloudProviderAzureConfig(name string) string {
	return fmt.Sprintf(`
resource "yb_cloud_provider" "azu" {
  code = "azu"
  name        = "%s"
  regions {
    code = "westus2"
    name = "westus2"
	vnet_name = "***REMOVED***"
	zones {
      name = "westus2-1"
	  subnet = "***REMOVED***"
	}
  }
}
`, name)
}
