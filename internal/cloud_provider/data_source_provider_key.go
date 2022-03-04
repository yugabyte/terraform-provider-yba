package cloud_provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
)

func ProviderKey() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve cloud provider access key",

		ReadContext: dataSourceProviderKeyRead,

		Schema: map[string]*schema.Schema{
			"customer_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"provider_id": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func dataSourceProviderKeyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	c := meta.(*api.ApiClient).YugawareClient

	r, _, err := c.AccessKeysApi.List(ctx, d.Get("customer_id").(string), d.Get("provider_id").(string)).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r[0].IdKey.KeyCode)
	return diags
}
