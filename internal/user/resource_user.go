package user

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/customer"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

func ResourceUser() *schema.Resource {
	return &schema.Resource{
		Description: "User Resource",

		CreateContext: resourceUserCreate,
		ReadContext:   resourceUserRead,
		UpdateContext: resourceUserUpdate,
		DeleteContext: resourceUserDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"connection_info": customer.ConnectionInfoSchema(),
			"email": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"password": {
				Type:     schema.TypeString,
				Required: true,
			},
			"role": {
				Type:     schema.TypeString,
				Required: true,
			},
			"features": {
				Type:     schema.TypeMap,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
				ForceNew: true,
			},
			"is_primary": {
				Type:     schema.TypeBool,
				Computed: true,
			},
		},
	}
}

func resourceUserCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	req := client.UserRegistrationData{
		Email:           d.Get("email").(string),
		Password:        utils.GetStringPointer(d.Get("password").(string)),
		ConfirmPassword: utils.GetStringPointer(d.Get("password").(string)),
		Role:            d.Get("role").(string),
	}
	r, _, err := c.UserManagementApi.CreateUser(ctx, cUUID).User(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.Uuid)
	return resourceUserRead(ctx, d, meta)
}

func resourceUserRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	r, _, err := c.UserManagementApi.GetUserDetails(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("email", r.Email); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("role", r.Role); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("is_primary", r.IsPrimary); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func resourceUserUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	if d.HasChange("role") {
		_, _, err := c.UserManagementApi.UpdateUserRole(ctx, cUUID, d.Id()).Role(d.Get("role").(string)).Execute()
		if err != nil {
			return diag.FromErr(err)
		}
	}
	if d.HasChange("password") {
		req := client.UserRegistrationData{
			Email:           d.Get("email").(string),
			Password:        utils.GetStringPointer(d.Get("password").(string)),
			ConfirmPassword: utils.GetStringPointer(d.Get("password").(string)),
			Role:            d.Get("role").(string),
		}
		_, _, err := c.UserManagementApi.UpdateUserPassword(ctx, cUUID, d.Id()).Users(req).Execute()
		if err != nil {
			return diag.FromErr(err)
		}
	}
	return resourceUserRead(ctx, d, meta)
}
func resourceUserDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	_, _, err := c.UserManagementApi.DeleteUser(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
