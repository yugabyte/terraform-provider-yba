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

// keyspaceDetailsElem returns the schema.Resource used for keyspace_details lists.
// When includeBackupType is true a backup_type field is added; this is only meaningful
// for full backup entries where the table type (YQL/PGSQL/REDIS) is available.
func keyspaceDetailsElem(includeBackupType bool) *schema.Resource {
	s := map[string]*schema.Schema{
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
			Type:        schema.TypeList,
			Computed:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
			Description: "List of table names backed up in this keyspace. Empty for full keyspace backups.",
		},
	}
	if includeBackupType {
		s["backup_type"] = &schema.Schema{
			Type:     schema.TypeString,
			Computed: true,
			Description: "Backup type for this entry: " +
				"YQL_TABLE_TYPE, PGSQL_TABLE_TYPE, or REDIS_TABLE_TYPE.",
		}
	}
	return &schema.Resource{Schema: s}
}

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
				Description: "Table type of the backup: YQL_TABLE_TYPE, PGSQL_TABLE_TYPE, or REDIS_TABLE_TYPE.",
			},
			"storage_config_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the storage configuration used for the backup.",
			},
			"roles_included": {
				Type:     schema.TypeBool,
				Computed: true,
				Description: "Whether this backup includes YSQL roles and grants. " +
					"When true, a restore should also restore roles and grants to preserve " +
					"database access controls.",
			},
			"backup_category": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Category of the backup: \"full\" for a parent backup, " +
					"\"incremental\" for a backup within an incremental chain. " +
					"When universe_uuid or universe_name is used this is always \"full\" because " +
					"only parent backups are listed. " +
					"When backup_uuid is used this may be \"incremental\" if the UUID refers to " +
					"an incremental backup.",
			},
			"base_backup_uuid": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "UUID of the base (full) backup that this backup belongs to. " +
					"For a full backup this equals backup_uuid. " +
					"For an incremental backup this points to the head of the chain.",
			},
			"incremental_backup_chain": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "Ordered list of incremental backups that belong to the same chain " +
					"as this backup. Empty when no incremental backups exist for the chain. " +
					"Entries are ordered by create_time descending (newest incremental first).",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"backup_uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "UUID of this incremental backup.",
						},
						"state": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "State of this incremental backup.",
						},
						"create_time": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Timestamp when this incremental backup was created (RFC3339 UTC).",
						},
						"storage_location": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "Storage location of the first keyspace in this incremental backup. " +
								"For multi-keyspace YCQL backups use keyspace_details to access all locations.",
						},
						"keyspace_details": {
							Type:     schema.TypeList,
							Computed: true,
							Description: "Per-keyspace/database details for this incremental backup. " +
								"For multi-keyspace YCQL backups each entry corresponds to one keyspace " +
								"with its own storage location. For YSQL, typically one entry per database. " +
								"Use keyspace_details[N].storage_location when building " +
								"backup_storage_info blocks for a restore.",
							Elem: keyspaceDetailsElem(false),
						},
					},
				},
			},
			"keyspace_details": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "Per-keyspace/database details for the backup. " +
					"For multi-keyspace YCQL backups each entry corresponds to one keyspace " +
					"with its own storage location. For YSQL, typically one entry per database. " +
					"Use keyspace_details[N].storage_location and keyspace_details[N].backup_type " +
					"when building backup_storage_info blocks for a restore.",
				Elem: keyspaceDetailsElem(true),
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

// fetchAndSetIncrementalChain calls ListIncrementalBackups for the given base backup UUID,
// then populates the incremental_backup_chain field.
// If the API returns no incrementals, it sets an empty list.
func fetchAndSetIncrementalChain(
	ctx context.Context,
	d *schema.ResourceData,
	c *client.APIClient,
	cUUID string,
	baseBackupUUID string,
) diag.Diagnostics {
	chain, response, err := c.BackupsAPI.ListIncrementalBackups(ctx, cUUID, baseBackupUUID).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Backup", "List Incremental Backups")
		return diag.FromErr(errMessage)
	}

	entries := make([]map[string]interface{}, 0, len(chain))
	for _, incr := range chain {
		// The API returns all backups sharing the base UUID, including the parent backup
		// itself (where backup_uuid == base_backup_uuid). Exclude it here because the
		// parent's data is already exposed at the top level of this data source.
		if incr.GetBackupUUID() == incr.GetBaseBackupUUID() {
			continue
		}
		storageLocation := ""
		responseList := incr.GetResponseList()
		if len(responseList) > 0 {
			storageLocation = responseList[0].DefaultLocation
		}
		createTimeStr := ""
		if incr.HasCreateTime() {
			createTimeStr = incr.GetCreateTime().UTC().Format(time.RFC3339)
		}
		details := make([]map[string]interface{}, 0, len(responseList))
		for _, entry := range responseList {
			details = append(details, map[string]interface{}{
				"storage_location":     entry.DefaultLocation,
				"keyspace":             entry.Keyspace,
				"backup_size_in_bytes": int(entry.BackupSizeInBytes),
				"tables":               entry.TablesList,
			})
		}
		entries = append(entries, map[string]interface{}{
			"backup_uuid":      incr.GetBackupUUID(),
			"state":            incr.GetState(),
			"create_time":      createTimeStr,
			"storage_location": storageLocation,
			"keyspace_details": details,
		})
	}

	if err := d.Set("incremental_backup_chain", entries); err != nil {
		return diag.FromErr(err)
	}
	return nil
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

	// base_backup_uuid: present for both full (== backupUUID) and incremental backups.
	baseBackupUUID := backup.GetBackupUUID() // default to self for full backups
	if backup.HasBaseBackupUUID() {
		baseBackupUUID = backup.GetBaseBackupUUID()
	}
	if err := d.Set("base_backup_uuid", baseBackupUUID); err != nil {
		return diag.FromErr(err)
	}

	backupCategory := "full"
	if baseBackupUUID != backup.GetBackupUUID() {
		backupCategory = "incremental"
	}
	if err := d.Set("backup_category", backupCategory); err != nil {
		return diag.FromErr(err)
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
	if err := d.Set("roles_included", backupInfo.GetUseRoles()); err != nil {
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

	// Fetch the incremental backup chain using the base backup UUID.
	if chainDiags := fetchAndSetIncrementalChain(ctx, d, c, cUUID, baseBackupUUID); chainDiags != nil {
		diags = append(diags, chainDiags...)
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
	if err := d.Set("roles_included", b.GetUseRoles()); err != nil {
		return diag.FromErr(err)
	}

	// base_backup_uuid: for a full backup this equals backup_uuid; for an incremental
	// backup it points to the head of the chain.
	baseBackupUUID := info.GetBaseBackupUUID()
	if baseBackupUUID == "" {
		baseBackupUUID = info.BackupUUID
	}
	if err := d.Set("base_backup_uuid", baseBackupUUID); err != nil {
		return diag.FromErr(err)
	}

	backupCategory := "full"
	if baseBackupUUID != info.BackupUUID {
		backupCategory = "incremental"
	}
	if err := d.Set("backup_category", backupCategory); err != nil {
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

	// Fetch the incremental backup chain. For a full backup, ListIncrementalBackups is
	// called with the full backup UUID itself; for an incremental backup, we use its
	// base_backup_uuid so the entire chain is returned.
	if chainDiags := fetchAndSetIncrementalChain(ctx, d, c, cUUID, baseBackupUUID); chainDiags != nil {
		diags = append(diags, chainDiags...)
	}

	d.SetId(info.BackupUUID)
	return diags
}
