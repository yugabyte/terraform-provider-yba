package customer

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
)

func Customer() *schema.Resource {
	return &schema.Resource{
		Description: "Data source that retrieves the customer UUID given an API token",

		ReadContext: dataSourceCustomerRead,

		Schema: map[string]*schema.Schema{
			"api_token": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The API Token for the customer. This can be found in the YugabyteDB Anywhere Portal",
			},
			"cuuid": {
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
				Description: "Customer UUID",
			},
		},
	}
}

func dataSourceCustomerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	ctx = context.WithValue(ctx, client.ContextAPIKeys, map[string]client.APIKey{"apiKeyAuth": {Key: d.Get("api_token").(string)}})
	r, _, err := c.SessionManagementApi.GetSessionInfo(ctx).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("api_token", *r.ApiToken); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("cuuid", *r.CustomerUUID); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.CustomerUUID)
	return diags
}
