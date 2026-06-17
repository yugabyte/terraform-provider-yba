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

// TestAccRuntimeConfigDataSource sets a key with the resource, then reads it
// back through the data source and asserts the value round-trips. It iterates
// the shared runtimeConfigCases table (see common_test.go) — the same
// Boolean/Duration/Integer/String keys the resource round-trip test uses — to
// show the data source returns the value as a plain string regardless of the
// key's underlying type, which is what lets a configuration consume a key
// managed (or set) elsewhere.
//
// Subtests run serially (no t.Parallel): each key is a shared global singleton,
// also touched by the other runtime config tests.
func TestAccRuntimeConfigDataSource(t *testing.T) {
	for _, tc := range runtimeConfigCases {
		t.Run(tc.name, func(t *testing.T) { testRuntimeConfigDataSourceRoundTrip(t, tc) })
	}
}

func testRuntimeConfigDataSourceRoundTrip(t *testing.T, tc runtimeConfigCase) {
	dataSourceName := "data.yba_runtime_config.test"
	id := globalScope + "/" + tc.key

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckRuntimeConfigValueAbsent(globalScope, tc.key, tc.value),
		Steps: []resource.TestStep{
			{
				Config: runtimeConfigDataSourceConfig(tc.key, tc.value),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "key", tc.key),
					resource.TestCheckResourceAttr(dataSourceName, "scope", globalScope),
					resource.TestCheckResourceAttr(dataSourceName, "value", tc.value),
					resource.TestCheckResourceAttr(dataSourceName, "id", id),
				),
			},
		},
	})
}

// TestAccRuntimeConfigDataSource_UnknownKey verifies the data source surfaces
// YBA's error verbatim when asked to read a key that is not a mutable runtime
// config key. YBA returns 404 "No mutable key found: <key>".
func TestAccRuntimeConfigDataSource_UnknownKey(t *testing.T) {
	const unknownKey = "yb.terraform.acctest.nonexistent_key"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      runtimeConfigDataSourceReadConfig(unknownKey),
				ExpectError: regexp.MustCompile(`No mutable key found`),
			},
		},
	})
}

// runtimeConfigDataSourceConfig manages the key with the resource and reads it
// back with the data source. depends_on forces the data source read to happen
// after the resource has set the value, so the assertion sees the live value.
func runtimeConfigDataSourceConfig(key, value string) string {
	return fmt.Sprintf(`
resource "yba_runtime_config" "test" {
  key   = %q
  value = %q
}

data "yba_runtime_config" "test" {
  key        = yba_runtime_config.test.key
  depends_on = [yba_runtime_config.test]
}
`, key, value)
}

// runtimeConfigDataSourceReadConfig reads a key with the data source alone (no
// managed resource), used to exercise the read error path.
func runtimeConfigDataSourceReadConfig(key string) string {
	return fmt.Sprintf(`
data "yba_runtime_config" "test" {
  key = %q
}
`, key)
}
