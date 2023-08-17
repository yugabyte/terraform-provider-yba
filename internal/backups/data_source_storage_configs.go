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

package backups

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// StorageConfigs lists the customer configured Storage configs used in Backups
func StorageConfigs() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve list of storage configurations.",

		ReadContext: dataSourceStorageConfigsRead,

		Schema: map[string]*schema.Schema{
			"uuid_list": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Computed:    true,
				Description: "List of storage configuration UUIDs. These can be used in the backup resource.",
			},
			"config_name": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Accepts name of the storage configuration. The corresponding " +
					"storage config UUID is stored in ID to be used in *yba_backups* resource.",
			},
		},
	}
}

func dataSourceStorageConfigsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CustomerConfigurationApi.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Storage Configs", "Read")
		return diag.FromErr(errMessage)
	}

	var ids []string
	var configName string
	for _, config := range r {
		if config.Type == "STORAGE" {
			ids = append(ids, *config.ConfigUUID)
			configName = d.Get("config_name").(string)
			if configName != "" {
				if config.ConfigName == configName {
					d.SetId(*config.ConfigUUID)
				}
			}
		}
	}
	if err = d.Set("uuid_list", ids); err != nil {
		return diag.FromErr(err)
	}
	if configName == "" {
		if len(ids) != 0 {
			d.SetId(ids[0])
		} else {
			d.SetId("")
		}
	}
	return diags
}
