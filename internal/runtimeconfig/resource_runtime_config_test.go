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
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// TestAccRuntimeConfig_GlobalScope exercises the full lifecycle of a
// yba_runtime_config resource on the global scope using the Boolean key
// (boolCase): create, in-place update, import, and the delete path (which resets
// the key to its YBA-side default).
func TestAccRuntimeConfig_GlobalScope(t *testing.T) {
	resourceName := "yba_runtime_config.test"
	id := globalScope + "/" + boolCase.key

	// Serial (not Parallel): runtime config keys are shared singletons on the
	// standing YBA, so concurrent tests touching the same key would collide.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy: testAccCheckRuntimeConfigValueAbsent(
			globalScope, boolCase.key, boolCase.value),
		Steps: []resource.TestStep{
			{
				// Create at the default value to exercise the create/set path.
				Config: runtimeConfigConfig(boolCase.key, "false"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckRuntimeConfigValue(globalScope, boolCase.key, "false"),
					resource.TestCheckResourceAttr(resourceName, "key", boolCase.key),
					resource.TestCheckResourceAttr(resourceName, "value", "false"),
					resource.TestCheckResourceAttr(resourceName, "scope", globalScope),
					resource.TestCheckResourceAttr(resourceName, "id", id),
				),
			},
			{
				// Update in place to a non-default value; verifies the update path
				// and leaves an override for the destroy step to reset.
				Config: runtimeConfigConfig(boolCase.key, boolCase.value),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckRuntimeConfigValue(globalScope, boolCase.key, boolCase.value),
					resource.TestCheckResourceAttr(resourceName, "value", boolCase.value),
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

// TestAccRuntimeConfig_TypeRoundTrip verifies that values backed by several YBA
// data types round-trip without drift through a resource whose `value` is a
// plain string. YBA stores values verbatim and returns them byte-for-byte, so:
//   - the API reports back exactly the string we set (no type normalization),
//   - re-applying the identical config is a no-op (PlanOnly step), and
//   - import round-trips the state.
//
// It iterates the shared runtimeConfigCases table (see common_test.go), so it
// covers the same Boolean/Duration/Integer/String keys as the data source test.
// Subtests run serially (no t.Parallel): each key is a shared global singleton.
func TestAccRuntimeConfig_TypeRoundTrip(t *testing.T) {
	for _, tc := range runtimeConfigCases {
		t.Run(tc.name, func(t *testing.T) { testRuntimeConfigTypeRoundTrip(t, tc) })
	}
}

func testRuntimeConfigTypeRoundTrip(t *testing.T, tc runtimeConfigCase) {
	resourceName := "yba_runtime_config.test"
	id := globalScope + "/" + tc.key

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckRuntimeConfigValueAbsent(globalScope, tc.key, tc.value),
		Steps: []resource.TestStep{
			{
				Config: runtimeConfigConfig(tc.key, tc.value),
				Check: resource.ComposeTestCheckFunc(
					// YBA returns the value byte-for-byte: no type normalization.
					testAccCheckRuntimeConfigValue(globalScope, tc.key, tc.value),
					resource.TestCheckResourceAttr(resourceName, "value", tc.value),
					resource.TestCheckResourceAttr(resourceName, "id", id),
				),
			},
			{
				// Re-applying the identical config must be a no-op: the string
				// value round-trips with zero drift for every data type.
				Config:   runtimeConfigConfig(tc.key, tc.value),
				PlanOnly: true,
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
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
				Config:      runtimeConfigConfigWithScope(bogusScope, boolCase.key, boolCase.value),
				ExpectError: regexp.MustCompile(`Cannot set the key in this scope`),
			},
		},
	})
}

// TestAccRuntimeConfig_MalformedValue verifies that when a value does not parse
// for the key's data type, the apply fails surfacing YBA's validation error
// rather than silently storing garbage. boolCase.key is a Boolean key, so
// "notabool" cannot be parsed; YBA enables value validation by default
// (runtime_config.data_validation.enabled) and returns 400 with
// "<value> is not a valid value for desired key".
func TestAccRuntimeConfig_MalformedValue(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      runtimeConfigConfig(boolCase.key, "notabool"),
				ExpectError: regexp.MustCompile(`is not a valid value for desired key`),
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
