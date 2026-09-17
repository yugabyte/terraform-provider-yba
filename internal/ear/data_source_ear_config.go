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

package ear

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// DataSourceEARConfig defines the encryption-at-rest configuration lookup.
func DataSourceEARConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Looks up an encryption-at-rest configuration by name, whichever key " +
			"management service backs it. Use it to reference a configuration created in the " +
			"YugabyteDB Anywhere UI from a universe's `encryption_at_rest` block, or to find " +
			"the UUID for `terraform import` into the matching `yba_*_ear_config` resource.",

		ReadContext: dataSourceEARConfigRead,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the configuration. Unique per customer.",
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the configuration.",
			},
			"key_provider": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Key management service behind the configuration: `GCP`, `AWS`, " +
					"`AZU`, `HASHICORP`, `CIPHERTRUST`, `OCI` or `SMARTKEY`.",
			},
			"in_use": {
				Type:     schema.TypeBool,
				Computed: true,
				Description: "True while any universe holds key history for this " +
					"configuration.",
			},
			"universes": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "Universes that hold key history for this configuration, " +
					"including ones that later disabled encryption or moved to another " +
					"configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "UUID of the universe.",
						},
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Name of the universe.",
						},
					},
				},
			},
		},
	}
}

func dataSourceEARConfigRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	name := d.Get("name").(string)

	cfg, err := getEARConfigByName(ctx, apiClient.YugawareClient, apiClient.CustomerID, name)
	if err != nil {
		return diag.FromErr(err)
	}
	if cfg == nil {
		return diag.FromErr(fmt.Errorf("no encryption at rest config named %q found", name))
	}

	d.SetId(cfg.UUID)
	if err := d.Set("uuid", cfg.UUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("key_provider", cfg.Provider); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("in_use", cfg.InUse); err != nil {
		return diag.FromErr(err)
	}
	universes := make([]interface{}, 0, len(cfg.Universes))
	for _, u := range cfg.Universes {
		universes = append(universes, map[string]interface{}{"uuid": u.UUID, "name": u.Name})
	}
	if err := d.Set("universes", universes); err != nil {
		return diag.FromErr(err)
	}
	return nil
}
