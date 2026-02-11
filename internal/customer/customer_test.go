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

package customer_test

import (
	"fmt"
	"testing"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// TestAccCustomer_Basic tests basic customer creation and read
// Note: This test requires a fresh YBA installation without any registered customer
func TestAccCustomer_Basic(t *testing.T) {
	// This test can only run once on a fresh YBA instance
	// After first run, the customer already exists
	t.Skip("Customer acceptance tests require a fresh YBA instance - run manually")

	rName := fmt.Sprintf("tf-acctest-customer-%s", sdkacctest.RandString(8))
	rEmail := fmt.Sprintf("test-%s@yugabyte.com", sdkacctest.RandString(8))
	rPassword := "TestPassword123!"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: customerConfig(rName, rEmail, "dev", rPassword),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCustomerExists("yba_customer.test"),
					resource.TestCheckResourceAttr("yba_customer.test", "name", rName),
					resource.TestCheckResourceAttr("yba_customer.test", "email", rEmail),
					resource.TestCheckResourceAttr("yba_customer.test", "code", "dev"),
					resource.TestCheckResourceAttrSet("yba_customer.test", "cuuid"),
					resource.TestCheckResourceAttrSet("yba_customer.test", "api_token"),
				),
			},
		},
	})
}

// TestAccCustomer_CodeVariations tests different code values
func TestAccCustomer_CodeVariations(t *testing.T) {
	t.Skip("Customer acceptance tests require a fresh YBA instance - run manually")

	codes := []string{"dev", "demo", "stage", "prod"}

	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			rName := fmt.Sprintf("tf-acctest-%s-%s", code, sdkacctest.RandString(6))
			rEmail := fmt.Sprintf("%s-%s@yugabyte.com", code, sdkacctest.RandString(6))
			rPassword := "TestPassword123!"

			resource.Test(t, resource.TestCase{
				PreCheck:          func() { acctest.TestAccPreCheck(t) },
				ProviderFactories: acctest.ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: customerConfig(rName, rEmail, code, rPassword),
						Check: resource.ComposeTestCheckFunc(
							testAccCheckCustomerExists("yba_customer.test"),
							resource.TestCheckResourceAttr("yba_customer.test", "code", code),
						),
					},
				},
			})
		})
	}
}

func testAccCheckCustomerExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return fmt.Errorf("no ID is set for customer resource")
		}
		if r.Primary.Attributes["cuuid"] == "" {
			return fmt.Errorf("cuuid not set")
		}
		if r.Primary.Attributes["api_token"] == "" {
			return fmt.Errorf("api_token not set")
		}
		return nil
	}
}

func customerConfig(name, email, code, password string) string {
	return fmt.Sprintf(`
resource "yba_customer" "test" {
  name     = "%s"
  email    = "%s"
  code     = "%s"
  password = "%s"
}
`, name, email, code, password)
}

func customerConfigDefaultCode(name, email, password string) string {
	return fmt.Sprintf(`
resource "yba_customer" "test" {
  name     = "%s"
  email    = "%s"
  password = "%s"
}
`, name, email, password)
}
