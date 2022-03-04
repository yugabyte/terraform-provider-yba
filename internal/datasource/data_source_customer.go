package datasource

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
)

func Customer() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve customer UUID",

		ReadContext: dataSourceCustomerRead,

		Schema: map[string]*schema.Schema{},
	}
}

func dataSourceCustomerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	r, _, err := c.SessionManagementApi.GetSessionInfo(ctx).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	meta.(*api.ApiClient).ApiKeys[*r.CustomerUUID] = client.APIKey{Key: *r.ApiToken}

	d.SetId(*r.CustomerUUID)
	return diags
}
