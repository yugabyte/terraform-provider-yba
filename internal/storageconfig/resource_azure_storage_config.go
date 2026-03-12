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
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceAzureStorageConfig defines Azure Blob storage configuration resource
func ResourceAzureStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Azure Blob Storage Configuration for YugabyteDB Anywhere backups.",

		CreateContext: resourceAzureStorageConfigCreate,
		ReadContext:   resourceAzureStorageConfigRead,
		UpdateContext: resourceAzureStorageConfigUpdate,
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
				Description: "Name of the Azure storage configuration.",
			},
			"backup_location": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Azure Blob Storage container URI " +
					"(e.g., https://<account>.blob.core.windows.net/<container>).",
			},
			"sas_token": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				Description: "Azure SAS (Shared Access Signature) token. " +
					"Required if use_azure_iam is false.",
			},
			"use_azure_iam": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Use Azure managed identities for authentication. " +
					"If true, sas_token is not required. Default: false.",
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
							Description: "Azure region name (e.g., eastus).",
						},
						"location": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Azure Blob Storage URI for this region.",
						},
						"sas_token": {
							Type:        schema.TypeString,
							Optional:    true,
							Sensitive:   true,
							Description: "Azure SAS token for this region (if different from main).",
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

func resourceAzureStorageConfigCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data, err := buildAzureData(d)
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "AZ",
		Type:         "STORAGE",
	}

	r, response, err := c.CustomerConfigurationAPI.CreateCustomerConfig(ctx, cUUID).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Azure Storage Config", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ConfigUUID)
	return resourceAzureStorageConfigRead(ctx, d, meta)
}

func buildAzureData(d *schema.ResourceData) (map[string]interface{}, error) {
	data := map[string]interface{}{
		"BACKUP_LOCATION": d.Get("backup_location").(string),
	}

	useAzureIAM := d.Get("use_azure_iam").(bool)
	if useAzureIAM {
		data["USE_AZURE_IAM"] = strconv.FormatBool(useAzureIAM)
	} else {
		sasToken := d.Get("sas_token").(string)
		if sasToken == "" {
			return nil, fmt.Errorf("sas_token is required when use_azure_iam is false")
		}
		data[utils.AzureStorageSasTokenEnv] = sasToken
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
			if v := loc["sas_token"].(string); v != "" {
				locData["AZURE_STORAGE_SAS_TOKEN"] = v
			}
			locations = append(locations, locData)
		}
		if len(locations) > 0 {
			data["REGION_LOCATIONS"] = locations
		}
	}

	return data, nil
}

func resourceAzureStorageConfigRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CustomerConfigurationAPI.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Azure Storage Config", "Read")
		return diag.FromErr(errMessage)
	}

	config, err := findStorageConfig(r, d.Id(), "AZ")
	if err != nil {
		// Check if the error is specifically due to the storage config not being found
		if utils.IsResourceNotFoundError(err) {
			// If the storage config was deleted outside of Terraform, remove it from state
			// so that Terraform can recreate it on the next apply.
			tflog.Warn(
				ctx,
				fmt.Sprintf(
					"Azure Storage Config %s not found, removing from state: %v",
					d.Id(),
					err,
				),
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
	if backupLoc, ok := data["BACKUP_LOCATION"]; ok {
		if err = d.Set("backup_location", backupLoc); err != nil {
			return diag.FromErr(err)
		}
	}

	// Check if using Azure IAM
	if useIAM, ok := data["USE_AZURE_IAM"]; ok {
		useAzureIAM, _ := strconv.ParseBool(useIAM.(string))
		if err = d.Set("use_azure_iam", useAzureIAM); err != nil {
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
			// Don't read back SAS tokens - they're sensitive
			locations = append(locations, locData)
		}
		if err = d.Set("region_locations", locations); err != nil {
			return diag.FromErr(err)
		}
	}

	// Don't read back SAS token - it's sensitive and obfuscated in API response

	return nil
}

func resourceAzureStorageConfigUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data, err := buildAzureData(d)
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "AZ",
		Type:         "STORAGE",
	}

	_, response, err := c.CustomerConfigurationAPI.EditCustomerConfig(ctx, cUUID, d.Id()).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Azure Storage Config", "Update")
		return diag.FromErr(errMessage)
	}

	return resourceAzureStorageConfigRead(ctx, d, meta)
}
