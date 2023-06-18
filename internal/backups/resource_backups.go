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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceBackups creates and maintains resource for backup schedules
func ResourceBackups() *schema.Resource {
	return &schema.Resource{
		Description: "Scheduled Backups for Universe",

		CreateContext: resourceBackupsCreate,
		ReadContext:   resourceBackupsRead,
		UpdateContext: resourceBackupsUpdate,
		DeleteContext: resourceBackupsDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Update: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		CustomizeDiff: resourceBackupDiff(),

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The UUID of the universe that this backup schedule targets.",
			},
			"schedule_name": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Required:    true, //compulsory for V2 schedules
				Description: "Backup schedule name.",
			},
			"cron_expression": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"cron_expression", "frequency"},
				Description:  "A cron expression to use.",
			},
			"frequency": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"cron_expression", "frequency"},
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// verify if the duration string for frequency represent the
					// same value, if so, ignore diff, else, show difference in plan
					if old == "0" && new == "" {
						// cron expression is being used
						return true
					}
					oldFrequency, _, _, err := utils.GetMsFromDurationString(old)
					if err != nil {
						if strings.Contains(err.Error(), "missing unit in duration") {
							oldFrequencyInt, err := strconv.Atoi(old)
							if err != nil {
								return false
							}
							oldFrequency = int64(oldFrequencyInt)

						}
					}
					// Frequency in config file must always be in duration format
					newFrequency, _, _, err := utils.GetMsFromDurationString(new)
					if err != nil {
						return false
					}
					if oldFrequency == newFrequency {
						return true
					}
					return false
				},
				Description: "Frequency to run the backup.  Accepts string duration in the" +
					" standard format https://pkg.go.dev/time#Duration.",
			},
			"keyspace": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Keyspace to backup.",
			},
			"storage_config_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "UUID of the storage configuration to use. Can be " +
					"retrieved from the storage config data source.",
			},
			"time_before_delete": {
				Type:     schema.TypeString,
				Optional: true, // If not provided, backups kept indefinitely
				ForceNew: true,
				Description: "Time before deleting the backup from storage. Accepts string" +
					" duration in the standard format https://pkg.go.dev/time#Duration. " +
					"Backups are kept indefinitely if not set.",
			},
			"sse": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Description: "Is SSE.",
			},
			"transactional_backup": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Description: "Flag for indicating if backup is transactional across tables.",
			},
			"parallelism": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Description: "Number of concurrent commands to run on nodes over SSH.",
			},
			"backup_type": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"YQL_TABLE_TYPE", "REDIS_TABLE_TYPE", "PGSQL_TABLE_TYPE"}, false)),
				Description: "Type of the backup. Permitted values: YQL_TABLE_TYPE, " +
					"REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE.",
			},
			"table_uuid_list": {
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true,
				Description: "List of Table UUIDs.",
			},
			"delete_backup": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Delete backup while deleting schedule.",
			},
			"incremental_backup_frequency": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Frequency to take incremental backups.  " +
					"Accepts string duration in the standard format https://pkg.go.dev/time#Duration.",
			},
		},
	}
}

func resourceBackupDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateValue("frequency", func(ctx context.Context, value,
			meta interface{}) error {
			if value.(string) != "" {
				duration := value.(string)
				frequency, _, _, err := utils.GetMsFromDurationString(duration)
				if err != nil {
					return err
				}
				// frequency is 0 when cron expression is set
				if frequency < utils.ConvertUnitToMs(1, "HOURS") && frequency != 0 {
					return errors.New("Frequency of backups cannot be less than 1 hour")
				}
			}
			return nil
		}),
		customdiff.ValidateValue("time_before_delete", func(ctx context.Context, value,
			meta interface{}) error {
			if value.(string) != "" {
				_, err := time.ParseDuration(value.(string))
				if err != nil {
					return fmt.Errorf("Backup Schedule Expiry Time: %w", err)
				}
			}
			return nil
		}),
		customdiff.ValidateValue("incremental_backup_frequency", func(ctx context.Context, value,
			meta interface{}) error {
			if value.(string) != "" {
				duration := value.(string)
				iFrequency, _, _, err := utils.GetMsFromDurationString(duration)
				if err != nil {
					return err
				}

				if iFrequency > utils.ConvertUnitToMs(1, "DAYS") {
					return errors.New("Frequency of incremental backups cannot be more than 1 day")
				}
			}
			return nil
		}),
		customdiff.IfValue("schedule_name",
			func(ctx context.Context, value, meta interface{}) bool {
				// If schedule exists, do not add incremental backup if not enabled
				// do not disable incremental backup if enabled
				return value.(string) != ""
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				oldIncrFreqInterface, newIncrFreqInterface := d.
					GetChange("incremental_backup_frequency")
				if !d.HasChange("schedule_name") {
					if oldIncrFreqInterface.(string) == "" && newIncrFreqInterface.(string) != "" {
						return errors.New("Cannot take incremental backups on existing schedules")
					}
					if oldIncrFreqInterface.(string) != "" && newIncrFreqInterface.(string) == "" {
						return errors.New("Cannot disable incremental backups on existing schedules")
					}
				}
				return nil
			}),
	)
}

func backupYBAVersionCheck(ctx context.Context, c *client.APIClient) (bool, string, error) {
	allowedVersions := []string{utils.YBAAllowBackupMinVersion}
	allowed, version, err := utils.CheckValidYBAVersion(ctx, c, allowedVersions)
	if err != nil {
		return false, "", err
	}
	return allowed, version, err
}

func resourceBackupsCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	var r client.Schedule
	if !allowed {

		return diag.FromErr(fmt.Errorf("Scheduling backups below version %s is not"+
			" supported, currently on %s", utils.YBAAllowBackupMinVersion, version))

	}
	if d.Get("schedule_name").(string) == "" {
		return diag.FromErr(errors.New("V2 Schedules require a name"))
	}
	keyspaceTableList := make([]client.KeyspaceTable, 0)
	keyspaceTable := client.KeyspaceTable{
		Keyspace:      utils.GetStringPointer(d.Get("keyspace").(string)),
		TableUUIDList: utils.StringSlice(d.Get("table_uuid_list").([]interface{})),
	}
	keyspaceTableList = append(keyspaceTableList, keyspaceTable)

	var timeBeforeDelete, frequency, incrementalFrequency int64
	var timeBeforeDeleteUnit, frequencyUnit, incrementalFrequencyUnit string
	var frequencyGiven, incrementalFrequencyGiven bool

	if d.Get("time_before_delete").(string) != "" {
		timeBeforeDelete, timeBeforeDeleteUnit, _, err = utils.GetMsFromDurationString(
			d.Get("time_before_delete").(string))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	if d.Get("frequency").(string) != "" {
		frequency, frequencyUnit, frequencyGiven, err = utils.GetMsFromDurationString(d.
			Get("frequency").(string))
		if err != nil {
			return diag.FromErr(err)
		}

		if frequency < utils.ConvertUnitToMs(1, "HOURS") {
			return diag.Errorf("Frequency of backups cannot be less than 1 hour")
		}
	}

	if d.Get("incremental_backup_frequency").(string) != "" {
		incrementalFrequency, incrementalFrequencyUnit, incrementalFrequencyGiven, err = utils.
			GetMsFromDurationString(d.Get("incremental_backup_frequency").(string))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	if frequencyGiven && incrementalFrequencyGiven {
		if incrementalFrequency < frequency {
			return diag.Errorf("Frequency of incremental " +
				"backups cannot be less than frequency of full backups")
		}
	} else if incrementalFrequencyGiven {
		if incrementalFrequency > utils.ConvertUnitToMs(1, "DAYS") {
			return diag.Errorf("Frequency of incremental backups cannot be more than 1 day")
		}
	}

	req := client.BackupRequestParams{
		StorageConfigUUID:                  d.Get("storage_config_uuid").(string),
		TimeBeforeDelete:                   utils.GetInt64Pointer(timeBeforeDelete),
		ExpiryTimeUnit:                     utils.GetStringPointer(timeBeforeDeleteUnit),
		Sse:                                utils.GetBoolPointer(d.Get("sse").(bool)),
		Parallelism:                        utils.GetInt32Pointer(int32(d.Get("parallelism").(int))),
		BackupType:                         utils.GetStringPointer(d.Get("backup_type").(string)),
		CronExpression:                     utils.GetStringPointer(d.Get("cron_expression").(string)),
		SchedulingFrequency:                utils.GetInt64Pointer(frequency),
		FrequencyTimeUnit:                  utils.GetStringPointer(frequencyUnit),
		KeyspaceTableList:                  &keyspaceTableList,
		ScheduleName:                       utils.GetStringPointer(d.Get("schedule_name").(string)),
		UniverseUUID:                       d.Get("universe_uuid").(string),
		IncrementalBackupFrequency:         utils.GetInt64Pointer(incrementalFrequency),
		IncrementalBackupFrequencyTimeUnit: utils.GetStringPointer(incrementalFrequencyUnit),
	}

	// V2 Schedule Backup
	tflog.Info(ctx, fmt.Sprintf("Current version %s, using V2 Create Schedule Backup API",
		version))

	var response *http.Response
	r, response, err = c.BackupsApi.CreatebackupSchedule(ctx, cUUID).Backup(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backups", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(r.GetScheduleUUID())
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	var b client.Schedule
	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {
		return diag.FromErr(fmt.Errorf("Reading backups below version %s is not"+
			" supported, currently on %s", utils.YBAAllowBackupMinVersion, version))
	}
	// V2 schedule list
	req := client.SchedulePagedApiQuery{
		SortBy:    "taskType",
		Direction: "DESC",
		Limit:     *utils.GetInt32Pointer(10),
		Filter: client.ScheduleApiFilter{
			Status: *utils.StringSlice(utils.CreateSingletonList("Active")),
			UniverseUUIDList: *utils.StringSlice(utils.CreateSingletonList(
				d.Get("universe_uuid"))),
		},
	}
	r, response, err := c.ScheduleManagementApi.ListSchedulesV2(ctx, cUUID).
		PageScheduleRequest(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backups", "Read")
		return diag.FromErr(errMessage)
	}
	tflog.Info(ctx, fmt.Sprintf("Current version %s, using V2 Read Schedule Backup API", version))
	b, err = findBackup(r.Entities, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("cron_expression", b.CronExpression); err != nil {
		return diag.FromErr(err)
	}

	frequencyString := fmt.Sprintf("%v", b.GetFrequency())
	if err = d.Set("frequency", frequencyString); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func findBackup(backups []client.Schedule, sUUID string) (client.Schedule, error) {
	for _, b := range backups {
		if b.GetScheduleUUID() == sUUID {
			return b, nil
		}
	}
	return client.Schedule{}, fmt.Errorf("Can't find backup schedule %s", sUUID)
}

func resourceBackupsUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {
		// since a change was introduced in 2.13.2 which requires an extra field for this API,
		// and that change was made after the generation of the platform-go-client, Updating
		// Backup schedules are currently disabled

		tflog.Error(ctx, fmt.Sprintf("Editing Backup Schedule is currently not supported for"+
			" version %s", version))
		if d.HasChange("delete_backup") {
			return resourceBackupsRead(ctx, d, meta)
		}
	}

	tflog.Info(ctx, fmt.Sprintf("Current version %s, using V2 Edit Schedule Backup API", version))

	if d.HasChange("frequency") || d.HasChange("cron_expression") ||
		d.HasChange("incremental_backup_frequency") {

		var frequency, incrementalFrequency int64
		var frequencyUnit, incrementalFrequencyUnit string
		var frequencyGiven, incrementalFrequencyGiven bool

		if d.Get("frequency") != "" && d.Get("frequency") != "0" {
			frequency, frequencyUnit, frequencyGiven, err = utils.
				GetMsFromDurationString(d.Get("frequency").(string))
			if err != nil {
				return diag.FromErr(err)
			}
			if frequency < utils.ConvertUnitToMs(1, "HOURS") {
				return diag.Errorf("Frequency of backups cannot be less than 1 hour")
			}
		}

		if d.Get("incremental_backup_frequency") != "" {
			incrementalFrequency, incrementalFrequencyUnit, incrementalFrequencyGiven, err = utils.
				GetMsFromDurationString(d.Get("incremental_backup_frequency").(string))
			if err != nil {
				return diag.FromErr(err)
			}
		}

		if frequencyGiven && incrementalFrequencyGiven {
			if incrementalFrequency < frequency {
				return diag.Errorf(
					"Frequency of incremental backups cannot be less than frequency of full backups")
			}
		} else if incrementalFrequencyGiven {
			if incrementalFrequency > utils.ConvertUnitToMs(1, "DAYS") {
				return diag.Errorf("Frequency of incremental backups cannot be more than 1 day")
			}
		}

		req := client.EditBackupScheduleParams{
			CronExpression: utils.GetStringPointer(
				d.Get("cron_expression").(string)),
			Frequency:                          utils.GetInt64Pointer(frequency),
			FrequencyTimeUnit:                  utils.GetStringPointer(frequencyUnit),
			IncrementalBackupFrequency:         utils.GetInt64Pointer(incrementalFrequency),
			IncrementalBackupFrequencyTimeUnit: utils.GetStringPointer(incrementalFrequencyUnit),
		}
		_, response, err := c.ScheduleManagementApi.EditBackupScheduleV2(ctx,
			cUUID, d.Id()).Body(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Backups", "Update")
			return diag.FromErr(errMessage)
		}
	}
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed && d.Get("delete_backup").(bool) {
		return diag.FromErr(fmt.Errorf("Deleting backups along with schedules "+
			"below version %s is not supported, currently on %s", utils.YBAAllowBackupMinVersion, version))
	}
	if d.Get("delete_backup").(bool) {

		backupsList, response, err := c.BackupsApi.ListOfBackups(ctx, cUUID,
			d.Get("universe_uuid").(string)).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Backups", "Delete - Fetch Backups")
			return diag.FromErr(errMessage)
		}

		var req client.DeleteBackupParams
		deleteBackupInfoList := make([]client.DeleteBackupInfo, 0)
		for _, b := range backupsList {
			if b.GetScheduleUUID() == d.Id() {
				deleteBackupInfo := client.DeleteBackupInfo{
					BackupUUID:        b.GetBackupUUID(),
					StorageConfigUUID: utils.GetStringPointer(b.GetStorageConfigUUID()),
				}
				deleteBackupInfoList = append(deleteBackupInfoList, deleteBackupInfo)

			}
		}
		if len(deleteBackupInfoList) > 0 {
			req = client.DeleteBackupParams{
				DeleteBackupInfos: deleteBackupInfoList,
			}
			_, response, err = c.BackupsApi.DeleteBackupsV2(ctx, cUUID).DeleteBackup(req).Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
					"Backups", "Delete - Associated backups")
				return diag.FromErr(errMessage)
			}
			tflog.Info(ctx, fmt.Sprintf("Deleted backups with scheduleUUID %s", d.Id()))
		} else {
			tflog.Info(ctx, fmt.Sprintf("No backups to delete with scheduleUUID %s", d.Id()))
		}
	}
	if !allowed {
		return diag.FromErr(fmt.Errorf("Deleting backup schedules below version %s "+
			"is not supported, currently on %s", utils.YBAAllowBackupMinVersion, version))
	}
	// V2 schedule delete
	tflog.Info(ctx, fmt.Sprintf("Current version %s, using V2 Delete Schedule Backup API",
		version))

	_, response, err := c.ScheduleManagementApi.DeleteScheduleV2(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backups", "Delete")
		return diag.FromErr(errMessage)
	}

	d.SetId("")
	return nil
}
