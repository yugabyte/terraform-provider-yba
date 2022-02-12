package backups

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/customer_configuration"
	"strconv"
	"time"
)

func StorageConfigs() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve list of storage configs",

		ReadContext: dataSourceStorageConfigsRead,

		Schema: map[string]*schema.Schema{
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

	c := meta.(*api.ApiClient).YugawareClient
	r, err := c.PlatformAPIs.CustomerConfiguration.GetListOfCustomerConfig(
		&customer_configuration.GetListOfCustomerConfigParams{
			CUUID:      c.CustomerUUID(),
			Context:    ctx,
			HTTPClient: c.Session(),
		},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	var ids []string
	for _, config := range r.Payload {
		if *config.Type == "STORAGE" {
			ids = append(ids, string(config.ConfigUUID))
		}
	}
	if err = d.Set("uuid_list", ids); err != nil {
		return diag.FromErr(err)
	}

	// always run
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))
	return diags
}
