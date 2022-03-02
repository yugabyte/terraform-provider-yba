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
			"provider_id": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func dataSourceProviderKeyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	cUUID := meta.(*api.ApiClient).CustomerUUID
	req := c.AccessKeysApi.List(ctx, cUUID, d.Get("provider_id").(string))
	r, _, err := c.AccessKeysApi.ListExecute(req)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r[0].IdKey.KeyCode)
	return diags
}
