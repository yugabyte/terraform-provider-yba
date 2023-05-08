// Licensed to Yugabyte, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Apache License, Version 2.0
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
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// ResourceRestore to trigger Restore Operation
func ResourceRestore() *schema.Resource {
	return &schema.Resource{
		Description: "Restoring backups for Universe. This resource does not track the remote " +
			"state and is only provided as a convenience tool. It is recommended to remove this " +
			"resource after running terraform apply.",

		CreateContext: resourceRestoreCreate,
		ReadContext:   resourceRestoreRead,
		UpdateContext: resourceRestoreUpdate,
		DeleteContext: resourceRestoreDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		CustomizeDiff: resourceRestoreDiff(),

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The UUID of the target universe of restore",
			},
			"keyspace": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Target keyspace name",
			},
			"storage_location": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Storage Location of the backup to be restored.",
			},
			"sse": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Is SSE",
			},
			"restore_type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"YQL_TABLE_TYPE", "REDIS_TABLE_TYPE", "PGSQL_TABLE_TYPE",
						"TRANSACTION_STATUS_TABLE_TYPE"}, false)),
				Description: "Type of the restore. Permitted values: YQL_TABLE_TYPE, " +
					"REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE, TRANSACTION_STATUS_TABLE_TYPE",
			},

			"parallelism": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Number of concurrent commands to run on nodes over SSH",
			},
			"storage_config_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "UUID of the storage configuration to use. Can be retrieved" +
					" from the storage config data source.",
			},
		},
	}
}

func resourceRestoreDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateValue("storage_location", func(ctx context.Context, value,
			meta interface{}) error {
			if value.(string) == "" {
				return fmt.Errorf("Cannot have empty storage location for restores")
			}
			return nil
		}),
	)
}

func resourceRestoreCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {
		return diag.FromErr(fmt.Errorf("Restoring backups below version 2.17.3.0-b43 is not"+
			" supported, currently on %s", version))
	}
	backupStorage := client.BackupStorageInfo{
		StorageLocation: utils.GetStringPointer(d.Get("storage_location").(string)),
		Keyspace:        utils.GetStringPointer(d.Get("keyspace").(string)),
		Sse:             utils.GetBoolPointer(d.Get("sse").(bool)),
		BackupType:      utils.GetStringPointer(d.Get("restore_type").(string)),
	}
	backupStorageInfoList := make([]client.BackupStorageInfo, 0)
	backupStorageInfoList = append(backupStorageInfoList, backupStorage)

	req := client.RestoreBackupParams{
		ActionType:            utils.GetStringPointer("RESTORE"),
		UniverseUUID:          d.Get("universe_uuid").(string),
		StorageConfigUUID:     utils.GetStringPointer(d.Get("storage_config_uuid").(string)),
		Parallelism:           utils.GetInt32Pointer(int32(d.Get("parallelism").(int))),
		CustomerUUID:          &cUUID,
		BackupStorageInfoList: &backupStorageInfoList,
	}

	// V2 restore
	r, response, err := c.BackupsApi.RestoreBackupV2(ctx, cUUID).Backup(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Restore", "Create")
		return diag.FromErr(errMessage)
	}

	tflog.Debug(ctx, fmt.Sprintf("Waiting for restore %s to complete", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(d.Get("keyspace").(string))
	return resourceRestoreRead(ctx, d, meta)
}

func resourceRestoreRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {

	// fetch restore from restore table
	var diags diag.Diagnostics
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {
		return diag.FromErr(fmt.Errorf("Reading backup restores below version 2.17.3.0-b43 is not"+
			" supported, currently on %s", version))
	}

	req := client.RestorePagedApiQuery{
		Filter: client.RestoreApiFilter{
			States: *utils.StringSlice(utils.CreateSingletonList("Completed")),
			UniverseUUIDList: *utils.StringSlice(utils.CreateSingletonList(
				d.Get("universe_uuid"))),
		},
		SortBy:    "createTime",
		Direction: "DESC",
		Limit:     *utils.GetInt32Pointer(10),
	}
	var minTime = time.Unix(-2208988800, 0) // Jan 1, 1900
	var maxTime = minTime.Add(1<<63 - 1)

	startDate, err := time.Parse(time.RFC3339, minTime.Format(time.RFC3339))
	if err != nil {
		return diag.FromErr(err)
	}
	req.Filter.DateRangeStart = &startDate

	endDate, err := time.Parse(time.RFC3339, maxTime.Format(time.RFC3339))
	if err != nil {
		return diag.FromErr(err)
	}
	req.Filter.DateRangeEnd = &endDate

	r, response, err := c.BackupsApi.ListBackupRestoresV2(ctx, cUUID).PageRestoresRequest(
		req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Restore", "Read")
		return diag.FromErr(errMessage)
	}

	if len(r.Entities) > 0 {
		chosenRestore := r.Entities[0]
		keyspaceList := chosenRestore.GetRestoreKeyspaceList()
		if len(keyspaceList) > 0 {
			keyspace := keyspaceList[0]
			if err = d.Set("storage_location", keyspace.GetStorageLocation()); err != nil {
				return diag.FromErr(err)
			}
			if err = d.Set("keyspace", keyspace.GetTargetKeyspace()); err != nil {
				return diag.FromErr(err)
			}

		}
		if err = d.Set("universe_uuid", chosenRestore.UniverseUUID); err != nil {
			return diag.FromErr(err)
		}

		return diags
	}

	d.SetId(d.Get("keyspace").(string))
	return diags

}

func resourceRestoreUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	return diag.Diagnostics{}
}

func resourceRestoreDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	d.SetId("")
	return nil
}
