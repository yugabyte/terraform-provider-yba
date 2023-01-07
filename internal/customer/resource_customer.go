package customer

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
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

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"code": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Label for the user (i.e. admin)",
			},
			"email": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Email for the user, which is used for login on the YugabyteDB Anywhere portal.",
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the user.",
			},
			"password": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Secure password for the user. Must contain an uppercase letter, number, and symbol",
			},
			"api_token": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				ForceNew:    true,
				Description: "API token for the customer.",
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

func resourceCustomerCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	req := client.CustomerRegisterFormData{
		Code:     d.Get("code").(string),
		Email:    d.Get("email").(string),
		Name:     d.Get("name").(string),
		Password: d.Get("password").(string),
	}
	r, _, err := c.SessionManagementApi.RegisterCustomer(ctx).CustomerRegisterFormData(req).GenerateApiToken(true).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	token := ""
	if r.ApiToken != nil {
		token = *r.ApiToken
	}
	if err = d.Set("api_token", token); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("cuuid", *r.CustomerUUID); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.CustomerUUID)
	return diags
}

func resourceCustomerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	//c := meta.(*api.ApiClient).YugawareClient
	vc := meta.(*api.ApiClient).VanillaClient

	new_api, err := api.NewApiClient(vc.Host, d.Get("api_token").(string))
	new_client := new_api.YugawareClient
	r, _, err := new_client.SessionManagementApi.GetSessionInfo(ctx).Execute()

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

func resourceCustomerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	tflog.Debug(ctx, "marking as deleted; customer resources cannot be deleted or changed")
	d.SetId("")
	return diag.Diagnostics{}
}
