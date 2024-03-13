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
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ProviderRegions keeps track of the regions of the provider
func ProviderRegions() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve provider (cloud and onprem) regions.",

		ReadContext: dataSourceProviderRegionsRead,

		Schema: map[string]*schema.Schema{
			"provider_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "UUID of the provider.",
			},
			"regions_uuid": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "List of region UUIDs associated with the provider.",
			},
			"regions": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        RegionsSchema().Elem,
				Description: "List of regions associated with the provider.",
			},
		},
	}
}

func dataSourceProviderRegionsRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	pUUID := d.Get("provider_id").(string)

	r, response, err := c.RegionManagementApi.GetRegion(ctx, cUUID, pUUID).Execute()

	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Provider Regions", "Read")
		return diag.FromErr(errMessage)
	}

	regions := make([]string, 0)

	for _, region := range r {
		regions = append(regions, region.GetUuid())
	}

	if err = d.Set("regions_uuid", regions); err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("regions", flattenRegions(r)); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(fmt.Sprintf("%d", len(regions)))
	return diags
}
