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
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// ProviderFilter keeps track of the filtered providers
func ProviderFilter() *schema.Resource {
	return &schema.Resource{
		Description: "List of providers matching all the given filters.",

		ReadContext: dataSourceProviderFilterRead,

		Schema: map[string]*schema.Schema{
			"providers": {
				Type:     schema.TypeMap,
				Elem:     schema.TypeString,
				Computed: true,
				Description: "Map of provider name to UUIDs that match the filters. " +
					"In case no fields are given, the list will contain all providers.",
			},
			"codes": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
				Description: "List of provider codes to be matched." +
					" Allowed values: gcp, aws, azu, onprem.",
			},
			"regions": {
				Type:         schema.TypeList,
				Elem:         &schema.Schema{Type: schema.TypeString},
				Optional:     true,
				RequiredWith: []string{"codes"},
				Description: "List of region codes of a provider to be matched. " +
					"Specify with code.",
			},
			"zones": {
				Type:         schema.TypeList,
				Elem:         &schema.Schema{Type: schema.TypeString},
				Optional:     true,
				RequiredWith: []string{"codes", "regions"},
				Description: "List of zone codes of a provider to be matched. " +
					"Specify with code and regions.",
			},
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Part of the provider name to be matched.",
			},
		},
	}
}

func dataSourceProviderFilterRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	codes := d.Get("codes")
	name := d.Get("name").(string)

	providerList := make([]client.Provider, 0)

	if codes != nil && len(codes.([]interface{})) != 0 {
		codeList := utils.StringSlice(codes.([]interface{}))
		for _, code := range *codeList {
			r, response, err := c.CloudProvidersApi.GetListOfProviders(
				ctx, cUUID).ProviderCode(code).Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
					"Provider Filter", "Read")
				return diag.FromErr(errMessage)
			}
			providerList = append(providerList, r...)
		}
	} else {
		r, response, err := c.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
				"Provider Filter", "Read")
			return diag.FromErr(errMessage)
		}
		providerList = r
	}

	regions := d.Get("regions")
	var regionsList *[]string
	var zonesList *[]string
	if regions != nil {
		regionsList = utils.StringSlice(regions.([]interface{}))
		zones := d.Get("zones")
		if zones != nil {
			zonesList = utils.StringSlice(zones.([]interface{}))
		}
	}

	if name != "" {
		result := make([]client.Provider, 0)
		for _, p := range providerList {
			if strings.Contains(p.GetName(), name) {
				result = append(result, p)
			}
		}
		providerList = result
	}

	if len(*regionsList) > 0 && codes != nil && len(codes.([]interface{})) != 0 {
		result := make([]client.Provider, 0)
		for _, p := range providerList {
			addProvider := false
			for _, regionInProvider := range p.GetRegions() {
				if slices.Contains(*regionsList, regionInProvider.GetCode()) {
					if len(*zonesList) > 0 {
						for _, zoneInRegion := range regionInProvider.GetZones() {
							if slices.Contains(*zonesList, zoneInRegion.GetCode()) {
								addProvider = true
							}
						}
					} else {
						// check only regions
						addProvider = true
					}
				}
			}
			if addProvider {
				result = append(result, p)
			}
		}
		providerList = result
	}

	providers := make(map[string]string, 0)
	for _, p := range providerList {
		providers[p.GetName()] = p.GetUuid()
	}

	d.Set("providers", providers)
	d.SetId(strconv.Itoa(len(providers)))
	return diags
}
