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

// Package runtimeconfig implements the yba_runtime_config resource for managing
// individual YugabyteDB Anywhere runtime configuration keys on a given scope.
package runtimeconfig

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// globalRuntimeScope is the well-known UUID that YBA uses for global-scope
// runtime configuration. Documented at
// https://docs.yugabyte.com/preview/yugabyte-platform/administer-yugabyte-platform/manage-runtime-config/
const globalRuntimeScope = "00000000-0000-0000-0000-000000000000"

// runtimeConfigOperationTimeout bounds each runtime config call; every
// endpoint is a plain YBA database operation, no long-running task is
// involved.
const runtimeConfigOperationTimeout = 2 * time.Minute

// ResourceRuntimeConfig manages a single YBA runtime configuration key/value
// pair. Useful for enabling feature flags such as
// `yb.telemetry.allow_s3` (required to allow the S3 telemetry exporter) or
// `yb.universe.metrics_export_enabled` (required to enable metrics export
// per universe).
//
// The resource is intentionally kept generic so it can be reused for any
// runtime configuration key, not just telemetry-related ones.
func ResourceRuntimeConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Sets a runtime configuration key on one scope in " +
			"YugabyteDB Anywhere. The scope is the global scope, a customer, a " +
			"provider or a universe.\n\n" +
			"Use the global scope (`00000000-0000-0000-0000-000000000000`, the " +
			"default) for keys that apply to all of YBA, such as " +
			"`yb.telemetry.allow_s3` or `yb.universe.metrics_export_enabled`. " +
			"Set `scope` to a customer, provider or universe UUID to change a " +
			"key on that scope only. YBA sets a key only on the scopes that the " +
			"key supports: for example, it rejects a global key, such as " +
			"`yb.telemetry.allow_s3`, on a universe. Destroying the resource removes " +
			"the key from the scope. The scope then uses the value of a wider " +
			"scope, or the default of the key.\n\n" +
			"~> **Note:** Only a Super Admin user can set or remove a key on " +
			"the global scope.\n\n" +
			"~> **Security Note:** Set a secret key, such as " +
			"`yb.security.ldap.ldap_service_account_password`, with " +
			"`sensitive_value`, which Terraform hides in plan output. Terraform " +
			"stores the value in the state file, so use a secure backend and " +
			"restrict access to the state file. YBA masks the value of a secret " +
			"key when it reads the key. When YBA returns the mask of the value in " +
			"the state, Terraform keeps the value in the state.",

		CreateContext: resourceRuntimeConfigCreateOrUpdate,
		ReadContext:   resourceRuntimeConfigRead,
		UpdateContext: resourceRuntimeConfigCreateOrUpdate,
		DeleteContext: resourceRuntimeConfigDelete,

		CustomizeDiff: rejectSensitiveKeyInValue,

		Importer: &schema.ResourceImporter{
			StateContext: resourceRuntimeConfigImport,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(runtimeConfigOperationTimeout),
			Update: schema.DefaultTimeout(runtimeConfigOperationTimeout),
			Delete: schema.DefaultTimeout(runtimeConfigOperationTimeout),
			Read:   schema.DefaultTimeout(runtimeConfigOperationTimeout),
		},

		Schema: map[string]*schema.Schema{
			"scope": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  globalRuntimeScope,
				Description: "UUID of the scope: the global scope " +
					"`00000000-0000-0000-0000-000000000000` (the default), or a " +
					"customer, provider or universe UUID. Changing it replaces the " +
					"resource.",
			},
			"key": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Runtime configuration key, for example " +
					"`yb.telemetry.allow_s3`. Changing it replaces the resource.",
			},
			"value": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: valueArguments,
				Description: "Value of the key, as a string. Write booleans, " +
					"numbers, durations and lists as strings, for example `\"true\"` " +
					"or `\"3 hours\"`. YBA checks the value against the type of the " +
					"key and stores it as written. Set exactly one of `value` and " +
					"`sensitive_value`.",
			},
			"sensitive_value": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				ExactlyOneOf: valueArguments,
				Description: "Value of the key, as for `value`, hidden in plan " +
					"output. A key whose value YBA masks, " +
					"`" + strings.Join(sensitiveKeys, "` or `") + "`, takes its " +
					"value only from this argument. Set exactly one of `value` and " +
					"`sensitive_value`.",
			},
		},
	}
}

var valueArguments = []string{"value", "sensitive_value"}

// sensitiveKeys mirrors YBA's RuntimeConfService.sensitiveKeys: YBA masks the
// value of these keys when it reads them.
var sensitiveKeys = []string{
	"yb.security.ldap.ldap_service_account_password",
	"yb.security.secret",
}

