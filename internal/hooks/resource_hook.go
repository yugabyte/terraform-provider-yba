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

// Package hooks implements the yba_hook resource: a YugabyteDB Anywhere custom
// hook — a script YBA runs on universe nodes when a lifecycle trigger fires —
// together with the trigger and target it is bound to.
//
// YBA models the binding as a separate hook scope object, keyed by
// (trigger, target) and shared by every hook on that pair, with each hook
// attached to at most one scope. A hook without a scope is inert and a scope
// without hooks does nothing, so the resource absorbs the scope: it creates
// one on demand, adopts an existing one, and deletes a scope its last hook
// leaves. Scope bookkeeping is serialized through hookScopeMu because
// Terraform applies resources in parallel within one process.
package hooks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// hookExecutionLangs are the script languages YBA can execute; the schema
// validation and the docs both derive from this slice.
var hookExecutionLangs = []string{"Bash", "Python"}

// hookScriptFields and hookScopeFields split the schema by which YBA object a
// field lives on: the hook itself (updated via PUT) or its scope binding
// (reconciled via attach + scope bookkeeping). The update flow reverts each
// group independently when its API call fails.
var (
	hookScriptFields = []string{
		"name", "execution_lang", "hook_text", "use_sudo", "runtime_args",
	}
	hookScopeFields = []string{
		"trigger_type", "universe_uuid", "cluster_uuid", "provider_uuid",
	}
)

// hookScopeMu serializes every hook scope find-or-create / attach / delete
// sequence: scopes are keyed by (trigger, target) and shared between hooks, so
// two yba_hook resources applying in parallel would otherwise race to create
// or garbage-collect the same scope.
var hookScopeMu sync.Mutex

// hookOperationTimeout bounds each hook CRUD call; every endpoint is a plain
// YBA database operation, no long-running task is involved.
const hookOperationTimeout = 2 * time.Minute

// hookNameMaxLen is the hook name column width in YBA (Hook.name, length=100);
// YBA rejects longer names at apply time with a 400.
const hookNameMaxLen = 100

