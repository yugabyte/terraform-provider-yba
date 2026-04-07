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
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Lists fetches backup information by direct UUID lookup or by universe + date filter.
func Lists() *schema.Resource {
	return &schema.Resource{
		Description: "Fetch backup information for use in restore operations. " +
			"Supports two lookup modes:\n" +
			"  1. Direct lookup: provide backup_uuid to read a specific backup " +
			"(works for backups created outside Terraform).\n" +
			"  2. Universe filter: provide universe_uuid or universe_name (with optional " +
			"date_range_start/date_range_end) to fetch the most recent matching backup.",

		ReadContext: dataSourceBackupsListRead,

		Schema: map[string]*schema.Schema{
			// --- Lookup mode 1: direct by UUID ---
			"backup_uuid": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ConflictsWith: []string{"date_range_start", "date_range_end"},
				ExactlyOneOf:  []string{"backup_uuid", "universe_uuid", "universe_name"},
				Description: "UUID of a specific backup to read. " +
					"Mutually exclusive with universe_uuid, universe_name, " +
					"date_range_start, and date_range_end. " +
					"Use this when the backup UUID is known from the UI or API. " +
					"Also populated as an output when using universe filter mode.",
			},

			// --- Lookup mode 2: universe + optional date range ---
			"universe_name": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"backup_uuid", "universe_uuid", "universe_name"},
				Description: "Name of the universe whose latest backup you want to fetch. " +
					"Mutually exclusive with backup_uuid and universe_uuid. " +
					"Also populated as an output.",
			},
			"universe_uuid": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"backup_uuid", "universe_uuid", "universe_name"},
				Description: "UUID of the universe whose latest backup you want to fetch. " +
					"Mutually exclusive with backup_uuid and universe_name. " +
					"Also populated as an output.",
			},
			"date_range_start": {
				Type:             schema.TypeString,
				Optional:         true,
				ConflictsWith:    []string{"backup_uuid"},
				ValidateDiagFunc: validation.ToDiagFunc(validation.IsRFC3339Time),
				Description: "Earliest backup creation time to include, in RFC3339 format " +
					"(e.g. 2024-01-01T00:00:00Z). Must be UTC. " +
					"Only used with universe filter mode. " +
					"When omitted, no lower bound is applied.",
			},
			"date_range_end": {
				Type:             schema.TypeString,
				Optional:         true,
				ConflictsWith:    []string{"backup_uuid"},
				ValidateDiagFunc: validation.ToDiagFunc(validation.IsRFC3339Time),
				Description: "Latest backup creation time to include, in RFC3339 format " +
					"(e.g. 2024-12-31T23:59:59Z). Must be UTC. " +
					"Only used with universe filter mode. " +
					"When omitted, no upper bound is applied.",
			},

			// --- Computed outputs ---
			"state": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "State of the backup (e.g. Completed, InProgress, Failed).",
			},
			"create_time": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Timestamp when the backup was created (RFC3339 UTC).",
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
				Description: "Type of the backup: YQL_TABLE_TYPE, PGSQL_TABLE_TYPE, or REDIS_TABLE_TYPE.",
			},
			"storage_config_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the storage configuration used for the backup.",
			},
			"keyspace_details": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "Per-keyspace/database details for the backup. " +
					"For multi-keyspace YCQL backups each entry corresponds to one keyspace " +
					"with its own storage location. For YSQL, typically one entry per database. " +
					"Use keyspace_details[N].storage_location and keyspace_details[N].backup_type " +
					"when building backup_storage_info blocks for a restore.",
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
						"backup_type": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "Backup type for this entry: " +
								"YQL_TABLE_TYPE, PGSQL_TABLE_TYPE, or REDIS_TABLE_TYPE.",
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

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// Direct lookup by backup UUID - works for any backup regardless of how it was created.
	if backupUUID := d.Get("backup_uuid").(string); backupUUID != "" {
		return readBackupByUUID(ctx, d, c, cUUID, backupUUID)
	}

	// Universe filter mode - fetch the most recent backup matching the criteria.
	return readLatestBackupForUniverse(ctx, d, c, cUUID)
}

