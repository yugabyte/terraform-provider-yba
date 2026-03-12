// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
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

package customer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceCustomer creates and maintains resource for customer
// Follows yba-cli's register command pattern in cmd/auth/register.go
func ResourceCustomer() *schema.Resource {
	return &schema.Resource{
		Description: "Customer Resource. Registers a new customer/user in YugabyteDB Anywhere.\n\n" +
			"~> **Security Note:** The `api_token` and `password` are stored in the Terraform " +
			"state file (marked as sensitive). Use a secure backend (e.g., S3 with encryption, " +
			"Terraform Cloud) and restrict access to your state files.",

		CreateContext: resourceCustomerCreate,
		ReadContext:   resourceCustomerRead,
		UpdateContext: resourceCustomerUpdate,
		DeleteContext: resourceCustomerDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"code": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "dev",
				Description: "Environment label for the installation. " +
					"Allowed values: dev, demo, stage, prod. Defaults to 'dev'.",
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
				Description: "Name of the user.",
			},
			"password": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "Password for the user. Must meet YBA password requirements.",
			},
			"api_token": {
				Type:      schema.TypeString,
				Computed:  true,
				Sensitive: true,
				Description: "API token for the customer. This is generated after registration " +
					"and login. Store securely - it provides full access to YugabyteDB Anywhere.",
			},
			"cuuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Customer UUID.",
			},
		},
	}
}

func resourceCustomerCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient

	email := d.Get("email").(string)
	name := d.Get("name").(string)
	code := d.Get("code").(string)
	password := d.Get("password").(string)

	// Step 1: Register customer (yba-cli: authAPI.RegisterCustomer())
	tflog.Info(ctx, fmt.Sprintf("Registering customer with email: %s", email))
	req := client.CustomerRegisterFormData{
		Code:     code,
		Email:    email,
		Name:     name,
		Password: password,
	}
	r, response, err := c.SessionManagementAPI.RegisterCustomer(ctx).CustomerRegisterFormData(
		req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Customer", "Register")
		return diag.FromErr(errMessage)
	}
	tflog.Debug(ctx, "Customer registration successful")

	// Step 2: Login to get API token (yba-cli: authAPI.ApiLogin())
	tflog.Info(ctx, "Logging in to retrieve API token")
	loginReq := client.CustomerLoginFormData{
		Email:    email,
		Password: password,
	}
	loginResp, response, err := c.SessionManagementAPI.ApiLogin(ctx).CustomerLoginFormData(
		loginReq).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Customer", "Login")
		return diag.FromErr(errMessage)
	}
	tflog.Debug(ctx, "Login successful, API token retrieved")

	// Set the API token from login response
	token := ""
	if loginResp.ApiToken != nil {
		token = loginResp.GetApiToken()
	}
	if err = d.Set("api_token", token); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("cuuid", *r.CustomerUUID); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.CustomerUUID)
	tflog.Info(ctx, fmt.Sprintf("Customer created with UUID: %s", *r.CustomerUUID))
	return diags
}

func resourceCustomerRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	// Use the stored API token if available, otherwise use provider's API key
	// (yba-cli: authAPI.GetSessionInfo())
	vc := meta.(*api.APIClient).VanillaClient
	apiKey := meta.(*api.APIClient).APIKey
	storedToken := d.Get("api_token").(string)
	if storedToken != "" {
		apiKey = storedToken
	}

	newAPI, err := api.NewAPIClient(vc.EnableHTTPS, vc.Host, apiKey)
	if err != nil {
		return diag.FromErr(err)
	}
	newClient := newAPI.YugawareClient

	// Get session info to verify the customer exists and token is valid
	tflog.Debug(ctx, "Fetching session info to verify customer")
	r, response, err := newClient.SessionManagementAPI.GetSessionInfo(ctx).Execute()
	if err != nil {
		// If the customer was deleted outside of Terraform, remove it from state
		// so that Terraform can recreate it on the next apply.
		if utils.IsHTTPNotFound(response) || utils.IsHTTPBadRequestNotFound(response) {
			tflog.Warn(
				ctx,
				fmt.Sprintf("Customer %s not found, removing from state: %v", d.Id(), err),
			)
			d.SetId("")
			return diags
		}
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Customer", "Read - GetSessionInfo")
		return diag.FromErr(errMessage)
	}

	// Verify customer UUID matches
	if r.CustomerUUID == nil {
		return diag.FromErr(errors.New("could not retrieve Customer UUID from session"))
	}

	if err = d.Set("cuuid", *r.CustomerUUID); err != nil {
		return diag.FromErr(err)
	}

	// Keep the existing API token (don't overwrite with provider key)
	if storedToken != "" {
		if err = d.Set("api_token", storedToken); err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(*r.CustomerUUID)
	return diags
}

func resourceCustomerUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := d.Id()

	if d.HasChange("name") {
		newName := d.Get("name").(string)
		tflog.Info(ctx, fmt.Sprintf("Updating customer name to: %s", newName))

		req := client.CustomerAlertData{
			Name: utils.GetStringPointer(newName),
		}

		_, response, err := c.CustomerManagementAPI.UpdateCustomer(ctx, cUUID).
			Customer(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Customer", "Update")
			return diag.FromErr(errMessage)
		}
		tflog.Debug(ctx, "Customer name updated successfully")
	}

	return resourceCustomerRead(ctx, d, meta)
}

func resourceCustomerDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	tflog.Debug(ctx, "marking as deleted; customer resources cannot be deleted or changed")
	d.SetId("")
	return diag.Diagnostics{}
}
