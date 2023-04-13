package cloud_provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// ProviderKey keeps track of the access key of the provider
func ProviderKey() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve cloud provider access key",

		ReadContext: dataSourceProviderKeyRead,

		Schema: map[string]*schema.Schema{
			"provider_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "UUID of the provider",
			},
		},
	}
}

func dataSourceProviderKeyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.AccessKeysApi.List(ctx, cUUID, d.Get("provider_id").(string)).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Provider Key", "Read")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r[0].IdKey.KeyCode)
	return diags
}
