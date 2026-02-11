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
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceNFSStorageConfig defines NFS storage configuration resource
func ResourceNFSStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "NFS Storage Configuration for YugabyteDB Anywhere backups.",

		CreateContext: resourceNFSStorageConfigCreate,
		ReadContext:   resourceNFSStorageConfigRead,
		UpdateContext: resourceNFSStorageConfigUpdate,
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
				Description: "Name of the NFS storage configuration.",
			},
			"backup_location": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "NFS mount path for backups (e.g., /mnt/nfs/backups).",
			},
			"nfs_bucket": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "yugabyte_backup",
				Description: "NFS bucket/directory name within the backup location. Default: yugabyte_backup.",
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
							Description: "Region name.",
						},
						"location": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "NFS mount path for this region.",
						},
						"nfs_bucket": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "yugabyte_backup",
							Description: "NFS bucket/directory name for this region.",
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

func resourceNFSStorageConfigCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data := buildNFSData(d)

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "NFS",
		Type:         "STORAGE",
	}

	r, response, err := c.CustomerConfigurationAPI.CreateCustomerConfig(ctx, cUUID).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"NFS Storage Config", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ConfigUUID)
	return resourceNFSStorageConfigRead(ctx, d, meta)
}

func buildNFSData(d *schema.ResourceData) map[string]interface{} {
	data := map[string]interface{}{
		"BACKUP_LOCATION": d.Get("backup_location").(string),
	}

	if v := d.Get("nfs_bucket").(string); v != "" {
		data["NFS_BUCKET"] = v
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
			if v := loc["nfs_bucket"].(string); v != "" {
				locData["NFS_BUCKET"] = v
			}
			locations = append(locations, locData)
		}
		if len(locations) > 0 {
			data["REGION_LOCATIONS"] = locations
		}
	}

	return data
}

func resourceNFSStorageConfigRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CustomerConfigurationAPI.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"NFS Storage Config", "Read")
		return diag.FromErr(errMessage)
	}

	config, err := findStorageConfig(r, d.Id(), "NFS")
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

	if nfsBucket, ok := data["NFS_BUCKET"]; ok {
		if err = d.Set("nfs_bucket", nfsBucket); err != nil {
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
			if v, ok := loc["NFS_BUCKET"]; ok {
				locData["nfs_bucket"] = v
			}
			locations = append(locations, locData)
		}
		if err = d.Set("region_locations", locations); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceNFSStorageConfigUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	data := buildNFSData(d)

	req := client.CustomerConfig{
		ConfigName:   d.Get("name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         "NFS",
		Type:         "STORAGE",
	}

	_, response, err := c.CustomerConfigurationAPI.EditCustomerConfig(ctx, cUUID, d.Id()).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"NFS Storage Config", "Update")
		return diag.FromErr(errMessage)
	}

	return resourceNFSStorageConfigRead(ctx, d, meta)
}
