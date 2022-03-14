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
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyUniverse,
		Steps: []resource.TestStep{
			{
				Config: universeGcpConfigWithNodes(rName, 3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_cloud_provider.gcp", &universe),
					testAccCheckNumNodes(&universe, 3),
				),
			},
			{
				Config: universeGcpConfigWithNodes(rName, 4),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUniverseExists("yb_cloud_provider.gcp", &universe),
					testAccCheckNumNodes(&universe, 4),
				),
			},
		},
	})
}

func testAccCheckDestroyUniverse(s *terraform.State) error {
	conn := acctest.YWClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yb_universe" {
			continue
		}

		ctx, cUUID := acctest.GetCtxWithConnectionInfo(r.Primary)
		_, _, err := conn.UniverseManagementApi.GetUniverse(ctx, cUUID, r.Primary.ID).Execute()
		if err == nil || acctest.IsResourceNotFoundError(err) {
			return errors.New("universe resource is not destroyed")
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
	return universeConfigWithProviderWithNodes("gcp", name, nodes)
}

func universeAwsConfigWithNodes(name string, nodes int) string {
	return universeConfigWithProviderWithNodes("aws", name, nodes)
}

func universeAzureConfigWithNodes(name string, nodes int) string {
	return universeConfigWithProviderWithNodes("azure", name, nodes)
}

func universeConfigWithProviderWithNodes(p string, name string, nodes int) string {
	return cloudProviderGCPConfig(fmt.Sprintf("tf-acctest-gcp-universe-provider-%s", sdkacctest.RandString(12))) +
		fmt.Sprintf(`
data "yb_provider_key" "%s-key" {
  connection_info {
    cuuid     = yb_customer_resource.customer.cuuid
    api_token = yb_customer_resource.customer.api_token
  }

  provider_id = yb_cloud_provider.%s.id
}

resource "yb_universe" "gcp_universe" {
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
      region_list        = data.yb_provider_key.%s-key.id
      num_nodes          = %d
      replication_factor = 3
      instance_type      = "n1-standard-1"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "Persistent"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = "%s"
      access_key_code               = local.provider_key
    }
  }
  communication_ports {}
}
`, p, p, name, p, p, p, nodes, acctest.TestYBSoftwareVersion())

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
  dest_vpc_id = "***REMOVED***"
  name        = "%s"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 54422
  air_gap_install = false
}
`, acctest.TestApiKey(), acctest.TestGCPConfig(), name)
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
	YB_FIREWALL_TAGS = "cluster-server" 
	AWS_ACCESS_KEY_ID = "%s"
	AWS_SECRET_ACCESS_KEY = "%s"
  }
  name        = "%s"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
}
`, acctest.TestApiKey(), acctest.TestAWSAccessKey(), acctest.TestAWSSecretAccessKey(), name)
}

func cloudProviderAzureConfig(name string) string {
	return fmt.Sprintf(`
data "yb_customer_data" "customer" {
  api_token = "%s"
}

resource "yb_cloud_provider" "azure" {
  connection_info {
    cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

  code = "azure"
  config = { 
	YB_FIREWALL_TAGS = "cluster-server" 
	SUBSCRIPTION_ID = "%s"
	RESOURCE_GROUP = "%s"
	TENANT_ID = "%s"
	CLIENT_ID = "%s"
	CLIENT_SECRET = "%s"
  }
  name        = "%s"
  regions {
    code = "us-west1"
    name = "us-west1"
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
