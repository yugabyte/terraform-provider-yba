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
		Description: "Reads the value of one runtime configuration key on one " +
			"scope in YugabyteDB Anywhere.\n\n" +
			"The value is a string, as YBA stores it. Convert it with " +
			"`tobool`, `tonumber` or `jsondecode` to use it as another type. " +
			"When the scope has no value for the key, the data source returns " +
			"the value of a wider scope, or the default of the key. YBA masks " +
			"the value of a secret key, such as " +
			"`yb.security.ldap.ldap_service_account_password`. The plan fails " +
			"when YBA does not know the key.",

		ReadContext: dataSourceRuntimeConfigRead,

		Schema: map[string]*schema.Schema{
			"scope": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  globalRuntimeScope,
				Description: "UUID of the scope to read: the global scope " +
					"`00000000-0000-0000-0000-000000000000` (the default), or a " +
					"customer, provider or universe UUID.",
			},
			"key": {
				Type:     schema.TypeString,
				Required: true,
				Description: "Runtime configuration key to read, for example " +
					"`yb.telemetry.allow_s3`.",
			},
			"value": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Value of the key on the scope, as a string.",
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
