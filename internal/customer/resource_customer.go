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

package customer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// ResourceCustomer creates and maintains resource for customer
func ResourceCustomer() *schema.Resource {
	return &schema.Resource{
		Description: "Customer Resource." +
			"\nRequires YB_CUSTOMER_PASSWORD env variable to be set before creation",

		CreateContext: resourceCustomerCreate,
		ReadContext:   resourceCustomerRead,
		DeleteContext: resourceCustomerDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		CustomizeDiff: resourceCustomerDiff(),

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

// Validates if env variable is present only during create customer call
func resourceCustomerDiff() schema.CustomizeDiffFunc {
	return customdiff.All(customdiff.IfValueChange("email",
		func(ctx context.Context, old, new, meta interface{}) bool {
			return old.(string) == ""
		},
		func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
			errorMessage := "Empty env variable: "
			if _, present := os.LookupEnv("YB_CUSTOMER_PASSWORD"); !present {
				return fmt.Errorf("%sYB_CUSTOMER_PASSWORD", errorMessage)
			}
			return nil
		}),
	)
}

// fetches password from the environment variable during create customer call
func fetchCustomerPasswordFromEnv() (string, error) {
	customerPassword, isPresent := os.LookupEnv("YB_CUSTOMER_PASSWORD")
	if !isPresent {
		return "", errors.New("YB_CUSTOMER_PASSWORD env variable not found")
	}
	return customerPassword, nil
}

func resourceCustomerCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient

	password, err := fetchCustomerPasswordFromEnv()
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerRegisterFormData{
		Code:     d.Get("code").(string),
		Email:    d.Get("email").(string),
		Name:     d.Get("name").(string),
		Password: password,
	}
	r, response, err := c.SessionManagementApi.RegisterCustomer(ctx).CustomerRegisterFormData(
		req).GenerateApiToken(true).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Customer", "Create")
		return diag.FromErr(errMessage)
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

func resourceCustomerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	vc := meta.(*api.APIClient).VanillaClient
	apiKey := meta.(*api.APIClient).APIKey
	if d.Get("api_token").(string) != "" {
		apiKey = d.Get("api_token").(string)
	}
	newAPI, err := api.NewAPIClient(vc.Host, apiKey)
	if err != nil {
		return diag.FromErr(err)
	}
	newClient := newAPI.YugawareClient
	r, response, err := newClient.SessionManagementApi.GetSessionInfo(ctx).Execute()

	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Customer", "Read")
		return diag.FromErr(errMessage)
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

func resourceCustomerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	tflog.Debug(ctx, "marking as deleted; customer resources cannot be deleted or changed")
	d.SetId("")
	return diag.Diagnostics{}
}
