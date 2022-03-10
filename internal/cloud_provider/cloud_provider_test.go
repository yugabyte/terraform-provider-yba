package cloud_provider_test

import (
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

	rName := fmt.Sprintf("tf-acctest-provider-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.TestAccProviders,
		CheckDestroy:      testAccCheckDestroyCloudProvider,
		Steps: []resource.TestStep{
			{
				Config: cloudProviderGCPConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCloudProviderGCPExists("example_widget.foo", &provider),
				),
			},
		},
	})
}

func testAccCheckDestroyCloudProvider(*terraform.State) error {
	return nil
}

func cloudProviderGCPConfig(name string) string {
	return ""
}

func testAccCheckCloudProviderGCPExists(name string, provider *client.Provider) resource.TestCheckFunc {
	return nil
}
