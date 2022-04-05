package universe_test

import (
	"errors"
	"fmt"
	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/acctest"
	"testing"
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
	conn := acctest.YWClient

	for _, r := range s.RootModule().Resources {
		if r.Type == "yb_universe" {
			ctx, cUUID := acctest.GetCtxWithConnectionInfo(r.Primary)
			_, _, err := conn.UniverseManagementApi.GetUniverse(ctx, cUUID, r.Primary.ID).Execute()
			if err == nil || acctest.IsResourceNotFoundError(err) {
				return errors.New("universe resource is not destroyed")
			}
		} else if r.Type == "yb_cloud_provider" {
			ctx, cUUID := acctest.GetCtxWithConnectionInfo(r.Primary)
			res, _, err := conn.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
			if err != nil {
				return err
			}
			for _, p := range res {
				if *p.Uuid == r.Primary.ID {
					return errors.New("cloud provider is not destroyed")
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

		conn := acctest.YWClient
		ctx, cUUID := acctest.GetCtxWithConnectionInfo(r.Primary)
		res, _, err := conn.UniverseManagementApi.GetUniverse(ctx, cUUID, r.Primary.ID).Execute()
		if err != nil {
			return err
		}
		*universe = res
		return nil
	}
}

func testAccCheckNumNodes(universe *client.UniverseResp, expected int32) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		found := universe.UniverseDetails.Clusters[0].UserIntent.GetNumNodes()
		if found != expected {
			return errors.New(fmt.Sprintf("expected %d nodes; found %d", expected, found))
		}
		return nil
	}
}

func universeGcpConfigWithNodes(name string, nodes int) string {
	return cloudProviderGCPConfig(fmt.Sprintf(name+"-provider")) + universeConfigWithProviderWithNodes("gcp", name, nodes)
}

func universeAwsConfigWithNodes(name string, nodes int) string {
	return cloudProviderAWSConfig(fmt.Sprintf(name+"-provider")) + universeConfigWithProviderWithNodes("aws", name, nodes)
}

func universeAzureConfigWithNodes(name string, nodes int) string {
	return cloudProviderAzureConfig(fmt.Sprintf(name+"-provider")) + universeConfigWithProviderWithNodes("azu", name, nodes)
}

func universeConfigWithProviderWithNodes(p string, name string, nodes int) string {
	return fmt.Sprintf(`
data "yb_provider_key" "%s_key" {
  connection_info {
   	cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

  provider_id = yb_cloud_provider.%s.id
}

resource "yb_universe" "%s" {
  connection_info {
    cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

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
      yb_software_version           = "%s"
      access_key_code               = data.yb_provider_key.%s_key.id
    }
  }
  communication_ports {}
}
`, p, p, p, name, p, p, p, nodes, getUniverseInstanceType(p), getUniverseStorageType(p), acctest.TestYBSoftwareVersion(), p)
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
		return "c5.xlarge"
	}
	return "Standard_D4s_v3"
}

func cloudProviderGCPConfig(name string) string {
	return fmt.Sprintf(`
data "yb_customer_data" "customer" {
  api_token = "%s"
}

resource "yb_cloud_provider" "gcp" {
  connection_info {
    cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

  code = "gcp"
  config = merge(
    { YB_FIREWALL_TAGS = "cluster-server" },
    jsondecode(file("%s"))
  )
  dest_vpc_id = "default"
  name        = "%s"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 54422
  air_gap_install = false
}
`, acctest.TestApiKey(), acctest.TestGCPCredentials(), name)
}

func cloudProviderAWSConfig(name string) string {
	return fmt.Sprintf(`
data "yb_customer_data" "customer" {
  api_token = "%s"
}

resource "yb_cloud_provider" "aws" {
  connection_info {
    cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

  code = "aws"
  config = { 
	AWS_ACCESS_KEY_ID = "%s"
	AWS_SECRET_ACCESS_KEY = "%s"
  }
  name        = "%s"
  regions {
	security_group_id = "sg-019f83043a270f3bb"
	vnet_name = "vpc-09eea1b4c18fb9ba0"
    code = "us-east-1"
    name = "us-east-1"
	zones {
	  name = "us-east-1a"
	  subnet = "subnet-0cdb90ad5eaa47ed9"
	}
}
`, acctest.TestApiKey(), acctest.TestAWSAccessKey(), acctest.TestAWSSecretAccessKey(), name)
}

// TODO: there should be 3 zones here but https://yugabyte.atlassian.net/browse/PLAT-3034 needs to be resolved first
func cloudProviderAzureConfig(name string) string {
	return fmt.Sprintf(`
data "yb_customer_data" "customer" {
  api_token = "%s"
}

resource "yb_cloud_provider" "azu" {
  connection_info {
    cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

  code = "azu"
  config = { 
	AZURE_SUBSCRIPTION_ID = "%s"
	AZURE_RG = "%s"
	AZURE_TENANT_ID = "%s"
	AZURE_CLIENT_ID = "%s"
	AZURE_CLIENT_SECRET = "%s"
  }
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
`,
		acctest.TestApiKey(),
		acctest.TestAzureSubscriptionID(),
		acctest.TestAzureResourceGroup(),
		acctest.TestAzureTenantID(),
		acctest.TestAzureClientID(),
		acctest.TestAzureClientSecret(),
		name)
}