// ResourceHook manages a YBA custom hook and its trigger binding.
func ResourceHook() *schema.Resource {
	return &schema.Resource{
		Description: "YBA Hook Resource. Manages a custom hook — a Bash or " +
			"Python script that YugabyteDB Anywhere runs on universe nodes " +
			"when the configured trigger fires (for example node " +
			"provisioning, a rolling restart, or a software upgrade) — " +
			"together with where it applies: every universe (the default), " +
			"one provider, one universe, or one cluster.\n\n" +
			"Behind the API, YBA binds hooks to triggers through hook scope " +
			"objects shared by every hook with the same trigger and target. " +
			"The resource manages those scopes automatically: it reuses an " +
			"existing scope or creates one on demand, and deletes a scope " +
			"when its last hook is removed.\n\n" +
			"~> **Note:** Custom hooks must be enabled on the YBA instance: " +
			"set the global runtime config key " +
			"`yb.security.custom_hooks.enable_custom_hooks` to `true` (for " +
			"example with the `yba_runtime_config` resource). All custom " +
			"hook operations require a Super Admin API token (an Admin " +
			"token when YBA runs in cloud mode). Hooks run on VM-based " +
			"universes only (cloud and on-prem providers); YBA inserts no " +
			"hook tasks into Kubernetes universe operations.\n\n" +
			"~> **Note:** All hooks that fire on the same trigger run in " +
			"natural sort order of their names. Prefix names with a number " +
			"(`10-mount.sh`, `20-tune.sh`) to control execution order.\n\n" +
			"~> **Warning:** Deleting a hook scope in YBA cascade-deletes " +
			"every hook attached to it. This resource deletes a scope only " +
			"after re-reading it and finding it empty, but any other writer " +
			"(the YBA UI, the API, or a second Terraform state managing hooks " +
			"on the same trigger and target) can attach a hook between that " +
			"check and the delete and lose it to the cascade. Keep every hook " +
			"on one trigger and target in a single Terraform state, and do " +
			"not manage hooks on that pair outside Terraform.",

		CreateContext: resourceHookCreate,
		ReadContext:   resourceHookRead,
		UpdateContext: resourceHookUpdate,
		DeleteContext: resourceHookDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(hookOperationTimeout),
			Read:   schema.DefaultTimeout(hookOperationTimeout),
			Update: schema.DefaultTimeout(hookOperationTimeout),
			Delete: schema.DefaultTimeout(hookOperationTimeout),
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringLenBetween(1, hookNameMaxLen),
				Description: "Name of the hook, unique per customer, at most 100 " +
					"characters. The name also determines execution order: hooks " +
					"firing on the same trigger run in natural sort order of their " +
					"names.",
			},
			"execution_lang": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice(hookExecutionLangs, false),
				Description:  "Language the hook is written in. Allowed values: `Bash`, `Python`.",
			},
			"hook_text": {
				Type:     schema.TypeString,
				Required: true,
				Description: "Full contents of the hook script. Use the Terraform " +
					"`file()` function to load it from disk.",
			},
			"use_sudo": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Run the hook with superuser privileges. Requires the " +
					"global runtime config key `yb.security.custom_hooks.enable_sudo` " +
					"to be `true`. False by default.",
			},
			"runtime_args": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "Optional string arguments for the hook. YBA passes " +
					"each entry to the script as a `--KEY VALUE` command-line flag, " +
					"after its own `--parent_task <task>` and `--trigger <trigger>` " +
					"flags.",
			},
			"trigger_type": {
				Type:     schema.TypeString,
				Required: true,
				Description: "Trigger the hook runs on. Node lifecycle triggers are " +
					"`PreNodeProvision` and `PostNodeProvision`. `ApiTriggered` " +
					"hooks run only when explicitly invoked through the YBA " +
					"run-hooks API, which also requires the global runtime config " +
					"key `yb.security.custom_hooks.enable_api_triggered_hooks`. " +
					"Upgrade-task triggers follow the pattern " +
					"`Pre<Task>`/`Post<Task>` (around the whole task) and " +
					"`Pre<Task>NodeUpgrade`/`Post<Task>NodeUpgrade` (around each " +
					"node) for the tasks `RestartUniverse`, `SoftwareUpgrade`, " +
					"`RebootUniverse`, `ThirdpartySoftwareUpgrade` and " +
					"`ConfigureDBApis`, for example `PreRestartUniverse` or " +
					"`PostSoftwareUpgradeNodeUpgrade`. The `ConfigureDBApis` " +
					"triggers need YugabyteDB Anywhere 2025.2.0.0 or later; every " +
					"other trigger is available on every YBA version the provider " +
					"supports. Every trigger fires on VM-based universes only; " +
					"Kubernetes universe tasks run no hooks. YBA rejects unknown " +
					"values.",
			},
			"universe_uuid": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"provider_uuid"},
				Description: "UUID of the universe the hook applies to. Cannot be " +
					"combined with `provider_uuid`; leave both unset to apply the " +
					"hook to every universe.",
			},
			"cluster_uuid": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"universe_uuid"},
				Description: "UUID of the cluster within `universe_uuid` the hook " +
					"applies to; requires `universe_uuid`.",
			},
			"provider_uuid": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"universe_uuid"},
				Description: "UUID of the cloud provider the hook applies to. " +
					"Cannot be combined with `universe_uuid`; leave both unset to " +
					"apply the hook to every universe.",
			},
		},
	}
}

// hookFromResourceData builds the API hook payload from the script fields.
func hookFromResourceData(d *schema.ResourceData) api.Hook {
	runtimeArgs := map[string]string{}
	for k, v := range d.Get("runtime_args").(map[string]interface{}) {
		runtimeArgs[k] = fmt.Sprintf("%v", v)
	}
	return api.Hook{
		Name:          d.Get("name").(string),
		ExecutionLang: d.Get("execution_lang").(string),
		HookText:      d.Get("hook_text").(string),
		UseSudo:       d.Get("use_sudo").(bool),
		RuntimeArgs:   runtimeArgs,
	}
}

// scopeSpecFromResourceData builds the scope identity the hook must be bound
// to from the trigger/target fields.
func scopeSpecFromResourceData(d *schema.ResourceData) api.HookScopeSpec {
	return api.HookScopeSpec{
		TriggerType:  d.Get("trigger_type").(string),
		UniverseUUID: d.Get("universe_uuid").(string),
		ClusterUUID:  d.Get("cluster_uuid").(string),
		ProviderUUID: d.Get("provider_uuid").(string),
	}
}

