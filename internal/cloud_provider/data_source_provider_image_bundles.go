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

package cloud_provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ProviderImageBundles lists all image bundles for a provider
func ProviderImageBundles() *schema.Resource {
	return &schema.Resource{
		Description: "List all image bundles for a provider. " +
			"Optionally filter by architecture, name, or default status.",

		ReadContext: dataSourceProviderImageBundlesRead,

		Schema: map[string]*schema.Schema{
			"provider_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "UUID of the provider.",
			},
			"arch": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Filter by architecture. Allowed values: x86_64, aarch64.",
			},
			"default_only": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "If true, return only the default image bundle.",
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Filter by name. Matches image bundles whose name " +
					"contains this string (case-insensitive).",
			},
			"image_bundles": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "UUID of the image bundle.",
						},
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Name of the image bundle.",
						},
						"use_as_default": {
							Type:        schema.TypeBool,
							Computed:    true,
							Description: "Whether this image bundle is the default.",
						},
						"arch": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Architecture of the image bundle.",
						},
					},
				},
			},
		},
	}
}

func dataSourceProviderImageBundlesRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	pUUID := d.Get("provider_id").(string)

	req := c.PreviewAPI.GetListOfImageBundles(ctx, cUUID, pUUID)

	// Server-side arch filter
	if v, ok := d.GetOk("arch"); ok {
		req = req.Arch(v.(string))
	}

	r, response, err := req.Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Provider Image Bundles", "Read")
		return diag.FromErr(errMessage)
	}

	// Client-side filters
	defaultOnly := d.Get("default_only").(bool)
	nameFilter, nameFilterSet := d.GetOk("name")

	bundles := make([]map[string]interface{}, 0)
	for _, b := range r {
		if defaultOnly && !b.GetUseAsDefault() {
			continue
		}
		if nameFilterSet && !strings.Contains(
			strings.ToLower(b.GetName()),
			strings.ToLower(nameFilter.(string)),
		) {
			continue
		}
		bundles = append(bundles, map[string]interface{}{
			"uuid":           b.GetUuid(),
			"name":           b.GetName(),
			"use_as_default": b.GetUseAsDefault(),
			"arch":           b.Details.GetArch(),
		})
	}

	if err := d.Set("image_bundles", bundles); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(pUUID)
	return diags
}
