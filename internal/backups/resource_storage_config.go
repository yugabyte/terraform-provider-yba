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
			"azure_credentials": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Credentials for Azure storage configurations.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"sas_token": {
							Type:      schema.TypeString,
							Required:  true,
							Sensitive: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								if len(old) > 0 && utils.ObfuscateString(new) == old {
									azCredentialsInterface := d.Get("azure_credentials").([]interface{})
									if len(azCredentialsInterface) > 0 &&
										azCredentialsInterface[0] != nil {
										azCredentials := utils.MapFromSingletonList(
											azCredentialsInterface)
										azCredentials["sas_token"] = new
										azCredentialsList := []map[string]interface{}{
											azCredentials,
										}
										d.Set("azure_credentials", azCredentialsList)

									}
									return true
								}
								return false
							},
							Description: "Azure SAS Token. Can also be set using " +
								"environment variable AZURE_STORAGE_SAS_TOKEN.",
						},
					}},
			},
			"s3_credentials": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_key_id": {
							Type:      schema.TypeString,
							Required:  true,
							Sensitive: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								if len(old) > 0 && utils.ObfuscateString(new) == old {
									s3CredentialsInterface := d.Get("s3_credentials").([]interface{})
									if len(s3CredentialsInterface) > 0 &&
										s3CredentialsInterface[0] != nil {
										s3Credentials := utils.MapFromSingletonList(
											s3CredentialsInterface)
										s3Credentials["access_key_id"] = new
										s3CredentialsList := []map[string]interface{}{
											s3Credentials,
										}
										d.Set("s3_credentials", s3CredentialsList)

									}
									return true
								}
								return false
							},
							Description: "S3 Access Key ID. Can also be set using " +
								"environment variable AWS_ACCESS_KEY_ID.",
						},
						"secret_access_key": {
							Type:      schema.TypeString,
							Required:  true,
							Sensitive: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								if len(old) > 0 && utils.ObfuscateString(new) == old {
									s3CredentialsInterface := d.Get("s3_credentials").([]interface{})
									if len(s3CredentialsInterface) > 0 &&
										s3CredentialsInterface[0] != nil {
										s3Credentials := utils.MapFromSingletonList(
											s3CredentialsInterface)
										s3Credentials["secret_access_key"] = new
										s3CredentialsList := []map[string]interface{}{
											s3Credentials,
										}
										d.Set("s3_credentials", s3CredentialsList)

									}
									return true
								}
								return false
							},
							Description: "S3 Secret Access Key. Can also be set using " +
								"environment variable AWS_SECRET_ACCESS_KEY.",
						},
					}},
				Description: "Credentials for S3 storage configurations.",
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
			}
			return nil
		}),
		customdiff.IfValue("name",
			func(ctx context.Context, value, meta interface{}) bool {
				return value.(string) == "AZ"
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				errorMessage := "Empty env variable: "

				azCredentialsInterface := d.Get("azure_credentials").([]interface{})
				if len(azCredentialsInterface) == 0 ||
					(len(azCredentialsInterface) > 0 && azCredentialsInterface[0] == nil) {
					if _, isPresent := os.LookupEnv(utils.AzureStorageSasTokenEnv); !isPresent {
						return fmt.Errorf("%s%s", errorMessage, utils.AzureStorageSasTokenEnv)
					}
				} else {
					azCredentials := utils.MapFromSingletonList(azCredentialsInterface)
					sasToken := azCredentials["sas_token"]

					if sasToken == nil || len(sasToken.(string)) == 0 {
						return fmt.Errorf("SAS Token cannot be empty in azure_credentials")
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
					s3CredentialsInterface := d.Get("s3_credentials").([]interface{})
					if len(s3CredentialsInterface) == 0 ||
						(len(s3CredentialsInterface) > 0 && s3CredentialsInterface[0] == nil) {
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
					} else {
						s3Credentials := utils.MapFromSingletonList(s3CredentialsInterface)
						accessKeyID := s3Credentials["access_key_id"]
						secretAccessKey := s3Credentials["secret_access_key"]
						if accessKeyID == nil || len(accessKeyID.(string)) == 0 {
							return fmt.Errorf("access Key ID cannot be empty in s3_credentials")
						}
						if secretAccessKey == nil || len(secretAccessKey.(string)) == 0 {
							return fmt.Errorf("secret Access Key cannot be empty in s3_credentials")
						}
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
		s3CredentialsInterface := d.Get("s3_credentials").([]interface{})
		isIAM := d.Get("use_iam_instance_profile").(bool)
		if isIAM {
			data["IAM_INSTANCE_PROFILE"] = strconv.FormatBool(isIAM)
		} else {
			if len(s3CredentialsInterface) == 0 ||
				(len(s3CredentialsInterface) > 0 && s3CredentialsInterface[0] == nil) {
				awsCreds, err := utils.AwsCredentialsFromEnv()
				if err != nil {
					return nil, err
				}
				data[utils.AWSAccessKeyEnv] = awsCreds.AccessKeyID
				data[utils.AWSSecretAccessKeyEnv] = awsCreds.SecretAccessKey
			} else {
				s3Credentials := utils.MapFromSingletonList(s3CredentialsInterface)
				accessKeyID := s3Credentials["access_key_id"]
				secretAccessKey := s3Credentials["secret_access_key"]
				if accessKeyID != nil && len(accessKeyID.(string)) > 0 {
					data[utils.AWSAccessKeyEnv] = accessKeyID.(string)
				}
				if secretAccessKey != nil && len(secretAccessKey.(string)) > 0 {
					data[utils.AWSSecretAccessKeyEnv] = secretAccessKey.(string)
				}
			}
		}
	}
	if d.Get("name").(string) == "AZ" {
		azCredentialsInterface := d.Get("azure_credentials").([]interface{})
		if len(azCredentialsInterface) == 0 ||
			(len(azCredentialsInterface) > 0 && azCredentialsInterface[0] == nil) {
			azureCreds, err := utils.AzureStorageCredentialsFromEnv()
			if err != nil {
				return nil, err
			}
			data[utils.AzureStorageSasTokenEnv] = azureCreds
		} else {
			azCredentials := utils.MapFromSingletonList(azCredentialsInterface)
			sasToken := azCredentials["sas_token"]
			if sasToken != nil && len(sasToken.(string)) > 0 {
				data[utils.AzureStorageSasTokenEnv] = sasToken.(string)
			}
		}

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
	if config.GetName() == "S3" {
		s3CredentialsInterface := d.Get("s3_credentials").([]interface{})
		if len(s3CredentialsInterface) > 0 && s3CredentialsInterface[0] != nil {
			s3Credentials := utils.MapFromSingletonList(s3CredentialsInterface)
			accessKeyID := s3Credentials["access_key_id"]
			secretAccessKey := s3Credentials["secret_access_key"]
			if accessKeyID != nil && len(accessKeyID.(string)) > 0 {
				s3Credentials["access_key_id"] = config.GetData()[utils.AWSAccessKeyEnv]
			}
			if secretAccessKey != nil && len(secretAccessKey.(string)) > 0 {
				s3Credentials["secret_access_key"] = config.GetData()[utils.AWSSecretAccessKeyEnv]
			}
			s3CredentialsList := []map[string]interface{}{s3Credentials}
			if err = d.Set("s3_credentials", s3CredentialsList); err != nil {
				return diag.FromErr(err)
			}
		} else {
			s3CredentialsList := make([]map[string]interface{}, 0)
			if err = d.Set("s3_credentials", s3CredentialsList); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if config.GetName() == "AZ" {
		azCredentialsInterface := d.Get("azure_credentials").([]interface{})
		if len(azCredentialsInterface) > 0 && azCredentialsInterface[0] != nil {
			azCredentials := utils.MapFromSingletonList(azCredentialsInterface)
			sasToken := azCredentials["sas_token"]
			if sasToken != nil && len(sasToken.(string)) > 0 {
				azCredentials["sas_token"] = config.GetData()[utils.AzureStorageSasTokenEnv]
			}
			azCredentialsList := []map[string]interface{}{azCredentials}
			if err = d.Set("azure_credentials", azCredentialsList); err != nil {
				return diag.FromErr(err)
			}
		} else {
			azCredentialsList := make([]map[string]interface{}, 0)
			if err = d.Set("azure_credentials", azCredentialsList); err != nil {
				return diag.FromErr(err)
			}
		}
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
