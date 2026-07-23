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

// Fixtures and check helpers shared by the runtime config acceptance tests in
// this package — the resource tests (resource_runtime_config_test.go) and the
// data source tests (data_source_runtime_config_test.go).
package runtimeconfig_test

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// YBA invalidates its runtime-config cache asynchronously, so a read right
// after a write/delete can be stale — observed as a CI flake where the destroy
// check saw the old value while YBA was under universe-task load. Poll briefly
// before failing.
const runtimeConfigSettle = 30 * time.Second

func retryRuntimeConfigCheck(check func() error) error {
	deadline := time.Now().Add(runtimeConfigSettle)
	for {
		err := check()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(2 * time.Second)
	}
}

// globalScope is the well-known UUID YBA uses for global-scope runtime config.
const globalScope = "00000000-0000-0000-0000-000000000000"

// runtimeConfigCase is one (key, value) fixture exercised against the standing
// YBA. Each key is a GLOBAL-scope, mutable runtime config key that is safe to
// flip briefly and that the resource's delete path resets to its YBA default.
type runtimeConfigCase struct {
	name  string // subtest name; also the YBA data type under test
	key   string
	value string // a valid, non-default value for this key's data type
}

// runtimeConfigCases is the single shared table of value round-trip fixtures. It
// is iterated identically by both round-trip tests:
//   - TestAccRuntimeConfig_TypeRoundTrip (resource_runtime_config_test.go)
//   - TestAccRuntimeConfigDataSource     (data_source_runtime_config_test.go)
//
// It spans several YBA data types (Boolean, Duration, Integer, String) to prove
// that a value round-trips byte-for-byte through the string-typed `value`, so
// the data type never drives plan drift. Every value above is deliberately
// different from the key's YBA default, so the destroy check
// (testAccCheckRuntimeConfigValueAbsent) is meaningful.
var runtimeConfigCases = []runtimeConfigCase{
	boolCase,
	{name: "duration", key: "yb.taskGC.gc_check_interval", value: "3 hours"},
	{name: "integer", key: "yb.health.max_num_parallel_node_checks", value: "5"},
	{name: "string", key: "yb.tlsCertificate.organizationName", value: "tf-acctest-org"},
}

// boolCase is the Boolean entry of runtimeConfigCases, named because the
// lifecycle and negative tests (TestAccRuntimeConfig_GlobalScope, _InvalidScope,
// _MalformedValue) need a specific known-Boolean key. yb.is_platform_downgrade_allowed
// only guards whether platform downgrades are permitted (never consulted outside
// an upgrade/downgrade flow) and defaults to "false", so flipping it to "true"
// and resetting on destroy restores the fixture's prior state.
var boolCase = runtimeConfigCase{
	name:  "boolean",
	key:   "yb.is_platform_downgrade_allowed",
	value: "true",
}

// testAccCheckRuntimeConfigValue asserts YBA reports the expected value for a
// runtime config key, reading it back through the API independently of state.
func testAccCheckRuntimeConfigValue(scope, key, expected string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		return retryRuntimeConfigCheck(func() error {
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
		})
	}
}

// testAccCheckRuntimeConfigValueAbsent asserts that after destroy a key is no
// longer at the test value — i.e. the delete path reset it to its YBA-side
// default. A 404 (key fully removed from the scope) is an equally valid
// post-destroy state.
func testAccCheckRuntimeConfigValueAbsent(scope, key, testValue string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		return retryRuntimeConfigCheck(func() error {
			c := acctest.APIClient.YugawareClient
			cUUID := acctest.APIClient.CustomerID

			value, response, err := c.RuntimeConfigurationAPI.
				GetConfigurationKey(context.Background(), cUUID, scope, key).Execute()
			if err != nil {
				if acctest.IsResourceNotFoundError(err) {
					return nil
				}
				return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
					"Runtime Config", "Read")
			}
			if value == testValue {
				return fmt.Errorf(
					"runtime config key %q in scope %q still has test value %q after destroy",
					key, scope, value)
			}
			return nil
		})
	}
}
