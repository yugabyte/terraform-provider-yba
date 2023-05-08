// Licensed to Yugabyte, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package user

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// ResourceUser TODO: none of these functions will work until the date issue is resolved
// https://yugabyte.atlassian.net/browse/PLAT-3305
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

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"email": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Email for the user, which is used for login on the " +
					"YugabyteDB Anywhere portal.",
			},
			"password": {
				Type:     schema.TypeString,
				Required: true,
				Description: "Secure password for the user. Must contain an " +
					"uppercase letter, number, and symbol.",
			},
			"role": {
				Type:     schema.TypeString,
				Required: true,
				Description: "User role. Permitted values: Admin, ReadOnly, SuperAdmin, " +
					"BackupAdmin",
			},
			"features": {
				Type:        schema.TypeMap,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Optional:    true,
				ForceNew:    true,
				Description: "Features of a user, json format",
			},
			"is_primary": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Flag indicating if this is the primary user for the customer",
			},
		},
	}
}

func resourceUserCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	req := client.UserRegistrationData{
		Email:           d.Get("email").(string),
		Password:        utils.GetStringPointer(d.Get("password").(string)),
		ConfirmPassword: utils.GetStringPointer(d.Get("password").(string)),
		Role:            d.Get("role").(string),
	}
	r, response, err := c.UserManagementApi.CreateUser(ctx, cUUID).User(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"User", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.Uuid)
	return resourceUserRead(ctx, d, meta)
}

func resourceUserRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.UserManagementApi.GetUserDetails(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"User", "Read")
		return diag.FromErr(errMessage)
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

func resourceUserUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	if d.HasChange("role") {
		_, response, err := c.UserManagementApi.UpdateUserRole(ctx, cUUID, d.Id()).Role(
			d.Get("role").(string)).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"User", "Update - Role")
			return diag.FromErr(errMessage)
		}
	}
	if d.HasChange("password") {
		req := client.UserRegistrationData{
			Email:           d.Get("email").(string),
			Password:        utils.GetStringPointer(d.Get("password").(string)),
			ConfirmPassword: utils.GetStringPointer(d.Get("password").(string)),
			Role:            d.Get("role").(string),
		}
		_, response, err := c.UserManagementApi.UpdateUserPassword(ctx, cUUID, d.Id()).Users(
			req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"User", "Update - Password")
			return diag.FromErr(errMessage)
		}
	}
	return resourceUserRead(ctx, d, meta)
}
func resourceUserDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	_, response, err := c.UserManagementApi.DeleteUser(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"User", "Delete")
		return diag.FromErr(errMessage)
	}

	d.SetId("")
	return diags
}