// readBackupByUUID fetches a specific backup by its UUID and populates state.
func readBackupByUUID(
	ctx context.Context,
	d *schema.ResourceData,
	c *client.APIClient,
	cUUID string,
	backupUUID string,
) diag.Diagnostics {
	var diags diag.Diagnostics

	backup, response, err := c.BackupsAPI.GetBackupV2(ctx, cUUID, backupUUID).Execute()
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return diag.Errorf("backup %s not found", backupUUID)
		}
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Backup", "Read")
		return diag.FromErr(errMessage)
	}

	if err := d.Set("backup_uuid", backup.GetBackupUUID()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("state", backup.GetState()); err != nil {
		return diag.FromErr(err)
	}
	if backup.HasCreateTime() {
		if err := d.Set("create_time", backup.GetCreateTime().UTC().Format(time.RFC3339)); err != nil {
			return diag.FromErr(err)
		}
	}
	if backup.HasUniverseName() {
		if err := d.Set("universe_name", backup.GetUniverseName()); err != nil {
			return diag.FromErr(err)
		}
	}

	backupInfo := backup.GetBackupInfo()
	if err := d.Set("universe_uuid", backupInfo.GetUniverseUUID()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("storage_config_uuid", backupInfo.GetStorageConfigUUID()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("backup_type", backupInfo.GetBackupType()); err != nil {
		return diag.FromErr(err)
	}

	backupList := backupInfo.GetBackupList()
	if len(backupList) > 0 {
		if err := d.Set("storage_location", backupList[0].GetStorageLocation()); err != nil {
			return diag.FromErr(err)
		}
		details := make([]map[string]interface{}, 0, len(backupList))
		for _, sub := range backupList {
			backupType := ""
			if sub.BackupType != nil {
				backupType = *sub.BackupType
			}
			details = append(details, map[string]interface{}{
				"storage_location":     sub.GetStorageLocation(),
				"keyspace":             sub.GetKeyspace(),
				"backup_type":          backupType,
				"backup_size_in_bytes": int(sub.GetBackupSizeInBytes()),
				"tables":               sub.GetTableNameList(),
			})
		}
		if err := d.Set("keyspace_details", details); err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(backupUUID)
	return diags
}

// readLatestBackupForUniverse fetches the most recent backup for the given universe.
func readLatestBackupForUniverse(
	ctx context.Context,
	d *schema.ResourceData,
	c *client.APIClient,
	cUUID string,
) diag.Diagnostics {
	var diags diag.Diagnostics

	filter := client.BackupApiFilter{}

	// Only populate the universe field that was actually specified so we don't
	// send [""] for the unused one (which the API would treat as a name filter).
	if v := d.Get("universe_name").(string); v != "" {
		filter.UniverseNameList = []string{v}
	}
	if v := d.Get("universe_uuid").(string); v != "" {
		filter.UniverseUUIDList = []string{v}
	}

	// Date range is optional. When omitted, leave nil so omitempty removes the
	// field from JSON and the API applies no date filter.
	// Convert to UTC - YBA requires the Z suffix and rejects timezone offsets.
	if s := d.Get("date_range_start").(string); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return diag.FromErr(err)
		}
		utc := t.UTC()
		filter.DateRangeStart = &utc
	}
	if s := d.Get("date_range_end").(string); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return diag.FromErr(err)
		}
		utc := t.UTC()
		filter.DateRangeEnd = &utc
	}

	req := client.BackupPagedApiQuery{
		Filter:    filter,
		SortBy:    "createTime",
		Direction: "DESC",
		Limit:     *utils.GetInt32Pointer(10),
	}

	r, response, err := c.BackupsAPI.ListBackupsV2(ctx, cUUID).PageBackupsRequest(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Backup", "Read")
		return diag.FromErr(errMessage)
	}

	if len(r.Entities) == 0 {
		// No backup found - preserve the input filter values and clear ID.
		d.Set("universe_uuid", d.Get("universe_uuid"))
		d.Set("universe_name", d.Get("universe_name"))
		d.SetId("")
		return diags
	}

	b := r.Entities[0]
	info := b.GetCommonBackupInfo()

	if err := d.Set("backup_uuid", info.BackupUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("state", info.State); err != nil {
		return diag.FromErr(err)
	}
	if info.CreateTime != nil {
		if err := d.Set("create_time", info.CreateTime.UTC().Format(time.RFC3339)); err != nil {
			return diag.FromErr(err)
		}
	}
	if err := d.Set("storage_config_uuid", info.StorageConfigUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("backup_type", b.BackupType); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("universe_name", b.UniverseName); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("universe_uuid", b.UniverseUUID); err != nil {
		return diag.FromErr(err)
	}

	responseList := info.ResponseList
	if len(responseList) > 0 {
		if err := d.Set("storage_location", responseList[0].DefaultLocation); err != nil {
			return diag.FromErr(err)
		}
		// backup_type is uniform across all keyspace entries for a given backup.
		backupType := b.BackupType
		details := make([]map[string]interface{}, 0, len(responseList))
		for _, entry := range responseList {
			details = append(details, map[string]interface{}{
				"storage_location":     entry.DefaultLocation,
				"keyspace":             entry.Keyspace,
				"backup_type":          backupType,
				"backup_size_in_bytes": int(entry.BackupSizeInBytes),
				"tables":               entry.TablesList,
			})
		}
		if err := d.Set("keyspace_details", details); err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(info.BackupUUID)
	return diags
}
