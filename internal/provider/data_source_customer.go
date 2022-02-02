package provider

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"net/http"
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

	c := meta.(*ApiClient)
	r, err := c.MakeRequest(http.MethodGet, "api/v1/session_info", nil)
	if err != nil {
		return diag.FromErr(err)
	}
	defer r.Body.Close()

	customer := make(map[string]interface{})
	if err = json.NewDecoder(r.Body).Decode(&customer); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(customer["customerUUID"].(string))

	return diags
}
