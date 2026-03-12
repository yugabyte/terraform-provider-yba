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

package storageconfig

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceS3StorageConfig defines S3 storage configuration resource
func ResourceS3StorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "S3 Storage Configuration for YugabyteDB Anywhere backups. " +
			"Supports AWS S3 and S3-compatible storage (MinIO, Ceph, etc.).",

		CreateContext: resourceS3StorageConfigCreate,
		ReadContext:   resourceS3StorageConfigRead,
		UpdateContext: resourceS3StorageConfigUpdate,
		DeleteContext: resourceStorageConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the S3 storage configuration.",
			},
			"backup_location": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "S3 bucket URI (e.g., s3://bucket-name/path).",
			},
			"access_key_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "AWS Access Key ID. Required if use_iam_instance_profile is false.",
			},
			"secret_access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "AWS Secret Access Key. Required if use_iam_instance_profile is false.",
			},
			"use_iam_instance_profile": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Use IAM Role from the YugabyteDB Anywhere host. " +
					"If true, access_key_id and secret_access_key are not required. Default: false.",
			},
			// S3-compatible storage settings
			"aws_host_base": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "S3-compatible endpoint URL (e.g., s3.amazonaws.com for AWS, " +
					"or custom endpoint for MinIO/Ceph). Leave empty for default AWS S3.",
			},
			"path_style_access": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Use path-style access for S3 requests " +
					"(required for some S3-compatible storage). Default: false.",
			},
			"use_chunked_encoding": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Use chunked encoding for S3 requests. Default: true.",
			},
			"signing_region": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "AWS signing region for S3 requests. Used as fallback region for STS.",
			},
			// IAM Configuration
			"iam_config": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Description: "Advanced IAM configuration settings.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"credential_source": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "DEFAULT",
							ValidateFunc: validation.StringInSlice([]string{
								"DEFAULT", "EC2_INSTANCE_METADATA", "ECS_CONTAINER",
								"ENVIRONMENT", "PROFILE", "STS_ASSUME_ROLE",
							}, false),
							Description: "IAM credential source. Options: DEFAULT, EC2_INSTANCE_METADATA, " +
								"ECS_CONTAINER, ENVIRONMENT, PROFILE, STS_ASSUME_ROLE.",
						},
						"iam_user_profile": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "default",
							Description: "AWS profile name for PROFILE credential source.",
						},
						"session_duration_secs": {
							Type:         schema.TypeInt,
							Optional:     true,
							Default:      3600,
							ValidateFunc: validation.IntBetween(900, 43200),
							Description:  "Session duration in seconds for assume role (900-43200). Default: 3600.",
						},
						"regional_sts": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     true,
							Description: "Use regional STS endpoint instead of global. Default: true.",
						},
						"sts_region": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Region for STS endpoint.",
						},
					},
				},
			},
			// Multi-region backup locations
			"region_locations": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "Region-specific backup locations for multi-region backups.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"region": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "AWS region name (e.g., us-east-1).",
						},
						"location": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "S3 bucket URI for this region.",
						},
						"aws_host_base": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "S3-compatible endpoint for this region.",
						},
						"signing_region": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "AWS signing region for this location.",
						},
					},
				},
			},
			// Computed fields
			"config_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the storage configuration.",
			},
		},
	}
}

func resourceS3StorageConfigCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data := buildS3Data(d)

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "S3",
		Type:         "STORAGE",
	}

	r, response, err := c.CustomerConfigurationAPI.CreateCustomerConfig(ctx, cUUID).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"S3 Storage Config", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ConfigUUID)
	return resourceS3StorageConfigRead(ctx, d, meta)
}

func buildS3Data(d *schema.ResourceData) map[string]interface{} {
	data := map[string]interface{}{
		"BACKUP_LOCATION": d.Get("backup_location").(string),
	}

	useIAM := d.Get("use_iam_instance_profile").(bool)
	if useIAM {
		data["IAM_INSTANCE_PROFILE"] = strconv.FormatBool(useIAM)
	} else {
		if v := d.Get("access_key_id").(string); v != "" {
			data[utils.AWSAccessKeyEnv] = v
		}
		if v := d.Get("secret_access_key").(string); v != "" {
			data[utils.AWSSecretAccessKeyEnv] = v
		}
	}

	// S3-compatible settings
	if v := d.Get("aws_host_base").(string); v != "" {
		data["AWS_HOST_BASE"] = v
	}
	if v := d.Get("path_style_access").(bool); v {
		data["PATH_STYLE_ACCESS"] = strconv.FormatBool(v)
	}
	if v, ok := d.GetOk("use_chunked_encoding"); ok {
		data["USE_CHUNKED_ENCODING"] = strconv.FormatBool(v.(bool))
	}
	if v := d.Get("signing_region").(string); v != "" {
		data["SIGNING_REGION"] = v
	}

	// IAM Configuration
	if iamConfigs := d.Get("iam_config").([]interface{}); len(iamConfigs) > 0 &&
		iamConfigs[0] != nil {
		iamConfig := iamConfigs[0].(map[string]interface{})
		iamData := map[string]interface{}{}

		if v := iamConfig["credential_source"].(string); v != "" {
			iamData["CREDENTIAL_SOURCE"] = v
		}
		if v := iamConfig["iam_user_profile"].(string); v != "" {
			iamData["IAM_USER_PROFILE"] = v
		}
		if v := iamConfig["session_duration_secs"].(int); v > 0 {
			iamData["SESSION_DURATION_SECS"] = v
		}
		if v, ok := iamConfig["regional_sts"].(bool); ok {
			iamData["REGIONAL_STS"] = v
		}
		if v := iamConfig["sts_region"].(string); v != "" {
			iamData["STS_REGION"] = v
		}

		if len(iamData) > 0 {
			data["IAM_CONFIGURATION"] = iamData
		}
	}

	// Region locations
	if regionLocs := d.Get("region_locations").([]interface{}); len(regionLocs) > 0 {
		locations := make([]map[string]interface{}, 0, len(regionLocs))
		for _, rl := range regionLocs {
			if rl == nil {
				continue
			}
			loc := rl.(map[string]interface{})
			locData := map[string]interface{}{
				"REGION":   loc["region"].(string),
				"LOCATION": loc["location"].(string),
			}
			if v := loc["aws_host_base"].(string); v != "" {
				locData["AWS_HOST_BASE"] = v
			}
			if v := loc["signing_region"].(string); v != "" {
				locData["SIGNING_REGION"] = v
			}
			locations = append(locations, locData)
		}
		if len(locations) > 0 {
			data["REGION_LOCATIONS"] = locations
		}
	}

	return data
}

