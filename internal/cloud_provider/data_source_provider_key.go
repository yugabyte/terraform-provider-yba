package cloud_provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/customer"
)

func ProviderKey() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve cloud provider access key",

		ReadContext: dataSourceProviderKeyRead,

		Schema: map[string]*schema.Schema{
			"connection_info": customer.ConnectionInfoSchema(),
			"provider_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "UUID of the provider",
			},
		},
	}
}

func dataSourceProviderKeyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	c := meta.(*api.ApiClient).YugawareClient

	r, _, err := c.AccessKeysApi.List(ctx, cUUID, d.Get("provider_id").(string)).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r[0].IdKey.KeyCode)
	return diags
}
