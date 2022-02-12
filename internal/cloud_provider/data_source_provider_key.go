package cloud_provider

import (
	"context"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/access_keys"
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
	r, err := c.PlatformAPIs.AccessKeys.List(&access_keys.ListParams{
		CUUID:      c.CustomerUUID(),
		PUUID:      strfmt.UUID(d.Get("provider_id").(string)),
		Context:    ctx,
		HTTPClient: c.Session(),
	},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(r.Payload[0].IDKey.KeyCode)
	return diags
}
