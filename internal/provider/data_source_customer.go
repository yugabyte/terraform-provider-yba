package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceCustomer() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Retrieve customer UUID",

		ReadContext: dataSourceCustomerRead,

		Schema: map[string]*schema.Schema{
			"uuid": {
				// This description is used by the documentation generator and the language server.
				Description: "Customer UUID",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
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
	err = json.NewDecoder(r.Body).Decode(&customer)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("uuid", customer["customerUUID"]); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	return diags
}
