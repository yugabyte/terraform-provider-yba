package datasource

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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
	d.SetId(string(c.CustomerUUID()))

	return diags
}
