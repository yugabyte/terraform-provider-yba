package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client"
)

func dataSourceCustomer() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve customer UUID",

		ReadContext: dataSourceCustomerRead,

		Schema: map[string]*schema.Schema{},
	}
}

func dataSourceCustomerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*client.YugawareClient)
	d.SetId(string(c.CustomerUUID()))

	return diags
}
