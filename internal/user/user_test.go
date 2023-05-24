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

package user_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
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
				Config: userConfigWithRole("Admin", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists("yba_user.user", &user),
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
				Config: userConfigWithRole("ReadOnly", rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists("yba_user.user", &user),
				),
			},
		},
	})
}

func testAccCheckDestroyUser(s *terraform.State) error {
	conn := acctest.APIClient.YugawareClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_user" {
			continue
		}

		cUUID := acctest.APIClient.CustomerID
		_, _, err := conn.UserManagementApi.GetUserDetails(context.Background(), cUUID,
			r.Primary.ID).Execute()
		if err == nil || acctest.IsResourceNotFoundError(err) {
			return errors.New("User resource is not destroyed")
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

		conn := acctest.APIClient.YugawareClient
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.UserManagementApi.GetUserDetails(context.Background(), cUUID,
			r.Primary.ID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"User", "Read")
			return errMessage
		}
		*user = res
		return nil
	}
}

func userConfigWithRole(role string, name string) string {
	return fmt.Sprintf(`

resource "yba_user" "user" {
  email = "%s@yugabyte.com"
  password = "Password1@"
  role = "%s"
}
`, name, role)
}