func scopeMatchesSpec(scope api.HookScope, spec api.HookScopeSpec) bool {
	return scope.TriggerType == spec.TriggerType &&
		scope.UniverseUUID == spec.UniverseUUID &&
		scope.ClusterUUID == spec.ClusterUUID &&
		scope.ProviderUUID == spec.ProviderUUID
}

func findScopeBySpec(scopes []api.HookScope, spec api.HookScopeSpec) *api.HookScope {
	for i := range scopes {
		if scopeMatchesSpec(scopes[i], spec) {
			return &scopes[i]
		}
	}
	return nil
}

func findScopeContaining(scopes []api.HookScope, hookUUID string) *api.HookScope {
	for i := range scopes {
		for _, id := range scopes[i].HookUUIDs {
			if id == hookUUID {
				return &scopes[i]
			}
		}
	}
	return nil
}

// reconcileHookAttachment binds the hook to the scope identified by spec:
// no-op when already there, otherwise find-or-create the scope and attach
// (YBA's attach re-points a hook that was attached elsewhere), then delete the
// scope the hook left if this hook was the only one in it.
func reconcileHookAttachment(
	ctx context.Context, apiClient *api.APIClient, hookUUID string, spec api.HookScopeSpec,
) error {
	hookScopeMu.Lock()
	defer hookScopeMu.Unlock()

	vc := apiClient.VanillaClient
	scopes, err := vc.ListHookScopes(ctx, apiClient.CustomerID, apiClient.APIKey)
	if err != nil {
		return err
	}
	current := findScopeContaining(scopes, hookUUID)
	if current != nil && scopeMatchesSpec(*current, spec) {
		return nil
	}

	target := findScopeBySpec(scopes, spec)
	if target == nil {
		created, createErr := vc.CreateHookScope(
			ctx, apiClient.CustomerID, apiClient.APIKey, spec)
		if createErr != nil {
			// Another writer (a different Terraform state, the YBA UI) may have
			// created the scope between the list and the create; adopt it.
			relisted, listErr := vc.ListHookScopes(
				ctx, apiClient.CustomerID, apiClient.APIKey)
			if listErr == nil {
				target = findScopeBySpec(relisted, spec)
			}
			if target == nil {
				return createErr
			}
		} else {
			target = created
			tflog.Info(ctx, fmt.Sprintf(
				"Created hook scope %q for trigger %q", target.UUID, spec.TriggerType))
		}
	}

	if err := vc.AttachHookToScope(
		ctx, apiClient.CustomerID, target.UUID, hookUUID, apiClient.APIKey); err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	// The attach moved the hook out of its previous scope (HookScope.addHook
	// re-points the hook's single scope column). Remove that scope only if YBA
	// now reports it empty: its delete cascades onto any hook still attached.
	return deleteScopeIfEmpty(ctx, apiClient, current.UUID)
}

// deleteScopeIfEmpty re-lists the scopes and deletes scopeUUID only when YBA
// reports no hook attached to it; a scope that is already gone, or that
// another writer has since attached a hook to, is left alone. Callers hold
// hookScopeMu.
func deleteScopeIfEmpty(ctx context.Context, apiClient *api.APIClient, scopeUUID string) error {
	vc := apiClient.VanillaClient
	scopes, err := vc.ListHookScopes(ctx, apiClient.CustomerID, apiClient.APIKey)
	if err != nil {
		return err
	}
	for i := range scopes {
		if scopes[i].UUID != scopeUUID {
			continue
		}
		if len(scopes[i].HookUUIDs) > 0 {
			return nil
		}
		tflog.Info(ctx, fmt.Sprintf("Deleting empty hook scope %q (trigger %q)",
			scopeUUID, scopes[i].TriggerType))
		if err := vc.DeleteHookScope(
			ctx, apiClient.CustomerID, scopeUUID, apiClient.APIKey); err != nil {
			return fmt.Errorf("removing empty hook scope %s: %w", scopeUUID, err)
		}
		return nil
	}
	return nil
}

