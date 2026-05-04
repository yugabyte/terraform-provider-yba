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

package telemetry

import (
	"context"
	"fmt"
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

// ResourceRuntimeConfig manages a single YBA runtime configuration key/value
// pair. Useful for enabling feature flags such as
// `yb.telemetry.allow_otlp` (required to allow OTLP-typed telemetry providers)
// or `yb.universe.metrics_export_enabled` (required to enable metrics export
// per universe).
//
// The resource is intentionally kept generic so it can be reused for any
// runtime configuration key, not just telemetry-related ones.
func ResourceRuntimeConfig() *schema.Resource {
	return &schema.Resource{
		Description: "YBA Runtime Config Resource. Sets a runtime configuration key on a " +
			"specific scope. Use the global scope " +
			"(`00000000-0000-0000-0000-000000000000`) for feature flags such as " +
			"`yb.telemetry.allow_otlp` or `yb.universe.metrics_export_enabled`. " +
			"Deleting the resource resets the key to its default by calling the " +
			"YBA delete-key API.\n\n" +
			"~> **Note:** Most runtime config keys require a Super Admin user.\n\n" +
			"~> **Note:** Some keys are write-only on the YBA side; the read flow " +
			"will reflect the most recent value YBA reports for the scope.",

		CreateContext: resourceRuntimeConfigCreateOrUpdate,
		ReadContext:   resourceRuntimeConfigRead,
		UpdateContext: resourceRuntimeConfigCreateOrUpdate,
		DeleteContext: resourceRuntimeConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceRuntimeConfigImport,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(2 * time.Minute),
			Update: schema.DefaultTimeout(2 * time.Minute),
			Delete: schema.DefaultTimeout(2 * time.Minute),
			Read:   schema.DefaultTimeout(2 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"scope": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     globalRuntimeScope,
				Description: "Scope UUID for the runtime config. Defaults to the YBA global scope.",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Runtime configuration key (e.g. `yb.telemetry.allow_otlp`).",
			},
			"value": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Value of the runtime configuration key. Sent as plain text to YBA.",
			},
		},
	}
}

func resourceRuntimeConfigCreateOrUpdate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	c := apiClient.YugawareClient
	scope := d.Get("scope").(string)
	key := d.Get("key").(string)
	value := d.Get("value").(string)

	tflog.Info(ctx, fmt.Sprintf(
		"Setting runtime config %q in scope %q to %q", key, scope, value))
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
	c := apiClient.YugawareClient
	scope := d.Get("scope").(string)
	key := d.Get("key").(string)

	value, response, err := c.RuntimeConfigurationAPI.
		GetConfigurationKey(ctx, apiClient.CustomerID, scope, key).Execute()
	if err != nil {
		if utils.IsHTTPNotFound(response) {
			tflog.Warn(ctx, fmt.Sprintf(
				"Runtime config key %q not found in scope %q, removing from state", key, scope))
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Runtime Config", "Read"))
	}
	if err := d.Set("value", value); err != nil {
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
// resource state for a follow-up read.
func resourceRuntimeConfigImport(
	ctx context.Context, d *schema.ResourceData, _ interface{},
) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "/", 2)
	if len(parts) == 1 {
		if err := d.Set("scope", globalRuntimeScope); err != nil {
			return nil, err
		}
		if err := d.Set("key", parts[0]); err != nil {
			return nil, err
		}
		d.SetId(globalRuntimeScope + "/" + parts[0])
	} else {
		if err := d.Set("scope", parts[0]); err != nil {
			return nil, err
		}
		if err := d.Set("key", parts[1]); err != nil {
			return nil, err
		}
	}
	return []*schema.ResourceData{d}, nil
}
