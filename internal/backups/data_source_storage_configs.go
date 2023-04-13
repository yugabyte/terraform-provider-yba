package backups

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// StorageConfigs lists the customer configured Storage configs used in Backups
func StorageConfigs() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve list of storage configs",

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
				Description: "Config name will accept the storage config to be used by the user. The " +
					"selected UUID will be stored in the ID",
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
