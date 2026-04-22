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
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ProviderKey keeps track of the access key of the provider
func ProviderKey() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve provider (cloud and onprem) access key.",

		ReadContext: dataSourceProviderKeyRead,

		Schema: map[string]*schema.Schema{
			"provider_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "UUID of the provider.",
			},
			"available_access_keys": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of all available access key names for this provider.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceProviderKeyRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.AccessKeysAPI.List(ctx, cUUID, d.Get("provider_id").(string)).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Provider Key", "Read")
		return diag.FromErr(errMessage)
	}

	availableKeys := make([]string, 0, len(r))
	for _, k := range r {
		info := k.GetKeyInfo()
		name := info.GetKeyPairName()
		if name != "" {
			availableKeys = append(availableKeys, name)
		}
	}

	if err := d.Set("available_access_keys", availableKeys); err != nil {
		return diag.FromErr(err)
	}

	latest := providerutil.LatestAccessKey(r)
	if latest == nil {
		return diag.FromErr(fmt.Errorf("no access keys found for provider %s",
			d.Get("provider_id").(string)))
	}
	keyInfo := latest.GetKeyInfo()
	d.SetId(keyInfo.GetKeyPairName())
	return diags
}
