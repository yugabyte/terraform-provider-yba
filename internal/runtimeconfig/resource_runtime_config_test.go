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

package runtimeconfig_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const (
	// globalScope is the well-known UUID YBA uses for global-scope runtime config.
	globalScope = "00000000-0000-0000-0000-000000000000"

	// testKey is a GLOBAL-scope boolean runtime config key that is safe to flip
	// and reset on the shared standing YBA: it only guards whether platform
	// downgrades are permitted (never consulted outside an upgrade/downgrade
	// flow) and defaults to "false", so resetting it on destroy restores the
	// fixture's prior state.
	testKey = "yb.is_platform_downgrade_allowed"

	// overrideValue is a non-default value the test applies so that the
	// destroy-resets-to-default behavior is observable.
	overrideValue = "true"
)

// TestAccRuntimeConfig_GlobalScope exercises the full lifecycle of a
// yba_runtime_config resource on the global scope: create, in-place update,
// import, and the delete path (which resets the key to its YBA-side default).
func TestAccRuntimeConfig_GlobalScope(t *testing.T) {
	resourceName := "yba_runtime_config.test"
	id := globalScope + "/" + testKey

	// Serial (not Parallel): runtime config keys are shared singletons on the
	// standing YBA, so concurrent tests touching the same key would collide.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckRuntimeConfigReset,
		Steps: []resource.TestStep{
			{
				// Create at the default value to exercise the create/set path.
				Config: runtimeConfigConfig(testKey, "false"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckRuntimeConfigValue(globalScope, testKey, "false"),
					resource.TestCheckResourceAttr(resourceName, "key", testKey),
					resource.TestCheckResourceAttr(resourceName, "value", "false"),
					resource.TestCheckResourceAttr(resourceName, "scope", globalScope),
					resource.TestCheckResourceAttr(resourceName, "id", id),
				),
			},
			{
				// Update in place to a non-default value; verifies the update path
				// and leaves an override for the destroy step to reset.
				Config: runtimeConfigConfig(testKey, overrideValue),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckRuntimeConfigValue(globalScope, testKey, overrideValue),
					resource.TestCheckResourceAttr(resourceName, "value", overrideValue),
				),
			},
			{
				// Import using the "<scope>/<key>" ID and confirm state round-trips.
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccCheckRuntimeConfigValue asserts YBA reports the expected value for a
// runtime config key, reading it back through the API independently of state.
func testAccCheckRuntimeConfigValue(scope, key, expected string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := acctest.APIClient.YugawareClient
		cUUID := acctest.APIClient.CustomerID

		value, response, err := c.RuntimeConfigurationAPI.
			GetConfigurationKey(context.Background(), cUUID, scope, key).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Runtime Config", "Read")
		}
		if value != expected {
			return fmt.Errorf("runtime config key %q in scope %q = %q, want %q",
				key, scope, value, expected)
		}
		return nil
	}
}

// testAccCheckRuntimeConfigReset verifies that destroy reset every managed key
// to its YBA-side default: the non-default override the test applied must no
// longer be present.
func testAccCheckRuntimeConfigReset(s *terraform.State) error {
	c := acctest.APIClient.YugawareClient
	cUUID := acctest.APIClient.CustomerID

	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_runtime_config" {
			continue
		}
		scope := r.Primary.Attributes["scope"]
		key := r.Primary.Attributes["key"]

		value, response, err := c.RuntimeConfigurationAPI.
			GetConfigurationKey(context.Background(), cUUID, scope, key).Execute()
		if err != nil {
			// A 404 means the override is gone entirely, a valid post-destroy state.
			if acctest.IsResourceNotFoundError(err) {
				continue
			}
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Runtime Config", "Read")
		}
		if value == overrideValue {
			return fmt.Errorf(
				"runtime config key %q in scope %q still has overridden value %q after destroy",
				key, scope, value)
		}
	}
	return nil
}

// TestAccRuntimeConfig_NonMutableKey verifies the resource fails the apply
// (rather than silently creating a broken resource) when the key is not a
// mutable runtime config key on the target YBA. YBA returns 404
// "No mutable key found" for both unknown keys and keys that exist but are not
// runtime-mutable, so an obviously-bogus key gives a version-independent signal.
func TestAccRuntimeConfig_NonMutableKey(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      runtimeConfigConfig("yb.terraform.acctest.nonexistent_key", "true"),
				ExpectError: regexp.MustCompile(`No mutable key found`),
			},
		},
	})
}

// TestAccRuntimeConfig_InvalidScope verifies the apply fails when the key is
// real and mutable but the chosen scope cannot hold it. YBA rejects the set with
// 400 "Cannot set the key in this scope".
func TestAccRuntimeConfig_InvalidScope(t *testing.T) {
	const bogusScope = "99999999-9999-9999-9999-999999999999"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      runtimeConfigConfigWithScope(bogusScope, testKey, "true"),
				ExpectError: regexp.MustCompile(`Cannot set the key in this scope`),
			},
		},
	})
}

func runtimeConfigConfig(key, value string) string {
	return fmt.Sprintf(`
resource "yba_runtime_config" "test" {
  key   = %q
  value = %q
}
`, key, value)
}

func runtimeConfigConfigWithScope(scope, key, value string) string {
	return fmt.Sprintf(`
resource "yba_runtime_config" "test" {
  scope = %q
  key   = %q
  value = %q
}
`, scope, key, value)
}
