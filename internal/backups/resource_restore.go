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

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceRestore to trigger Restore Operation
func ResourceRestore() *schema.Resource {
	return &schema.Resource{
		Description: "Restore backups for a universe. This resource triggers a restore operation " +
			"and waits for completion. Since restores are one-time operations, this resource does not " +
			"track remote state. It is recommended to remove this resource after running terraform apply.",

		CreateContext: resourceRestoreCreate,
		ReadContext:   resourceRestoreRead,
		DeleteContext: resourceRestoreDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		CustomizeDiff: customdiff.All(
			// Prevent accidental re-creation of completed restores
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				// If resource already exists (has an ID) and any ForceNew field changes,
				// block the operation to prevent accidental re-restore
				if d.Id() == "" {
					return nil
				}
				forceNewFields := []string{
					"universe_uuid", "storage_config_uuid", "backup_storage_info",
					"kms_config_uuid", "restore_to_point_in_time_millis",
					"parallelism", "enable_verbose_logs", "alter_load_balancer",
					"disable_checksum", "disable_multipart",
				}
				for _, field := range forceNewFields {
					if d.HasChange(field) {
						return fmt.Errorf(
							"cannot modify a completed restore - this would trigger " +
								"a new restore. To perform a new restore, first remove " +
								"this resource from state with: terraform state rm <address>")
					}
				}
				return nil
			},
		),

		Schema: map[string]*schema.Schema{
			// Required fields
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The UUID of the target universe to restore to.",
			},
			"storage_config_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the storage configuration where the backup is stored.",
			},

			// Backup storage info - supports multiple keyspaces
			"backup_storage_info": {
				Type:     schema.TypeList,
				Required: true,
				ForceNew: true,
				MinItems: 1,
				Description: "List of backup storage information for restoring. " +
					"Each entry specifies a keyspace/database to restore.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"storage_location": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Storage location of the backup to restore.",
						},
						"keyspace": {
							Type:     schema.TypeString,
							Required: true,
							Description: "Target keyspace/database name for the restore. " +
								"Can differ from the original to restore into a different keyspace.",
						},
						"backup_type": {
							Type:     schema.TypeString,
							Required: true,
							ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
								[]string{
									"YQL_TABLE_TYPE",
									"REDIS_TABLE_TYPE",
									"PGSQL_TABLE_TYPE",
								},
								false,
							)),
							Description: "Type of the backup. Allowed values: " +
								"YQL_TABLE_TYPE (YCQL), REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE (YSQL).",
						},
						"sse": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     false,
							Description: "Enable server-side encryption for S3 storage.",
						},
						"table_name_list": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
							Description: "List of specific table names to restore. " +
								"Only applicable for YCQL (YQL_TABLE_TYPE) backups on YBC-enabled universes. " +
								"Has no effect for YSQL - YSQL restores are always full-database.",
						},
						"selective_table_restore": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Only restore the tables listed in table_name_list " +
								"instead of all tables in the keyspace. " +
								"Only supported for YCQL (YQL_TABLE_TYPE) backups on YBC-enabled universes. " +
								"Setting this for YSQL will be rejected by the API.",
						},
						"old_owner": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "postgres",
							Description: "Current owner of the tables in the backup. " +
								"Used with new_owner to transfer ownership. Default: postgres.",
						},
						"new_owner": {
							Type:     schema.TypeString,
							Optional: true,
							Description: "New owner for the restored tables. " +
								"If specified, ownership is transferred from old_owner to new_owner.",
						},
						"use_tablespaces": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Restore tablespace information. " +
								"Only applicable for YSQL backups.",
						},
						"use_roles": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Restore global YSQL roles. " +
								"Only applicable for YSQL backups.",
						},
						"ignore_errors": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Ignore all restore errors. " +
								"WARNING: This is a preview API that could change.",
						},
						"error_if_tablespaces_exists": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Fail if tablespaces with the same names already exist. " +
								"Only applicable with use_roles enabled. " +
								"WARNING: This is a preview API that could change.",
						},
						"error_if_roles_exists": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Fail if roles with the same names already exist. " +
								"Only applicable with use_roles enabled. " +
								"WARNING: This is a preview API that could change.",
						},
					},
				},
			},

			// Optional top-level parameters
			"kms_config_uuid": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "UUID of the KMS configuration for encrypted backups. " +
					"Required if the backup was encrypted at rest.",
			},
			"parallelism": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Default:     8,
				Description: "Number of concurrent commands to run on nodes over SSH. Default: 8.",
			},
			"enable_verbose_logs": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Enable verbose logging during restore for debugging.",
			},
			"alter_load_balancer": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  true,
				Description: "Alter load balancer state during restore. " +
					"Set to false to keep load balancer running during restore. Default: true.",
			},
			"disable_checksum": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Disable checksum verification during restore.",
			},
			"disable_multipart": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Disable multipart upload/download for cloud storage.",
			},
			"restore_to_point_in_time_millis": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
				Description: "Restore to a specific point in time (Unix timestamp in milliseconds). " +
					"Used for Point-in-Time Recovery (PITR).",
			},
		},
	}
}

func resourceRestoreCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// Build backup storage info list
	backupStorageInfoList := make([]client.BackupStorageInfo, 0)
	storageInfos := d.Get("backup_storage_info").([]interface{})

	for _, si := range storageInfos {
		info := si.(map[string]interface{})

		backupStorageInfo := client.BackupStorageInfo{
			StorageLocation:       utils.GetStringPointer(info["storage_location"].(string)),
			BackupType:            utils.GetStringPointer(info["backup_type"].(string)),
			Sse:                   utils.GetBoolPointer(info["sse"].(bool)),
			SelectiveTableRestore: utils.GetBoolPointer(info["selective_table_restore"].(bool)),
			UseTablespaces:        utils.GetBoolPointer(info["use_tablespaces"].(bool)),
			UseRoles:              utils.GetBoolPointer(info["use_roles"].(bool)),
			IgnoreErrors:          utils.GetBoolPointer(info["ignore_errors"].(bool)),
			ErrorIfTablespacesExists: utils.GetBoolPointer(
				info["error_if_tablespaces_exists"].(bool),
			),
			ErrorIfRolesExists: utils.GetBoolPointer(info["error_if_roles_exists"].(bool)),
		}

		backupStorageInfo.Keyspace = utils.GetStringPointer(info["keyspace"].(string))

		// Optional old_owner
		if oldOwner, ok := info["old_owner"].(string); ok && oldOwner != "" {
			backupStorageInfo.OldOwner = utils.GetStringPointer(oldOwner)
		}

		// Optional new_owner
		if newOwner, ok := info["new_owner"].(string); ok && newOwner != "" {
			backupStorageInfo.NewOwner = utils.GetStringPointer(newOwner)
		}

		// Optional table_name_list
		if tableNames, ok := info["table_name_list"].([]interface{}); ok && len(tableNames) > 0 {
			tableNameList := make([]string, len(tableNames))
			for i, t := range tableNames {
				tableNameList[i] = t.(string)
			}
			backupStorageInfo.TableNameList = tableNameList
		}

		backupStorageInfoList = append(backupStorageInfoList, backupStorageInfo)
	}

	// Build restore request
	req := client.RestoreBackupParams{
		ActionType:            utils.GetStringPointer("RESTORE"),
		UniverseUUID:          d.Get("universe_uuid").(string),
		StorageConfigUUID:     utils.GetStringPointer(d.Get("storage_config_uuid").(string)),
		Parallelism:           utils.GetInt32Pointer(int32(d.Get("parallelism").(int))),
		CustomerUUID:          &cUUID,
		BackupStorageInfoList: backupStorageInfoList,
		EnableVerboseLogs:     utils.GetBoolPointer(d.Get("enable_verbose_logs").(bool)),
		AlterLoadBalancer:     utils.GetBoolPointer(d.Get("alter_load_balancer").(bool)),
		DisableChecksum:       utils.GetBoolPointer(d.Get("disable_checksum").(bool)),
		DisableMultipart:      utils.GetBoolPointer(d.Get("disable_multipart").(bool)),
	}

	// Optional KMS config
	if kmsConfigUUID, ok := d.GetOk("kms_config_uuid"); ok {
		req.KmsConfigUUID = utils.GetStringPointer(kmsConfigUUID.(string))
	}

	// Optional PITR timestamp
	if pitrMillis, ok := d.GetOk("restore_to_point_in_time_millis"); ok {
		millis := int64(pitrMillis.(int))
		req.RestoreToPointInTimeMillis = &millis
	}

	// Execute restore, retrying on 409 universe-task conflicts.
	var taskUUID string
	if diags := utils.DispatchAndWait(ctx, "Create Restore", cUUID, c,
		d.Timeout(schema.TimeoutCreate),
		utils.ResourceEntity, "Restore", "Create",
		func() (string, *http.Response, error) {
			r, resp, err := c.BackupsAPI.RestoreBackupV2(ctx, cUUID).Backup(req).Execute()
			if err != nil {
				return "", resp, err
			}
			taskUUID = *r.TaskUUID
			return taskUUID, resp, nil
		},
	); diags != nil {
		return diags
	}

	// Set ID using task UUID since restores don't have a persistent ID
	d.SetId(taskUUID)
	return resourceRestoreRead(ctx, d, meta)
}

func resourceRestoreRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	// Restores are one-time operations and don't have persistent state to read.
	// We just verify the resource exists in local state.
	if d.Id() == "" {
		return diag.Errorf("Restore resource has no ID")
	}

	return diags
}

func resourceRestoreDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	// Restores cannot be "deleted" - they are completed operations.
	// We just remove from Terraform state.
	d.SetId("")
	return nil
}