func resourceS3StorageConfigRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CustomerConfigurationAPI.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"S3 Storage Config", "Read")
		return diag.FromErr(errMessage)
	}

	config, err := findStorageConfig(r, d.Id(), "S3")
	if err != nil {
		// Check if the error is specifically due to the storage config not being found
		if utils.IsResourceNotFoundError(err) {
			// If the storage config was deleted outside of Terraform, remove it from state
			// so that Terraform can recreate it on the next apply.
			tflog.Warn(
				ctx,
				fmt.Sprintf("S3 Storage Config %s not found, removing from state: %v", d.Id(), err),
			)
			d.SetId("")
			return nil
		}
		// For other errors, return them as diagnostics
		return diag.FromErr(err)
	}

	if err = d.Set("name", config.ConfigName); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("config_uuid", config.ConfigUUID); err != nil {
		return diag.FromErr(err)
	}

	data := config.GetData()

	if v, ok := data["BACKUP_LOCATION"]; ok {
		if err = d.Set("backup_location", v); err != nil {
			return diag.FromErr(err)
		}
	}

	// IAM settings
	if v, ok := data["IAM_INSTANCE_PROFILE"]; ok {
		useIAM, _ := strconv.ParseBool(v.(string))
		if err = d.Set("use_iam_instance_profile", useIAM); err != nil {
			return diag.FromErr(err)
		}
	}

	// S3-compatible settings
	if v, ok := data["AWS_HOST_BASE"]; ok {
		if err = d.Set("aws_host_base", v); err != nil {
			return diag.FromErr(err)
		}
	}
	if v, ok := data["PATH_STYLE_ACCESS"]; ok {
		pathStyle, _ := strconv.ParseBool(v.(string))
		if err = d.Set("path_style_access", pathStyle); err != nil {
			return diag.FromErr(err)
		}
	}
	if v, ok := data["USE_CHUNKED_ENCODING"]; ok {
		chunked, _ := strconv.ParseBool(v.(string))
		if err = d.Set("use_chunked_encoding", chunked); err != nil {
			return diag.FromErr(err)
		}
	}
	if v, ok := data["SIGNING_REGION"]; ok {
		if err = d.Set("signing_region", v); err != nil {
			return diag.FromErr(err)
		}
	}

	// IAM Configuration
	if iamConfigRaw, ok := data["IAM_CONFIGURATION"]; ok && iamConfigRaw != nil {
		iamConfig := iamConfigRaw.(map[string]interface{})
		iamConfigList := []map[string]interface{}{{
			"credential_source":     iamConfig["CREDENTIAL_SOURCE"],
			"iam_user_profile":      iamConfig["IAM_USER_PROFILE"],
			"session_duration_secs": iamConfig["SESSION_DURATION_SECS"],
			"regional_sts":          iamConfig["REGIONAL_STS"],
			"sts_region":            iamConfig["STS_REGION"],
		}}
		if err = d.Set("iam_config", iamConfigList); err != nil {
			return diag.FromErr(err)
		}
	}

	// Region locations
	if regionLocsRaw, ok := data["REGION_LOCATIONS"]; ok && regionLocsRaw != nil {
		regionLocs := regionLocsRaw.([]interface{})
		locations := make([]map[string]interface{}, 0, len(regionLocs))
		for _, rl := range regionLocs {
			if rl == nil {
				continue
			}
			loc := rl.(map[string]interface{})
			locData := map[string]interface{}{
				"region":   loc["REGION"],
				"location": loc["LOCATION"],
			}
			if v, ok := loc["AWS_HOST_BASE"]; ok {
				locData["aws_host_base"] = v
			}
			if v, ok := loc["SIGNING_REGION"]; ok {
				locData["signing_region"] = v
			}
			locations = append(locations, locData)
		}
		if err = d.Set("region_locations", locations); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceS3StorageConfigUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data := buildS3Data(d)

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "S3",
		Type:         "STORAGE",
	}

	_, response, err := c.CustomerConfigurationAPI.EditCustomerConfig(ctx, cUUID, d.Id()).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"S3 Storage Config", "Update")
		return diag.FromErr(errMessage)
	}

	return resourceS3StorageConfigRead(ctx, d, meta)
}
