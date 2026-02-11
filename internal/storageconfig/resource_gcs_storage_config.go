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
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceGCSStorageConfig defines GCS storage configuration resource
func ResourceGCSStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "GCS (Google Cloud Storage) Configuration for YugabyteDB Anywhere backups.",

		CreateContext: resourceGCSStorageConfigCreate,
		ReadContext:   resourceGCSStorageConfigRead,
		UpdateContext: resourceGCSStorageConfigUpdate,
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
				Description: "Name of the GCS storage configuration.",
			},
			"backup_location": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "GCS bucket URI (e.g., gs://bucket-name/path).",
			},
			"credentials": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "GCP Service Account credentials JSON. Required if use_gcp_iam is false.",
			},
			"use_gcp_iam": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Use GCP IAM for authentication (workload identity). " +
					"Supported for Kubernetes GKE clusters with workload identity. " +
					"If true, credentials field is not required. Default: false.",
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
							Description: "GCP region name (e.g., us-central1).",
						},
						"location": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "GCS bucket URI for this region.",
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

func resourceGCSStorageConfigCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data, err := buildGCSData(d)
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "GCS",
		Type:         "STORAGE",
	}

	r, response, err := c.CustomerConfigurationAPI.CreateCustomerConfig(ctx, cUUID).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"GCS Storage Config", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ConfigUUID)
	return resourceGCSStorageConfigRead(ctx, d, meta)
}

func buildGCSData(d *schema.ResourceData) (map[string]interface{}, error) {
	data := map[string]interface{}{
		"BACKUP_LOCATION": d.Get("backup_location").(string),
	}

	useGCPIAM := d.Get("use_gcp_iam").(bool)
	if useGCPIAM {
		data["USE_GCP_IAM"] = strconv.FormatBool(useGCPIAM)
	} else {
		credentials := d.Get("credentials").(string)
		if credentials == "" {
			return nil, fmt.Errorf("credentials is required when use_gcp_iam is false")
		}
		// Remove newlines from credentials JSON
		credentials = strings.ReplaceAll(credentials, "\n", "")
		data[utils.GCSCredentialsJSON] = credentials
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
			locations = append(locations, locData)
		}
		if len(locations) > 0 {
			data["REGION_LOCATIONS"] = locations
		}
	}

	return data, nil
}

func resourceGCSStorageConfigRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CustomerConfigurationAPI.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"GCS Storage Config", "Read")
		return diag.FromErr(errMessage)
	}

	config, err := findStorageConfig(r, d.Id(), "GCS")
	if err != nil {
		d.SetId("")
		return nil
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

	// Check if using GCP IAM
	if useIAM, ok := data["USE_GCP_IAM"]; ok {
		useGCPIAM, _ := strconv.ParseBool(useIAM.(string))
		if err = d.Set("use_gcp_iam", useGCPIAM); err != nil {
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
			locations = append(locations, locData)
		}
		if err = d.Set("region_locations", locations); err != nil {
			return diag.FromErr(err)
		}
	}

	// Don't read back credentials - they're sensitive and obfuscated in API response

	return nil
}

func resourceGCSStorageConfigUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data, err := buildGCSData(d)
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "GCS",
		Type:         "STORAGE",
	}

	_, response, err := c.CustomerConfigurationAPI.EditCustomerConfig(ctx, cUUID, d.Id()).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"GCS Storage Config", "Update")
		return diag.FromErr(errMessage)
	}

	return resourceGCSStorageConfigRead(ctx, d, meta)
}
