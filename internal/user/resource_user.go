package user

import (
	"context"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/user_management"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/models"
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
	user := &models.UserRegistrationData{
		Email:           utils.GetStringPointer(d.Get("email").(string)),
		Password:        d.Get("password").(string),
		ConfirmPassword: d.Get("password").(string),
		Role:            utils.GetStringPointer(d.Get("role").(string)),
		Features:        d.Get("features").(map[string]interface{}),
	}
	u, err := c.PlatformAPIs.UserManagement.CreateUser(
		&user_management.CreateUserParams{
			User:       user,
			CUUID:      c.CustomerUUID(),
			Context:    ctx,
			HTTPClient: c.Session(),
		},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(string(u.Payload.UUID))
	return resourceUserRead(ctx, d, meta)
}

func resourceUserRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	r, err := c.PlatformAPIs.UserManagement.GetUserDetails(
		&user_management.GetUserDetailsParams{
			CUUID:      c.CustomerUUID(),
			UUUID:      strfmt.UUID(d.Id()),
			Context:    ctx,
			HTTPClient: c.Session(),
		},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	u := r.Payload
	if err = d.Set("email", u.Email); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("role", u.Role); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("role", u.Role); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func resourceUserUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
	if d.HasChange("role") {
		_, err := c.PlatformAPIs.UserManagement.UpdateUserRole(
			&user_management.UpdateUserRoleParams{
				CUUID:      c.CustomerUUID(),
				Role:       utils.GetStringPointer(d.Get("role").(string)),
				UUUID:      strfmt.UUID(d.Id()),
				Context:    ctx,
				HTTPClient: c.Session(),
			},
			c.SwaggerAuth,
		)
		if err != nil {
			return diag.FromErr(err)
		}
	}
	if d.HasChange("password") {
		_, err := c.PlatformAPIs.UserManagement.UpdateUserPassword(
			&user_management.UpdateUserPasswordParams{
				Users: &models.UserRegistrationData{
					Password:        d.Get("password").(string),
					ConfirmPassword: d.Get("password").(string),
				},
				CUUID:      c.CustomerUUID(),
				UUUID:      strfmt.UUID(d.Id()),
				Context:    ctx,
				HTTPClient: c.Session(),
			},
			c.SwaggerAuth,
		)
		if err != nil {
			return diag.FromErr(err)
		}
	}
	return resourceUserRead(ctx, d, meta)
}
func resourceUserDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	_, err := c.PlatformAPIs.UserManagement.DeleteUser(
		&user_management.DeleteUserParams{
			CUUID:      c.CustomerUUID(),
			UUUID:      strfmt.UUID(d.Id()),
			Context:    ctx,
			HTTPClient: c.Session(),
		},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
