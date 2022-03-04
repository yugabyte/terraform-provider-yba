package customer

import (
	"context"
	"errors"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
)

func ResourceCustomer() *schema.Resource {
	return &schema.Resource{
		Description: "Customer Resource",

		CreateContext: resourceCustomerCreate,
		ReadContext:   resourceCustomerRead,
		DeleteContext: resourceCustomerDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"code": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"email": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"password": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceCustomerCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	req := client.CustomerRegisterFormData{
		Code:     d.Get("code").(string),
		Email:    d.Get("email").(string),
		Name:     d.Get("name").(string),
		Password: d.Get("password").(string),
	}
	r, _, err := c.SessionManagementApi.RegisterCustomer(ctx).CustomerRegisterFormData(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}
	
	d.SetId(*r.CustomerUUID)
	return diags
}

func resourceCustomerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return diag.Diagnostics{}
}

func resourceCustomerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return diag.FromErr(errors.New("customer resource cannot be deleted or changed"))
}
