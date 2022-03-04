package backups

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"strconv"
	"time"
)

func StorageConfigs() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve list of storage configs",

		ReadContext: dataSourceStorageConfigsRead,

		Schema: map[string]*schema.Schema{
			"customer_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"uuid_list": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Computed: true,
			},
		},
	}
}

func dataSourceStorageConfigsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	c := meta.(*api.ApiClient).YugawareClient

	r, _, err := c.CustomerConfigurationApi.GetListOfCustomerConfig(ctx, d.Get("customer_id").(string)).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	var ids []string
	for _, config := range r {
		if config.Type == "STORAGE" {
			ids = append(ids, *config.ConfigUUID)
		}
	}
	if err = d.Set("uuid_list", ids); err != nil {
		return diag.FromErr(err)
	}

	// always run
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	return diags
}
