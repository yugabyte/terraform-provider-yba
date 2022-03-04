package customer

import (
	"context"
	"errors"
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
	_, _, err = c.SessionManagementApi.RegisterCustomer(ctx).CustomerRegisterFormData(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceCustomerRead(ctx, d, meta)
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
				return "Waiting", "Waiting", err
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
	r, _, err := c.SessionManagementApi.GetSessionInfo(ctx).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	meta.(*api.ApiClient).ApiKeys[*r.CustomerUUID] = client.APIKey{Key: *r.ApiToken}

	d.SetId(*r.CustomerUUID)
	return diags
}

func resourceCustomerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return diag.FromErr(errors.New("customer resource cannot be deleted or changed"))
}
