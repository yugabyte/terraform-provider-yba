package cloud_provider_test

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

func TestAccCloudProvider_GCP(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-gcp-provider-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyCloudProvider,
		Steps: []resource.TestStep{
			{
				Config: cloudProviderGCPConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCloudProviderExists("yb_cloud_provider.gcp", &provider),
				),
			},
		},
	})
}

func TestAccCloudProvider_AWS(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-aws-provider-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyCloudProvider,
		Steps: []resource.TestStep{
			{
				Config: cloudProviderAWSConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCloudProviderExists("yb_cloud_provider.aws", &provider),
				),
			},
		},
	})
}

func TestAccCloudProvider_Azure(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-azure-provider-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyCloudProvider,
		Steps: []resource.TestStep{
			{
				Config: cloudProviderAzureConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCloudProviderExists("yb_cloud_provider.azure", &provider),
				),
			},
		},
	})
}

func testAccCheckDestroyCloudProvider(s *terraform.State) error {
	conn := acctest.YWClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yb_cloud_provider" {
			continue
		}

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

	return nil
}

func testAccCheckCloudProviderExists(name string, provider *client.Provider) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for cloud provider resource")
		}

		conn := acctest.YWClient
		ctx, cUUID := acctest.GetCtxWithConnectionInfo(r.Primary)
		res, _, err := conn.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
		if err != nil {
			return err
		}
		for _, p := range res {
			if *p.Uuid == r.Primary.ID {
				*provider = p
				return nil
			}
		}
		return errors.New("cloud provider does not exist")
	}
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
  dest_vpc_id = "yugabyte-network"
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
