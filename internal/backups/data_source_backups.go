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
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Lists fetches the backups within the given set of conditions
func Lists() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve list of backups.",

		ReadContext: dataSourceBackupsListRead,

		Schema: map[string]*schema.Schema{
			// accept date range and check backups between that time to be chosen. Pick the latest
			// backup. Accept universe name or uuid to select backup
			"universe_name": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"universe_name", "universe_uuid"},
				Description:  "The name of the universe whose latest backup you want to fetch.",
			},
			"universe_uuid": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"universe_name", "universe_uuid"},
				Description:  "The UUID of the universe whose latest backup you want to fetch.",
			},
			"date_range_start": {
				Type:             schema.TypeString,
				Optional:         true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.IsRFC3339Time),
				Description: "Start date of range in which to fetch backups, " +
					"in RFC3339 format.",
			},

			"date_range_end": {
				Type:             schema.TypeString,
				Optional:         true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.IsRFC3339Time),
				Description: "End date of range in which to fetch backups, " +
					"in RFC3339 format.",
			},
			"storage_location": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Storage location of the first keyspace in the backup. " +
					"For multi-keyspace YCQL backups, use keyspace_details to access all locations.",
			},
			"backup_type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Type of the backup fetched.",
			},
			"storage_config_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the storage configuration used for backup.",
			},
			"keyspace_details": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "Per-keyspace/database details for the backup. " +
					"For multi-keyspace YCQL backups each entry corresponds to one keyspace, " +
					"each with its own storage location. For YSQL, typically one entry per database. " +
					"Use this to build backup_storage_info blocks when restoring multi-keyspace backups.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"storage_location": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Storage location for this keyspace/database backup.",
						},
						"keyspace": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Keyspace (YCQL) or database (YSQL) name.",
						},
						"backup_size_in_bytes": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Size of this keyspace backup in bytes.",
						},
						"tables": {
							Type:     schema.TypeList,
							Computed: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
							Description: "List of table names backed up in this keyspace. " +
								"Empty for full keyspace backups.",
						},
					},
				},
			},
		},
	}
}

func dataSourceBackupsListRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	req := client.BackupPagedApiQuery{
		Filter: client.BackupApiFilter{
			UniverseNameList: *utils.StringSlice(utils.CreateSingletonList(d.Get(
				"universe_name"))),
			UniverseUUIDList: *utils.StringSlice(utils.CreateSingletonList(d.Get(
				"universe_uuid"))),
		},
		SortBy:    "createTime",
		Direction: "DESC",
		Limit:     *utils.GetInt32Pointer(10),
	}

	var minTime = time.Unix(-2208988800, 0) // Jan 1, 1900
	var maxTime = minTime.Add(1<<63 - 1)

	startDateString := d.Get("date_range_start").(string)
	if startDateString != "" {
		startDate, err := time.Parse(time.RFC3339, startDateString)
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeStart = &startDate
	} else {
		startDate, err := time.Parse(time.RFC3339, minTime.Format(time.RFC3339))
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeStart = &startDate
	}

	endDateString := d.Get("date_range_end").(string)
	if endDateString != "" {
		endDate, err := time.Parse(time.RFC3339, endDateString)
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeEnd = &endDate

	} else {
		endDate, err := time.Parse(time.RFC3339, maxTime.Format(time.RFC3339))
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeEnd = &endDate
	}

	r, response, err := c.BackupsAPI.ListBackupsV2(ctx, cUUID).PageBackupsRequest(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Backup", "Read")
		return diag.FromErr(errMessage)
	}

	// Get the first entity from r
	if len(r.Entities) > 0 {
		chosenBackup := r.Entities[0]
		err = d.Set("storage_config_uuid", chosenBackup.GetCommonBackupInfo().StorageConfigUUID)
		if err != nil {
			return diag.FromErr(err)
		}
		if err = d.Set("backup_type", chosenBackup.BackupType); err != nil {
			return diag.FromErr(err)
		}
		responseList := chosenBackup.GetCommonBackupInfo().ResponseList
		if len(responseList) > 0 {
			if err = d.Set("storage_location", responseList[0].DefaultLocation); err != nil {
				return diag.FromErr(err)
			}
			keyspaceDetails := make([]map[string]interface{}, 0, len(responseList))
			for _, entry := range responseList {
				keyspaceDetails = append(keyspaceDetails, map[string]interface{}{
					"storage_location":     entry.DefaultLocation,
					"keyspace":             entry.Keyspace,
					"backup_size_in_bytes": int(entry.BackupSizeInBytes),
					"tables":               entry.TablesList,
				})
			}
			if err = d.Set("keyspace_details", keyspaceDetails); err != nil {
				return diag.FromErr(err)
			}
		}
		if err = d.Set("universe_name", chosenBackup.UniverseName); err != nil {
			return diag.FromErr(err)
		}
		if err = d.Set("universe_uuid", chosenBackup.UniverseUUID); err != nil {
			return diag.FromErr(err)
		}
		d.SetId(chosenBackup.CommonBackupInfo.BackupUUID)
		return diags
	}
	d.Set("universe_uuid", d.Get("universe_uuid"))
	d.Set("universe_name", d.Get("universe_name"))

	d.SetId("")
	return diags
}
