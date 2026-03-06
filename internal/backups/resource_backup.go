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
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceBackup creates and manages on-demand backups
func ResourceBackup() *schema.Resource {
	return &schema.Resource{
		Description: "On-demand backup for a YugabyteDB Anywhere universe. " +
			"Creates a one-time backup that can be managed and tracked by Terraform.",

		CreateContext: resourceBackupCreate,
		ReadContext:   resourceBackupRead,
		UpdateContext: resourceBackupUpdate,
		DeleteContext: resourceBackupDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute),
			Update: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			// Required fields - ForceNew (changing these creates a new backup)
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the universe to backup.",
			},
			"storage_config_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the storage configuration to use for the backup.",
			},
			"backup_type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"YQL_TABLE_TYPE", "REDIS_TABLE_TYPE", "PGSQL_TABLE_TYPE"}, false)),
				Description: "Type of tables to backup. " +
					"Allowed values: YQL_TABLE_TYPE (YCQL), REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE (YSQL).",
			},

			// Optional fields - ForceNew
			"keyspaces": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "List of keyspaces (YCQL) or databases (YSQL) to backup. " +
					"If empty or not specified, performs a full universe backup of all databases/keyspaces. " +
					"For YSQL, each entry is a database name. For YCQL, each entry is a keyspace name.",
			},
			"table_uuid_list": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "List of specific table UUIDs to backup. " +
					"Only applicable when a single keyspace is specified in 'keyspaces'. " +
					"If 'keyspaces' has multiple entries, this field is ignored.",
			},
			"base_backup_uuid": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "UUID of a previous backup to use as base for incremental backup. " +
					"Only supported on YB-Controller enabled universes.",
			},
			"kms_config_uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "KMS configuration UUID for encrypted backups.",
			},
			"use_tablespaces": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
				Description: "Include tablespace information in the backup. " +
					"Only applicable for YSQL (PGSQL_TABLE_TYPE) backups.",
			},
			"use_roles": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
				Description: "Backup global YSQL roles and grants to preserve " +
					"database access controls after restore. " +
					"Only applicable for YSQL (PGSQL_TABLE_TYPE) backups.",
			},
			"parallelism": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Default:     8,
				Description: "Number of concurrent commands to run on nodes over SSH.",
			},
			"sse": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Enable server-side encryption.",
			},
			"transactional_backup": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Create a transactional backup across tables.",
			},
			"table_by_table_backup": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Take table-by-table backups.",
			},

			// Updatable fields - can be changed without recreating
			"time_before_delete": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Time before the backup expires and is deleted from storage. " +
					"Accepts duration strings (e.g., '720h' for 30 days, '2160h' for 90 days). " +
					"If not set, backup is kept indefinitely. Can be updated after creation.",
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if old == "" && new == "" {
						return true
					}
					if old == "0" && new == "" {
						return true
					}
					oldMs, _, _, err := utils.GetMsFromDurationString(old)
					if err != nil {
						return false
					}
					newMs, _, _, err := utils.GetMsFromDurationString(new)
					if err != nil {
						return false
					}
					return oldMs == newMs
				},
			},

			// Computed fields - outputs
			// Note: backup UUID is stored in the resource's `id` attribute
			"state": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Current state of the backup (e.g., Completed, InProgress, Failed).",
			},
			"create_time": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Timestamp when the backup was created.",
			},
			"expiry_time": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Timestamp when the backup will expire (if expiry is set).",
			},
			"backup_size_in_bytes": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Size of the backup in bytes.",
			},
			"universe_name": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Name of the universe that this backup was created from.",
			},
			"is_full_backup": {
				Type:     schema.TypeBool,
				Computed: true,
				Description: "Whether this is a full universe backup (all databases/keyspaces) " +
					"or a specific keyspace backup.",
			},
			"keyspace_details": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "Per-keyspace/database details for the backup. " +
					"For multi-keyspace YCQL backups each entry corresponds to one keyspace, " +
					"each with its own storage location. For YSQL, typically one entry per database. " +
					"Reference these directly when building a yba_restore resource, " +
					"e.g. yba_backup.my_backup.keyspace_details[0].storage_location.",
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
							Type:        schema.TypeString,
							Computed:    true,
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

func resourceBackupCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// Build keyspace table list
	// Empty keyspaceTableList = full universe backup (all databases/keyspaces)
	// Populated keyspaceTableList = specific keyspace(s) backup
	keyspaceTableList := make([]client.KeyspaceTable, 0)
	keyspaces := d.Get("keyspaces").([]interface{})
	tableUUIDList := utils.StringSlice(d.Get("table_uuid_list").([]interface{}))

	if len(keyspaces) > 0 {
		// Multiple keyspaces specified - create an entry for each
		for i, ks := range keyspaces {
			keyspaceTable := client.KeyspaceTable{
				Keyspace: utils.GetStringPointer(ks.(string)),
			}
			// Only include table_uuid_list for the first keyspace (single keyspace case)
			// When multiple keyspaces are specified, table_uuid_list is ignored
			if i == 0 && len(keyspaces) == 1 && tableUUIDList != nil && len(*tableUUIDList) > 0 {
				keyspaceTable.TableUUIDList = *tableUUIDList
			}
			keyspaceTableList = append(keyspaceTableList, keyspaceTable)
		}
	}
	// If keyspaces is empty, keyspaceTableList stays empty
	// This triggers a full universe backup

	// Parse time before delete
	var timeBeforeDelete int64
	var timeBeforeDeleteUnit string
	var err error
	if d.Get("time_before_delete").(string) != "" {
		timeBeforeDelete, timeBeforeDeleteUnit, _, err = utils.GetMsFromDurationString(
			d.Get("time_before_delete").(string))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	// Build backup request
	req := client.BackupRequestParams{
		UniverseUUID:       d.Get("universe_uuid").(string),
		StorageConfigUUID:  d.Get("storage_config_uuid").(string),
		BackupType:         utils.GetStringPointer(d.Get("backup_type").(string)),
		KeyspaceTableList:  keyspaceTableList,
		TimeBeforeDelete:   utils.GetInt64Pointer(timeBeforeDelete),
		ExpiryTimeUnit:     utils.GetStringPointer(timeBeforeDeleteUnit),
		Sse:                utils.GetBoolPointer(d.Get("sse").(bool)),
		Parallelism:        utils.GetInt32Pointer(int32(d.Get("parallelism").(int))),
		UseTablespaces:     utils.GetBoolPointer(d.Get("use_tablespaces").(bool)),
		UseRoles:           utils.GetBoolPointer(d.Get("use_roles").(bool)),
		TableByTableBackup: utils.GetBoolPointer(d.Get("table_by_table_backup").(bool)),
	}

	// Optional fields
	if v, ok := d.GetOk("base_backup_uuid"); ok {
		req.BaseBackupUUID = utils.GetStringPointer(v.(string))
	}
	if v, ok := d.GetOk("kms_config_uuid"); ok {
		req.KmsConfigUUID = utils.GetStringPointer(v.(string))
	}

	tflog.Info(ctx, fmt.Sprintf("Creating on-demand backup for universe %s", req.UniverseUUID))

	// Call create backup API
	r, response, err := c.BackupsAPI.Createbackup(ctx, cUUID).Backup(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backup", "Create")
		return diag.FromErr(errMessage)
	}

	taskUUID := r.GetTaskUUID()
	tflog.Info(ctx, fmt.Sprintf("Backup task created with UUID: %s", taskUUID))

	// Wait for backup task to complete
	tflog.Info(ctx, "Waiting for backup task to complete...")
	err = utils.WaitForTask(ctx, taskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		return diag.FromErr(fmt.Errorf("backup task failed: %w", err))
	}

	tflog.Info(ctx, "Backup task completed successfully")

	// Find the backup UUID from the task
	universeUUID := d.Get("universe_uuid").(string)
	backupUUID, err := findBackupUUIDFromTask(ctx, c, cUUID, universeUUID, taskUUID)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to find backup UUID: %w", err))
	}

	d.SetId(backupUUID)
	tflog.Info(ctx, fmt.Sprintf("Created backup with UUID: %s", backupUUID))

	return resourceBackupRead(ctx, d, meta)
}

func resourceBackupRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	backupUUID := d.Id()

	// Get backup details
	backup, response, err := c.BackupsAPI.GetBackupV2(ctx, cUUID, backupUUID).Execute()
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			tflog.Warn(ctx, fmt.Sprintf("Backup %s not found, removing from state", backupUUID))
			d.SetId("")
			return diags
		}
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backup", "Read")
		return diag.FromErr(errMessage)
	}

	// Set computed fields
	if err := d.Set("state", backup.GetState()); err != nil {
		return diag.FromErr(err)
	}
	if backup.HasCreateTime() {
		if err := d.Set("create_time", backup.GetCreateTime().String()); err != nil {
			return diag.FromErr(err)
		}
	}
	if backup.HasExpiry() {
		if err := d.Set("expiry_time", backup.GetExpiry().String()); err != nil {
			return diag.FromErr(err)
		}
	}
	if backup.HasUniverseName() {
		if err := d.Set("universe_name", backup.GetUniverseName()); err != nil {
			return diag.FromErr(err)
		}
	}

	// Get backup info
	backupInfo := backup.GetBackupInfo()

	// Set universe UUID from backup info
	if err := d.Set("universe_uuid", backupInfo.GetUniverseUUID()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("storage_config_uuid", backupInfo.GetStorageConfigUUID()); err != nil {
		return diag.FromErr(err)
	}

	// Determine if this is a full backup based on whether keyspace list is empty
	// Full backup = no specific keyspace specified (backs up all databases)
	if err := d.Set("is_full_backup", backupInfo.GetFullBackup()); err != nil {
		return diag.FromErr(err)
	}

	// Populate keyspace_details from the per-keyspace BackupList so callers can
	// reference storage locations directly without a yba_backup_info data source.
	backupList := backupInfo.GetBackupList()
	keyspaceDetails := make([]map[string]interface{}, 0, len(backupList))
	for _, sub := range backupList {
		backupType := ""
		if sub.BackupType != nil {
			backupType = *sub.BackupType
		}
		entry := map[string]interface{}{
			"storage_location":     sub.GetStorageLocation(),
			"keyspace":             sub.GetKeyspace(),
			"backup_type":          backupType,
			"backup_size_in_bytes": int(sub.GetBackupSizeInBytes()),
			"tables":               sub.GetTableNameList(),
		}
		keyspaceDetails = append(keyspaceDetails, entry)
	}
	if err := d.Set("keyspace_details", keyspaceDetails); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceBackupUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	backupUUID := d.Id()

	// Only time_before_delete can be updated
	if d.HasChange("time_before_delete") {
		var timeBeforeDelete int64
		var timeBeforeDeleteUnit string
		var err error

		newValue := d.Get("time_before_delete").(string)
		if newValue != "" {
			timeBeforeDelete, timeBeforeDeleteUnit, _, err = utils.GetMsFromDurationString(newValue)
			if err != nil {
				return diag.FromErr(err)
			}
		}

		editParams := client.EditBackupParams{
			TimeBeforeDeleteFromPresentInMillis: utils.GetInt64Pointer(timeBeforeDelete),
			ExpiryTimeUnit:                      utils.GetStringPointer(timeBeforeDeleteUnit),
		}

		tflog.Info(ctx, fmt.Sprintf("Updating backup %s expiry time", backupUUID))

		_, response, err := c.BackupsAPI.EditBackupV2(ctx, cUUID, backupUUID).
			Backup(editParams).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Backup", "Update")
			return diag.FromErr(errMessage)
		}

		tflog.Info(ctx, fmt.Sprintf("Successfully updated backup %s", backupUUID))
	}

	return resourceBackupRead(ctx, d, meta)
}

func resourceBackupDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	backupUUID := d.Id()
	storageConfigUUID := d.Get("storage_config_uuid").(string)

	tflog.Info(ctx, fmt.Sprintf("Deleting backup %s", backupUUID))

	// Build delete request
	deleteBackupInfo := client.DeleteBackupInfo{
		BackupUUID:        backupUUID,
		StorageConfigUUID: utils.GetStringPointer(storageConfigUUID),
	}
	deleteParams := client.DeleteBackupParams{
		DeleteBackupInfos: []client.DeleteBackupInfo{deleteBackupInfo},
	}

	// Call delete API
	_, response, err := c.BackupsAPI.DeleteBackupsV2(ctx, cUUID).
		DeleteBackup(deleteParams).Execute()
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			tflog.Warn(ctx, fmt.Sprintf("Backup %s already deleted", backupUUID))
			d.SetId("")
			return nil
		}
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backup", "Delete")
		return diag.FromErr(errMessage)
	}

	tflog.Info(ctx, fmt.Sprintf("Successfully deleted backup %s", backupUUID))
	d.SetId("")
	return nil
}

// findBackupUUIDFromTask retrieves the backup UUID created by a backup task
func findBackupUUIDFromTask(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	universeUUID string,
	taskUUID string) (string, error) {

	// List backups and find the one created by this task
	req := c.BackupsAPI.FetchBackupsByTaskUUID(ctx, cUUID, universeUUID, taskUUID)
	backups, response, err := req.Execute()
	if err != nil {
		return "", utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backup", "FindByTask")
	}

	if len(backups) == 0 {
		return "", fmt.Errorf("no backup found for task %s", taskUUID)
	}

	// Return the first (and usually only) backup created by the task
	return backups[0].GetBackupUUID(), nil
}