// rejectSensitiveKeyInValue keeps a secret key out of the plan output of
// value.
func rejectSensitiveKeyInValue(
	_ context.Context, d *schema.ResourceDiff, _ interface{},
) error {
	key := d.Get("key").(string)
	if d.Get("value").(string) != "" && slices.Contains(sensitiveKeys, key) {
		return fmt.Errorf("YBA masks the value of %s: set it with sensitive_value, not value",
			key)
	}
	return nil
}

// valueArgument is the argument that holds the value in state. Without one,
// after an import, it is the argument that the key needs.
func valueArgument(d *schema.ResourceData) string {
	if d.Get("value").(string) == "" &&
		(d.Get("sensitive_value").(string) != "" ||
			slices.Contains(sensitiveKeys, d.Get("key").(string))) {
		return "sensitive_value"
	}
	return "value"
}

// fetchRuntimeConfigValue reads the value of a runtime config key on a scope.
// It is shared by the resource and data source reads. On any API error it
// returns the translated YBA error. YBA answers a key that is not set on the
// scope with the value the scope inherits, or the key's default, so a key
// removed out of band reads back as that value, not as a 404. notFound is
// true on a 404, which YBA returns when the scope no longer exists (a deleted
// provider or universe), the key is not a mutable key, or the scope does not
// support the key and has no value set there. The resource treats notFound as
// "removed from state" (and ignores the error), while the data source surfaces
// the YBA error directly. The entity label (utils.ResourceEntity /
// utils.DataSourceEntity) only shapes the error message.
func fetchRuntimeConfigValue(
	ctx context.Context, apiClient *api.APIClient, scope, key, entity string,
) (value string, notFound bool, err error) {
	v, response, e := apiClient.YugawareClient.RuntimeConfigurationAPI.
		GetConfigurationKey(ctx, apiClient.CustomerID, scope, key).Execute()
	if e != nil {
		return "", utils.IsHTTPNotFound(response), utils.ErrorFromHTTPResponse(
			response, e, entity, "Runtime Config", "Read")
	}
	return v, false, nil
}

func resourceRuntimeConfigCreateOrUpdate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	c := apiClient.YugawareClient
	scope := d.Get("scope").(string)
	key := d.Get("key").(string)
	value := d.Get(valueArgument(d)).(string)

	_, response, err := c.RuntimeConfigurationAPI.
		SetKey(ctx, apiClient.CustomerID, scope, key).
		NewValue(value).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Runtime Config", "Set"))
	}
	d.SetId(scope + "/" + key)
	return resourceRuntimeConfigRead(ctx, d, meta)
}

func resourceRuntimeConfigRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	scope := d.Get("scope").(string)
	key := d.Get("key").(string)

	value, notFound, err := fetchRuntimeConfigValue(
		ctx, apiClient, scope, key, utils.ResourceEntity)
	if notFound {
		tflog.Warn(ctx, fmt.Sprintf(
			"Runtime config key %q not found in scope %q, removing from state", key, scope))
		d.SetId("")
		return nil
	}
	if err != nil {
		return diag.FromErr(err)
	}
	arg := valueArgument(d)
	if prior := d.Get(arg).(string); utils.IsYBAMaskOf(key, value, prior) {
		value = prior
	}
	if err := d.Set(arg, value); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func resourceRuntimeConfigDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	c := apiClient.YugawareClient
	scope := d.Get("scope").(string)
	key := d.Get("key").(string)

	_, response, err := c.RuntimeConfigurationAPI.
		DeleteKey(ctx, apiClient.CustomerID, scope, key).Execute()
	if err != nil && !utils.IsHTTPNotFound(response) {
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Runtime Config", "Delete"))
	}
	d.SetId("")
	return nil
}

// resourceRuntimeConfigImport accepts an ID of the form `<scope-uuid>/<key>`
// (or just `<key>`, in which case the global scope is assumed) and primes the
// resource state for a follow-up read. A malformed ID (empty, or with an empty
// scope or key such as `/key` or `scope/`) is rejected up front so import never
// produces a resource that only fails on the next refresh.
func resourceRuntimeConfigImport(
	_ context.Context, d *schema.ResourceData, _ interface{},
) ([]*schema.ResourceData, error) {
	id := d.Id()
	scope, key := globalRuntimeScope, id
	if s, k, ok := strings.Cut(id, "/"); ok {
		scope, key = s, k
	}
	if scope == "" || key == "" {
		return nil, fmt.Errorf(
			"invalid import ID %q: expected %q or %q (for the global scope)",
			id, "<scope-uuid>/<key>", "<key>")
	}
	if err := d.Set("scope", scope); err != nil {
		return nil, err
	}
	if err := d.Set("key", key); err != nil {
		return nil, err
	}
	d.SetId(scope + "/" + key)
	return []*schema.ResourceData{d}, nil
}
