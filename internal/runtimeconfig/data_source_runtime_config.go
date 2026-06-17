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

package runtimeconfig

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// DataSourceRuntimeConfig reads the current value of a single YBA runtime
// configuration key on a given scope. The value is always a string — exactly as
// YBA stores and returns it — so a configuration can read a key set elsewhere
// (or set by the yba_runtime_config resource) and convert it as needed with
// tobool/tonumber/jsondecode.
func DataSourceRuntimeConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the value of a single YugabyteDB Anywhere runtime " +
			"configuration key on a given scope. The value is always returned as a " +
			"string, exactly as YBA stores it; convert it with `tobool`, " +
			"`tonumber`, or `jsondecode` to consume it as another type.\n\n" +
			"~> **Note:** Reading most runtime config keys requires a Super Admin user.",

		ReadContext: dataSourceRuntimeConfigRead,

		Schema: map[string]*schema.Schema{
			"scope": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     globalRuntimeScope,
				Description: "Scope UUID to read the key from. Defaults to the YBA global scope.",
			},
			"key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Runtime configuration key to read (e.g. `yb.telemetry.allow_s3`).",
			},
			"value": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Current value of the key as reported by YBA, as a plain string.",
			},
		},
	}
}

func dataSourceRuntimeConfigRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	scope := d.Get("scope").(string)
	key := d.Get("key").(string)

	// notFound is intentionally ignored: unlike the resource (which removes
	// itself from state when the key is gone), the data source surfaces YBA's
	// error directly so a missing or non-mutable key fails the plan.
	value, _, err := fetchRuntimeConfigValue(ctx, apiClient, scope, key, utils.DataSourceEntity)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("value", value); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(scope + "/" + key)
	return nil
}
