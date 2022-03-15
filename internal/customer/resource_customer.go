package customer

import (
	"context"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"time"
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

	// if we are registering a customer, this could be a new instance of platform, so we may need to wait for startup
	tflog.Debug(ctx, "Waiting for platform")
	err := waitForStart(ctx, c, 10*time.Minute)
	if err != nil {
		return diag.FromErr(err)
	}

	tflog.Debug(ctx, "Registering customer")
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

func waitForStart(ctx context.Context, c *client.APIClient, timeout time.Duration) error {
	wait := &resource.StateChangeConf{
		Delay:   1 * time.Second,
		Pending: []string{"Waiting"},
		Target:  []string{"Ready"},
		Timeout: timeout,

		Refresh: func() (result interface{}, state string, err error) {
			_, _, err = c.SessionManagementApi.AppVersion(ctx).Execute()
			if err != nil {
				return "Waiting", "Waiting", nil
			}

			return "Ready", "Ready", nil
		},
	}

	if _, err := wait.WaitForStateContext(ctx); err != nil {
		return err
	}

	return nil
}

func resourceCustomerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
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

func resourceCustomerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	tflog.Debug(ctx, "marking as deleted; customer resources cannot be deleted or changed")
	d.SetId("")
	return diag.Diagnostics{}
}
