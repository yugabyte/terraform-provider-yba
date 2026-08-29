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

package hooks_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const globalRuntimeScope = "00000000-0000-0000-0000-000000000000"

// The test uses global scopes on these two triggers. ApiTriggered hooks never
// fire on their own; PreRebootUniverse only fires on reboot tasks, and the
// test hook is a no-op echo, so even a leaked hook is harmless on the shared
// test YBA. Two triggers are needed to exercise the binding-move path.
var testTriggerTypes = []string{"ApiTriggered", "PreRebootUniverse"}

// enableCustomHooksFlags turns on the runtime flags custom hooks require and
// never resets them: other tests running in parallel against the same YBA may
// rely on the flags staying up.
func enableCustomHooksFlags(t *testing.T) {
	t.Helper()
	c := acctest.APIClient
	flags := map[string]string{
		"yb.security.custom_hooks.enable_custom_hooks": "true",
		"yb.security.custom_hooks.enable_sudo":         "true",
	}
	for key, val := range flags {
		_, resp, err := c.YugawareClient.RuntimeConfigurationAPI.
			SetKey(context.Background(), c.CustomerID, globalRuntimeScope, key).
			NewValue(val).Execute()
		if err != nil {
			t.Fatalf("enabling runtime flag %s=%s: %s", key, val,
				utils.ErrorFromHTTPResponse(resp, err, utils.TestEntity,
					"Custom Hooks Flags", "Set"))
		}
	}
}

// cleanLeakedTestScopes removes global scopes on the test triggers left behind
// by an earlier failed run. Scope identity is (trigger, target) — names can't
// isolate runs — so a leftover would collide with this run's scope bookkeeping.
// Deleting a scope cascades onto its attached hooks, which cleans up the
// leaked test hooks with it; on the CI-managed test YBA, global scopes on
// these triggers only ever come from this test.
func cleanLeakedTestScopes(t *testing.T) {
	t.Helper()
	c := acctest.APIClient
	ctx := context.Background()
	scopes, err := c.VanillaClient.ListHookScopes(ctx, c.CustomerID, c.APIKey)
	if err != nil {
		t.Fatalf("listing hook scopes for pre-clean: %v", err)
	}
	for _, scope := range scopes {
		if scope.UniverseUUID != "" || scope.ProviderUUID != "" ||
			!isTestTrigger(scope.TriggerType) {
			continue
		}
		t.Logf("removing leaked global %s hook scope %s from a previous run",
			scope.TriggerType, scope.UUID)
		if err := c.VanillaClient.DeleteHookScope(
			ctx, c.CustomerID, scope.UUID, c.APIKey); err != nil {
			t.Fatalf("deleting leaked scope %s: %v", scope.UUID, err)
		}
	}
}

func isTestTrigger(trigger string) bool {
	for _, tt := range testTriggerTypes {
		if tt == trigger {
			return true
		}
	}
	return false
}

func hookConfig(name, text, mode, trigger string, useSudo bool) string {
	return fmt.Sprintf(`
resource "yba_hook" "test" {
  name           = %[1]q
  execution_lang = "Bash"
  hook_text      = %[2]q
  use_sudo       = %[3]t
  runtime_args = {
    MODE = %[4]q
  }

  trigger_type = %[5]q
}
`, name, text, useSudo, mode, trigger)
}

