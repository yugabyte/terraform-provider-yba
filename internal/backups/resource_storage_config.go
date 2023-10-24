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

package backups

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceStorageConfig defines the schema to maintain the storage config resources
func ResourceStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Create Storage configurations.",

		CreateContext: resourceStorageConfigCreate,
		ReadContext:   resourceStorageConfigRead,
		UpdateContext: resourceStorageConfigUpdate,
		DeleteContext: resourceStorageConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		CustomizeDiff: resourceStorageConfigDiff(),

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"S3", "GCS", "AZ", "NFS"}, false)),
				Description: "Name of config provider. Allowed values: S3, GCS, NFS, AZ.",
			},
			"use_iam_instance_profile": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Use IAM Role from the YugabyteDB Anywhere Host for S3. " +
					"Storage configuration creation will fail on insufficient permissions on " +
					"the host. False by default.",
			},
			"data": {
				Type:        schema.TypeMap,
				Computed:    true,
				Description: "Location and Credentials.",
			},
			"backup_location": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The complete backup location including \"s3://\" or \"gs://\".",
			},
			"config_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the Storage Configuration.",
			},
		},
	}
}

func resourceStorageConfigDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateValue("name", func(ctx context.Context, value,
			meta interface{}) error {
			errorMessage := "Empty env variable: "
			switch code := value.(string); code {
			case "GCS":
				_, isPresent := os.LookupEnv(utils.GCPCredentialsEnv)
				if !isPresent {
					return fmt.Errorf("%s%s", errorMessage, utils.GCPCredentialsEnv)
				}
			case "AZ":
				if _, isPresent := os.LookupEnv(utils.AzureStorageSasTokenEnv); !isPresent {
					return fmt.Errorf("%s%s", errorMessage, utils.AzureStorageSasTokenEnv)
				}
			}
			return nil
		}),
		customdiff.IfValue("use_iam_instance_profile",
			func(ctx context.Context, value, meta interface{}) bool {
				// check if use_iam_credentials is set for configs other than S3
				return value.(bool)
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {

				name := d.Get("name").(string)

				// if not IAM AWS storage configuration, check for AWS credentials in env
				// throw error for other storage config options
				switch name {
				case "GCS":
					return fmt.Errorf("Cannot set use_iam_instance_profile " +
						"for GCS storage configuration")
				case "AZ":
					return fmt.Errorf("Cannot set use_iam_instance_profile " +
						"for AZ storage configuration")
				case "NFS":
					return fmt.Errorf("Cannot set use_iam_instance_profile " +
						"for NFS storage configuration")
				}

				return nil
			}),
		customdiff.IfValue("use_iam_instance_profile",
			func(ctx context.Context, value, meta interface{}) bool {
				// check if S3 storage configuration creation requires access keys
				return !value.(bool)
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				var errorString string
				errorMessage := "Empty env variable: "

				name := d.Get("name").(string)

				if name == "S3" {
					_, isPresentAccessKeyID := os.LookupEnv(utils.AWSAccessKeyEnv)
					if !isPresentAccessKeyID {
						errorString = fmt.Sprintf("%s%s ", errorString, utils.AWSAccessKeyEnv)
					}
					_, isPresentSecretAccessKey := os.LookupEnv(utils.AWSSecretAccessKeyEnv)
					if !isPresentSecretAccessKey {
						errorString = fmt.Sprintf("%s%s ", errorString,
							utils.AWSSecretAccessKeyEnv)
					}
					if !(isPresentAccessKeyID && isPresentSecretAccessKey) {
						errorString = fmt.Sprintf("%s%s", errorMessage, errorString)
						return fmt.Errorf(errorString)
					}
				}

				return nil
			}),
	)
}

func buildData(ctx context.Context, d *schema.ResourceData) (map[string]interface{}, error) {
	data := map[string]interface{}{
		"BACKUP_LOCATION": d.Get("backup_location").(string),
	}

	if d.Get("name").(string) == "GCS" {
		gcsCredString, err := utils.GcpGetCredentialsAsString()
		if err != nil {
			return nil, err
		}
		data[utils.GCSCredentialsJSON] = gcsCredString
	}

	if d.Get("name").(string) == "S3" {

		isIAM := d.Get("use_iam_instance_profile").(bool)
		if isIAM {
			data["IAM_INSTANCE_PROFILE"] = strconv.FormatBool(isIAM)
		} else {
			awsCreds, err := utils.AwsCredentialsFromEnv()
			if err != nil {
				return nil, err
			}
			data[utils.AWSAccessKeyEnv] = awsCreds.AccessKeyID
			data[utils.AWSSecretAccessKeyEnv] = awsCreds.SecretAccessKey
		}
	}
	if d.Get("name").(string) == "AZ" {
		azureCreds, err := utils.AzureStorageCredentialsFromEnv()
		if err != nil {
			return nil, err
		}
		data[utils.AzureStorageSasTokenEnv] = azureCreds
	}
	return data, nil
}

func resourceStorageConfigCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// type, name, config name, data [backup__location and credentials]
	data, err := buildData(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}
	req := client.CustomerConfig{
		ConfigName:   d.Get("config_name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         d.Get("name").(string),
		Type:         "STORAGE",
	}
	r, response, err := c.CustomerConfigurationApi.CreateCustomerConfig(ctx, cUUID).Config(
		req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Storage Config", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ConfigUUID)
	return resourceStorageConfigRead(ctx, d, meta)
}

func resourceStorageConfigRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CustomerConfigurationApi.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Storage Config", "Read")
		return diag.FromErr(errMessage)
	}
	config, err := findCustomerConfig(r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("config_name", config.ConfigName); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("data", config.Data); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("name", config.Name); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*config.ConfigUUID)
	return diags
}

func findCustomerConfig(configs []client.CustomerConfigUI, uuid string) (
	*client.CustomerConfigUI, error) {
	for _, c := range configs {
		if *c.ConfigUUID == uuid {
			return &c, nil
		}
	}
	return nil, errors.New("Could not find config with id " + uuid)
}

func resourceStorageConfigUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data, err := buildData(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   d.Get("config_name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         d.Get("name").(string),
		Type:         "STORAGE",
	}

	_, response, err := c.CustomerConfigurationApi.EditCustomerConfig(ctx, cUUID, d.Id()).Config(
		req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Storage Config", "Update")
		return diag.FromErr(errMessage)
	}

	return resourceStorageConfigRead(ctx, d, meta)
}

func resourceStorageConfigDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	_, response, err := c.CustomerConfigurationApi.DeleteCustomerConfig(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Storage Config", "Delete")
		return diag.FromErr(errMessage)
	}

	d.SetId("")
	return diags
}
