package user_test

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

func TestAccUser_Admin(t *testing.T) {
	var user client.UserWithFeatures

	rName := fmt.Sprintf("tf-acctest-admin-user-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyUser,
		Steps: []resource.TestStep{
			{
				Config: userConfigWithRole("ADMIN", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists("yb_user.user", &user),
				),
			},
		},
	})
}

func TestAccUser_ReadOnly(t *testing.T) {
	var user client.UserWithFeatures

	rName := fmt.Sprintf("tf-acctest-admin-user-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyUser,
		Steps: []resource.TestStep{
			{
				Config: userConfigWithRole("READONLY", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists("yb_user.user", &user),
				),
			},
		},
	})
}

func testAccCheckDestroyUser(s *terraform.State) error {
	conn := acctest.YWClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yb_user" {
			continue
		}

		ctx, cUUID := acctest.GetCtxWithConnectionInfo(r.Primary)
		_, _, err := conn.UserManagementApi.GetUserDetails(ctx, cUUID, r.Primary.ID).Execute()
		if err == nil || acctest.IsResourceNotFoundError(err) {
			return errors.New("user resource is not destroyed")
		}
	}

	return nil
}

func testAccCheckUserExists(name string, user *client.UserWithFeatures) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for user resource")
		}

		conn := acctest.YWClient
		ctx, cUUID := acctest.GetCtxWithConnectionInfo(r.Primary)
		res, _, err := conn.UserManagementApi.GetUserDetails(ctx, cUUID, r.Primary.ID).Execute()
		if err != nil {
			return err
		}
		*user = res
		return nil
	}
}

func userConfigWithRole(role string, name string) string {
	return fmt.Sprintf(`
data "yb_customer_data" "customer" {
  api_token = "%s"
}

resource "yb_user" "user" {
  connection_info {
    cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

  email = "%s@yugabyte.com"
  password = "Password1@"
  role = "%s"
  is_primary = false
}
`, acctest.TestApiKey(), name, role)
}