func TestAccHook_Lifecycle(t *testing.T) {
	name := acctest.RandomName("hook") + ".sh"
	renamed := acctest.RandomName("hook") + "-v2.sh"
	textV1 := "#!/bin/bash\necho v1\n"
	textV2 := "#!/bin/bash\necho v2\n"
	hookResource := "yba_hook.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			enableCustomHooksFlags(t)
			cleanLeakedTestScopes(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckHooksDestroy,
		Steps: []resource.TestStep{
			{
				Config: hookConfig(name, textV1, "v1", testTriggerTypes[0], false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHookMatches(hookResource, name, textV1, false),
					testAccCheckHookBoundToGlobalScope(hookResource, testTriggerTypes[0]),
					resource.TestCheckResourceAttr(hookResource, "execution_lang", "Bash"),
					resource.TestCheckResourceAttr(hookResource, "runtime_args.MODE", "v1"),
					resource.TestCheckResourceAttr(
						hookResource, "trigger_type", testTriggerTypes[0]),
				),
			},
			{
				// Everything updates in place: rename, new script text, sudo
				// flipped on, runtime args replaced, and the trigger moved —
				// which must re-bind the hook and garbage-collect the old scope.
				Config: hookConfig(renamed, textV2, "v2", testTriggerTypes[1], true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHookMatches(hookResource, renamed, textV2, true),
					testAccCheckHookBoundToGlobalScope(hookResource, testTriggerTypes[1]),
					testAccCheckGlobalScopeAbsent(testTriggerTypes[0]),
					resource.TestCheckResourceAttr(hookResource, "runtime_args.MODE", "v2"),
				),
			},
			{
				ResourceName:      hookResource,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccCheckHookMatches reads the hook back through the API, independently
// of state, and compares the fields the step just applied.
func testAccCheckHookMatches(n, name, text string, useSudo bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("hook %q not found in state", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("hook %q has no id", n)
		}
		c := acctest.APIClient
		hook, err := c.VanillaClient.GetHook(
			context.Background(), c.CustomerID, rs.Primary.ID, c.APIKey)
		if err != nil {
			return err
		}
		if hook.Name != name {
			return fmt.Errorf("hook name = %q, want %q", hook.Name, name)
		}
		if hook.HookText != text {
			return fmt.Errorf("hook text = %q, want %q", hook.HookText, text)
		}
		if hook.UseSudo != useSudo {
			return fmt.Errorf("hook useSudo = %t, want %t", hook.UseSudo, useSudo)
		}
		return nil
	}
}

// testAccCheckHookBoundToGlobalScope asserts, server-side, that a global scope
// for the trigger exists and lists the hook.
func testAccCheckHookBoundToGlobalScope(n, trigger string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("hook %q not found in state", n)
		}
		c := acctest.APIClient
		scopes, err := c.VanillaClient.ListHookScopes(
			context.Background(), c.CustomerID, c.APIKey)
		if err != nil {
			return err
		}
		for _, scope := range scopes {
			if scope.TriggerType != trigger ||
				scope.UniverseUUID != "" || scope.ProviderUUID != "" {
				continue
			}
			for _, id := range scope.HookUUIDs {
				if id == rs.Primary.ID {
					return nil
				}
			}
			return fmt.Errorf("global %s scope %s exists but does not list hook %s (has %v)",
				trigger, scope.UUID, rs.Primary.ID, scope.HookUUIDs)
		}
		return fmt.Errorf("no global %s hook scope found", trigger)
	}
}

// testAccCheckGlobalScopeAbsent asserts the resource garbage-collected the
// scope its hook left.
func testAccCheckGlobalScopeAbsent(trigger string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := acctest.APIClient
		scopes, err := c.VanillaClient.ListHookScopes(
			context.Background(), c.CustomerID, c.APIKey)
		if err != nil {
			return err
		}
		for _, scope := range scopes {
			if scope.TriggerType == trigger &&
				scope.UniverseUUID == "" && scope.ProviderUUID == "" {
				return fmt.Errorf(
					"global %s hook scope %s still exists; it should have been "+
						"garbage-collected when its last hook left", trigger, scope.UUID)
			}
		}
		return nil
	}
}

func testAccCheckHooksDestroy(s *terraform.State) error {
	c := acctest.APIClient
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "yba_hook" {
			continue
		}
		_, err := c.VanillaClient.GetHook(
			context.Background(), c.CustomerID, rs.Primary.ID, c.APIKey)
		if err == nil {
			return fmt.Errorf("hook %s still exists after destroy", rs.Primary.ID)
		}
		if !errors.Is(err, api.ErrHookMissing) {
			return fmt.Errorf("unexpected error checking destroyed hook %s: %w",
				rs.Primary.ID, err)
		}
	}
	// The destroy must also have garbage-collected the scopes the hooks were
	// bound to.
	for _, trigger := range testTriggerTypes {
		if err := testAccCheckGlobalScopeAbsent(trigger)(s); err != nil {
			return err
		}
	}
	return nil
}