func resourceHookCreate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	hook := hookFromResourceData(d)

	tflog.Info(ctx, fmt.Sprintf("Creating hook %q", hook.Name))
	created, err := apiClient.VanillaClient.CreateHook(
		ctx, apiClient.CustomerID, apiClient.APIKey, hook)
	if err != nil {
		return diag.FromErr(err)
	}
	if created.UUID == "" {
		return diag.Errorf("create hook returned an empty UUID")
	}
	// The hook exists from here on: set the ID before binding it to the
	// trigger, so a failed attach taints the resource instead of leaking an
	// unmanaged hook in YBA.
	d.SetId(created.UUID)
	if err := reconcileHookAttachment(
		ctx, apiClient, created.UUID, scopeSpecFromResourceData(d)); err != nil {
		return diag.FromErr(err)
	}
	return resourceHookRead(ctx, d, meta)
}

func resourceHookRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	hook, err := apiClient.VanillaClient.GetHook(
		ctx, apiClient.CustomerID, d.Id(), apiClient.APIKey)
	if err != nil {
		if errors.Is(err, api.ErrHookMissing) {
			tflog.Warn(ctx, fmt.Sprintf("Hook %q not found, removing from state", d.Id()))
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}
	if err = d.Set("name", hook.Name); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("execution_lang", hook.ExecutionLang); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("hook_text", hook.HookText); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("use_sudo", hook.UseSudo); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("runtime_args", hook.RuntimeArgs); err != nil {
		return diag.FromErr(err)
	}

	// The trigger binding lives on the hook's scope. A hook detached
	// out-of-band reads back with an empty trigger_type; the next plan shows
	// the diff and apply re-binds it.
	scopes, err := apiClient.VanillaClient.ListHookScopes(
		ctx, apiClient.CustomerID, apiClient.APIKey)
	if err != nil {
		return diag.FromErr(err)
	}
	spec := api.HookScopeSpec{}
	if scope := findScopeContaining(scopes, d.Id()); scope != nil {
		spec = api.HookScopeSpec{
			TriggerType:  scope.TriggerType,
			UniverseUUID: scope.UniverseUUID,
			ClusterUUID:  scope.ClusterUUID,
			ProviderUUID: scope.ProviderUUID,
		}
	}
	if err := d.Set("trigger_type", spec.TriggerType); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("universe_uuid", spec.UniverseUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("cluster_uuid", spec.ClusterUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("provider_uuid", spec.ProviderUUID); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func resourceHookUpdate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	hook := hookFromResourceData(d)

	tflog.Info(ctx, fmt.Sprintf("Updating hook %q (%s)", hook.Name, d.Id()))
	if _, err := apiClient.VanillaClient.UpdateHook(
		ctx, apiClient.CustomerID, d.Id(), apiClient.APIKey, hook); err != nil {
		utils.RevertFields(d, hookScriptFields...)
		utils.RevertFields(d, hookScopeFields...)
		return diag.FromErr(err)
	}
	if err := reconcileHookAttachment(
		ctx, apiClient, d.Id(), scopeSpecFromResourceData(d)); err != nil {
		utils.RevertFields(d, hookScopeFields...)
		return diag.FromErr(err)
	}
	return resourceHookRead(ctx, d, meta)
}

func resourceHookDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	vc := apiClient.VanillaClient

	hookScopeMu.Lock()
	defer hookScopeMu.Unlock()

	scopes, err := vc.ListHookScopes(ctx, apiClient.CustomerID, apiClient.APIKey)
	if err != nil {
		return diag.FromErr(err)
	}
	current := findScopeContaining(scopes, d.Id())

	tflog.Info(ctx, fmt.Sprintf("Deleting hook %q", d.Id()))
	if err := vc.DeleteHook(
		ctx, apiClient.CustomerID, d.Id(), apiClient.APIKey); err != nil {
		return diag.FromErr(err)
	}
	// The hook is gone. Its scope stays only while other hooks still use it;
	// deleteScopeIfEmpty re-reads before deleting so the cascade never takes a
	// hook that arrived in the meantime.
	if current != nil {
		if err := deleteScopeIfEmpty(ctx, apiClient, current.UUID); err != nil {
			return diag.FromErr(err)
		}
	}
	d.SetId("")
	return nil
}
