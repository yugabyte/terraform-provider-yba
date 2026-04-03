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

// ResourceBackupSchedule creates and maintains resource for backup schedules
func ResourceBackupSchedule() *schema.Resource {
	return &schema.Resource{
		Description: "Backup schedule for a YugabyteDB Anywhere universe. " +
			"Configures automated backups at specified intervals " +
			"using cron expressions or frequency settings.",

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
					var oldFrequency int64
					oldDuration, _, _, err := utils.GetMsFromDurationString(old)
					if err != nil {
						// State might have raw milliseconds (e.g., "3600000" instead of "1h")
						if strings.Contains(err.Error(), "missing unit in duration") {
							oldFrequencyInt, parseErr := strconv.ParseInt(old, 10, 64)
							if parseErr != nil {
								return false
							}
							oldFrequency = oldFrequencyInt
						} else {
							return false
						}
					} else {
						oldFrequency = oldDuration
					}
					// Frequency in config file must always be in duration format
					newFrequency, _, _, err := utils.GetMsFromDurationString(new)
					if err != nil {
						return false
					}
					return oldFrequency == newFrequency
				},
				Description: "Frequency to run the backup.  Accepts string duration in the " +
					"standard format <https://pkg.go.dev/time#Duration>.",
			},
			"keyspaces": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "List of keyspaces (YCQL) or databases (YSQL) to back up on each run. " +
					"If empty or not specified, performs a full universe backup of all databases/keyspaces. " +
					"For YSQL, each entry is a database name. For YCQL, each entry is a keyspace name.",
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
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if old == "" || new == "" {
						return old == new
					}
					oldDuration, err := time.ParseDuration(old)
					if err != nil {
						return false
					}
					newDuration, err := time.ParseDuration(new)
					if err != nil {
						return false
					}
					return oldDuration == newDuration
				},
				Description: "Time before deleting the backup from storage. Accepts " +
					"string duration in the standard format <https://pkg.go.dev/time#Duration>. " +
					"Backups are kept indefinitely if not set.",
			},
			"sse": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Description: "Is SSE.",
			},
			"transactional_backup": {
				Type:          schema.TypeBool,
				Optional:      true,
				ForceNew:      true,
				Deprecated:    "Deprecated in the YBA API. Use table_by_table_backup instead.",
				ConflictsWith: []string{"table_by_table_backup"},
				Description:   "Deprecated in the YBA API. Use table_by_table_backup instead.",
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
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				ForceNew: true,
				Description: "List of specific table UUIDs to backup. " +
					"Only applicable when a single keyspace is specified in 'keyspaces'. " +
					"If 'keyspaces' has multiple entries, this field is ignored.",
			},
			"delete_backup": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Delete backup while deleting schedule. False by default.",
			},
			"incremental_backup_frequency": {
				Type:     schema.TypeString,
				Optional: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if old == "" || new == "" {
						return old == new
					}
					oldDuration, err := time.ParseDuration(old)
					if err != nil {
						return false
					}
					newDuration, err := time.ParseDuration(new)
					if err != nil {
						return false
					}
					return oldDuration == newDuration
				},
				Description: "Frequency to take incremental backups. " +
					"Accepts string duration in the standard format <https://pkg.go.dev/time#Duration>.",
			},
			"kms_config_uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "KMS configuration UUID for encrypted backups.",
			},
			"enable_point_in_time_restore": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Enable Point-In-Time-Restore capability. Only for YBC-enabled universes.",
			},
			"use_tablespaces": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Include tablespaces information in backup.",
			},
			"use_roles": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Backup global YSQL roles.",
			},
			"min_num_backups_to_retain": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Description: "Minimum number of backups to retain for this schedule.",
			},
			"table_by_table_backup": {
				Type:          schema.TypeBool,
				Optional:      true,
				ForceNew:      true,
				Default:       false,
				ConflictsWith: []string{"transactional_backup"},
				Description:   "Take table-by-table backups. Conflicts with transactional_backup.",
			},
			"parallel_db_backups": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Description: "Number of parallel DB backups.",
			},
			"use_local_timezone": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Use local timezone for cron expression, otherwise use UTC.",
			},
			"enabled": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Whether the backup schedule is enabled. Set to false to pause the schedule.",
			},
			"run_immediate_backup_on_resume": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "When resuming a paused schedule (setting enabled=true), " +
					"run a full or incremental backup immediately instead of waiting for " +
					"the next scheduled time.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Current status of the schedule: Active, Paused, or Stopped.",
			},
		},
	}
}

// ResourceBackupsDeprecated returns the deprecated yba_backups resource.
// Deprecated: Use ResourceBackupSchedule (yba_backup_schedule) instead.
func ResourceBackupsDeprecated() *schema.Resource {
	r := ResourceBackupSchedule()
	r.DeprecationMessage = "yba_backups is deprecated. Use yba_backup_schedule instead. " +
		"To migrate existing state, run: terraform state mv yba_backups.<name> yba_backup_schedule.<name>"
	return r
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
						return errors.New(
							"Cannot disable incremental backups on existing schedules",
						)
					}
				}
				return nil
			}),
		// Validate PITR requires incremental backups
		customdiff.IfValue("enable_point_in_time_restore",
			func(ctx context.Context, value, meta interface{}) bool {
				return value.(bool)
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				incrFreq := d.Get("incremental_backup_frequency").(string)
				if incrFreq == "" {
					return errors.New("enable_point_in_time_restore requires " +
						"incremental_backup_frequency to be set")
				}
				return nil
			}),
		// Validate min_num_backups_to_retain is positive (only if explicitly set)
		customdiff.ValidateValue("min_num_backups_to_retain", func(ctx context.Context, value,
			meta interface{}) error {
			v := value.(int)
			// Skip validation if not set (zero value) - allows imports to work
			if v == 0 {
				return nil
			}
			if v < 1 {
				return errors.New("min_num_backups_to_retain must be at least 1")
			}
			return nil
		}),
		// Validate parallel_db_backups is positive (only if explicitly set)
		customdiff.ValidateValue("parallel_db_backups", func(ctx context.Context, value,
			meta interface{}) error {
			v := value.(int)
			// Skip validation if not set (zero value) - allows imports to work
			if v == 0 {
				return nil
			}
			if v < 1 {
				return errors.New("parallel_db_backups must be at least 1")
			}
			return nil
		}),
	)
}

func resourceBackupsCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	if d.Get("schedule_name").(string) == "" {
		return diag.FromErr(errors.New("V2 Schedules require a name"))
	}
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

	var timeBeforeDelete, frequency, incrementalFrequency int64
	var timeBeforeDeleteUnit, frequencyUnit, incrementalFrequencyUnit string
	var frequencyGiven, incrementalFrequencyGiven bool
	var err error

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
		if incrementalFrequency > frequency {
			return diag.Errorf("Frequency of incremental " +
				"backups cannot be more than frequency of full backups")
		}
	} else if incrementalFrequencyGiven {
		if incrementalFrequency > utils.ConvertUnitToMs(1, "DAYS") {
			return diag.Errorf("Frequency of incremental backups cannot be more than 1 day")
		}
	}

	req := client.BackupRequestParams{
		StorageConfigUUID: d.Get("storage_config_uuid").(string),
		CustomerUUID:      &cUUID,
		TimeBeforeDelete:  utils.GetInt64Pointer(timeBeforeDelete),
		ExpiryTimeUnit:    utils.GetStringPointer(timeBeforeDeleteUnit),
		Sse:               utils.GetBoolPointer(d.Get("sse").(bool)),
		Parallelism: utils.GetInt32Pointer(
			int32(d.Get("parallelism").(int)),
		),
		BackupType: utils.GetStringPointer(d.Get("backup_type").(string)),
		CronExpression: utils.GetStringPointer(
			d.Get("cron_expression").(string),
		),
		SchedulingFrequency:                utils.GetInt64Pointer(frequency),
		FrequencyTimeUnit:                  utils.GetStringPointer(frequencyUnit),
		KeyspaceTableList:                  keyspaceTableList,
		ScheduleName:                       utils.GetStringPointer(d.Get("schedule_name").(string)),
		UniverseUUID:                       d.Get("universe_uuid").(string),
		IncrementalBackupFrequency:         utils.GetInt64Pointer(incrementalFrequency),
		IncrementalBackupFrequencyTimeUnit: utils.GetStringPointer(incrementalFrequencyUnit),
		// New fields
		KmsConfigUUID: utils.GetStringPointer(d.Get("kms_config_uuid").(string)),
		EnablePointInTimeRestore: utils.GetBoolPointer(
			d.Get("enable_point_in_time_restore").(bool),
		),
		UseTablespaces: utils.GetBoolPointer(d.Get("use_tablespaces").(bool)),
		UseRoles:       utils.GetBoolPointer(d.Get("use_roles").(bool)),
		MinNumBackupsToRetain: utils.GetInt32Pointer(
			int32(d.Get("min_num_backups_to_retain").(int)),
		),
		TableByTableBackup: utils.GetBoolPointer(d.Get("table_by_table_backup").(bool)),
		ParallelDBBackups:  utils.GetInt32Pointer(int32(d.Get("parallel_db_backups").(int))),
		UseLocalTimezone:   utils.GetBoolPointer(d.Get("use_local_timezone").(bool)),
	}

	// Create schedule async
	universeUUID := d.Get("universe_uuid").(string)
	scheduleName := d.Get("schedule_name").(string)
	tflog.Info(
		ctx,
		fmt.Sprintf("Creating backup schedule %s for universe %s", scheduleName, universeUUID),
	)

	var response *http.Response
	r, response, err := c.BackupsAPI.CreateBackupScheduleAsync(ctx, cUUID).Backup(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backup Schedule", "Create")
		return diag.FromErr(errMessage)
	}

	taskUUID := r.GetTaskUUID()
	if taskUUID == "" {
		return diag.Errorf("no task UUID returned from create backup schedule async API")
	}

	tflog.Info(ctx, fmt.Sprintf("Waiting for backup schedule creation task %s", taskUUID))
	err = utils.WaitForTask(ctx, taskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		return diag.FromErr(fmt.Errorf("backup schedule creation failed: %w", err))
	}

	// The async API doesn't return resourceUUID, so find the schedule by name
	// Schedule names are unique within a universe
	scheduleUUID := r.GetResourceUUID()
	if scheduleUUID == "" {
		scheduleUUID, err = findScheduleUUIDByName(ctx, c, cUUID, universeUUID, scheduleName)
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to find created schedule: %w", err))
		}
	}

	d.SetId(scheduleUUID)
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// Build filter - include both Active and Stopped statuses
	filter := client.ScheduleApiFilter{
		Status: []string{"Active", "Stopped"},
	}

	// Only filter by universe_uuid if it's known (not during import)
	universeUUID := d.Get("universe_uuid").(string)
	if universeUUID != "" {
		filter.UniverseUUIDList = *utils.StringSlice(utils.CreateSingletonList(universeUUID))
	}

	// Paginate through all schedules to find the one we're looking for
	var b *client.ScheduleResp
	const pageSize int32 = 100
	var offset int32 = 0

	for {
		req := client.SchedulePagedApiQuery{
			SortBy:    "scheduleName",
			Direction: "ASC",
			Limit:     pageSize,
			Offset:    offset,
			Filter:    filter,
		}

		r, response, err := c.ScheduleManagementAPI.ListSchedulesV2(ctx, cUUID).
			PageScheduleRequest(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Backup Schedule", "Read")
			return diag.FromErr(errMessage)
		}

		// Search for the schedule in current page
		for i := range r.Entities {
			if r.Entities[i].ScheduleUUID == d.Id() {
				b = &r.Entities[i]
				break
			}
		}

		// Found the schedule or no more pages
		if b != nil || !r.GetHasNext() {
			break
		}

		offset += pageSize
	}

	if b == nil {
		// Schedule not found - remove from state
		tflog.Warn(
			ctx,
			fmt.Sprintf("Backup Schedule %s not found, removing from state", d.Id()),
		)
		d.SetId("")
		return diags
	}

	var err error
	if err = d.Set("cron_expression", b.CronExpression); err != nil {
		return diag.FromErr(err)
	}

	frequencyString := fmt.Sprintf("%v", b.Frequency)
	if err = d.Set("frequency", frequencyString); err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("schedule_name", b.ScheduleName); err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("use_local_timezone", b.UseLocalTimezone); err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("status", b.Status); err != nil {
		return diag.FromErr(err)
	}

	// Set enabled based on status (Active = enabled, Paused/Stopped = disabled)
	enabled := b.Status == "Active"
	if err = d.Set("enabled", enabled); err != nil {
		return diag.FromErr(err)
	}

	// Set universe_uuid from the schedule response (needed for imports)
	if b.BackupInfo.UniverseUUID != "" {
		if err = d.Set("universe_uuid", b.BackupInfo.UniverseUUID); err != nil {
			return diag.FromErr(err)
		}
	}

	// Set fields from BackupInfo (needed for imports to avoid replacement)
	if err = d.Set("backup_type", b.BackupInfo.BackupType); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("storage_config_uuid", b.BackupInfo.StorageConfigUUID); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("parallelism", int(b.BackupInfo.Parallelism)); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("use_tablespaces", b.BackupInfo.UseTablespaces); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("use_roles", b.BackupInfo.UseRoles); err != nil {
		return diag.FromErr(err)
	}
	pitrEnabled := b.BackupInfo.PointInTimeRestoreEnabled
	if err = d.Set("enable_point_in_time_restore", pitrEnabled); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("table_by_table_backup", b.TableByTableBackup); err != nil {
		return diag.FromErr(err)
	}

	// Set time_before_delete if present (convert ms to duration string)
	if b.BackupInfo.TimeBeforeDelete > 0 {
		tbdMs := b.BackupInfo.TimeBeforeDelete
		tbdDuration := (time.Duration(tbdMs) * time.Millisecond).String()
		if err = d.Set("time_before_delete", tbdDuration); err != nil {
			return diag.FromErr(err)
		}
	}

	// Set incremental backup frequency if present (convert ms to duration string)
	if b.IncrementalBackupFrequency > 0 {
		incrFreqDuration := (time.Duration(b.IncrementalBackupFrequency) * time.Millisecond).String()
		if err = d.Set("incremental_backup_frequency", incrFreqDuration); err != nil {
			return diag.FromErr(err)
		}
	}

	return diags
}

func findScheduleUUIDByName(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	universeUUID string,
	scheduleName string,
) (string, error) {
	req := client.SchedulePagedApiQuery{
		SortBy:    "scheduleName",
		Direction: "ASC",
		Limit:     *utils.GetInt32Pointer(500),
		Filter: client.ScheduleApiFilter{
			UniverseUUIDList: *utils.StringSlice(utils.CreateSingletonList(universeUUID)),
		},
	}
	r, _, err := c.ScheduleManagementAPI.ListSchedulesV2(ctx, cUUID).
		PageScheduleRequest(req).Execute()
	if err != nil {
		return "", fmt.Errorf("failed to list schedules: %w", err)
	}

	for _, s := range r.Entities {
		if s.ScheduleName == scheduleName {
			return s.ScheduleUUID, nil
		}
	}
	return "", fmt.Errorf(
		"schedule with name %s not found in universe %s",
		scheduleName,
		universeUUID,
	)
}

func resourceBackupsUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	universeUUID := d.Get("universe_uuid").(string)

	var err error

	// Handle enable/disable (pause/resume) of schedule
	if d.HasChange("enabled") {
		enabled := d.Get("enabled").(bool)
		runImmediate := d.Get("run_immediate_backup_on_resume").(bool)

		var status string
		if enabled {
			status = "Active"
		} else {
			status = "Stopped"
		}

		toggleReq := client.BackupScheduleToggleParams{
			Status:                     status,
			RunImmediateBackupOnResume: utils.GetBoolPointer(runImmediate),
		}

		tflog.Info(ctx, fmt.Sprintf("Toggling backup schedule %s to %s", d.Id(), status))

		_, response, err := c.ScheduleManagementAPI.ToggleBackupSchedule(
			ctx, cUUID, universeUUID, d.Id()).Body(toggleReq).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Backup Schedule", "Toggle")
			return diag.FromErr(errMessage)
		}
	}

	if d.HasChange("frequency") || d.HasChange("cron_expression") ||
		d.HasChange("incremental_backup_frequency") || d.HasChange("time_before_delete") {

		var frequency, incrementalFrequency, timeBeforeDelete int64
		var frequencyUnit, incrementalFrequencyUnit string
		var frequencyGiven, incrementalFrequencyGiven bool

		if d.Get("frequency") != "" && d.Get("frequency") != "0" {
			if d.HasChange("frequency") {
				frequency, frequencyUnit, frequencyGiven, err = utils.
					GetMsFromDurationString(d.Get("frequency").(string))
				if err != nil {
					return diag.FromErr(err)
				}
				if frequency < utils.ConvertUnitToMs(1, "HOURS") {
					return diag.Errorf("Frequency of backups cannot be less than 1 hour")
				}
			} else {
				r, response, err := c.ScheduleManagementAPI.GetSchedule(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Backups", "Update - Fetch Backup Schedule")
					return diag.FromErr(errMessage)
				}
				frequency = r.GetFrequency()
				frequencyGiven = true
				frequencyUnit = r.GetFrequencyTimeUnit()
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
			if incrementalFrequency > frequency {
				return diag.Errorf(
					"Frequency of incremental backups cannot be more than frequency of full backups",
				)
			}
		} else if incrementalFrequencyGiven {
			if incrementalFrequency > utils.ConvertUnitToMs(1, "DAYS") {
				return diag.Errorf("Frequency of incremental backups cannot be more than 1 day")
			}
		}

		if d.Get("time_before_delete").(string) != "" {
			timeBeforeDelete, _, _, err = utils.GetMsFromDurationString(
				d.Get("time_before_delete").(string))
			if err != nil {
				return diag.FromErr(err)
			}
		}

		req := client.BackupScheduleEditParams{
			CronExpression: utils.GetStringPointer(
				d.Get("cron_expression").(string)),
			SchedulingFrequency:                utils.GetInt64Pointer(frequency),
			FrequencyTimeUnit:                  utils.GetStringPointer(frequencyUnit),
			IncrementalBackupFrequency:         utils.GetInt64Pointer(incrementalFrequency),
			IncrementalBackupFrequencyTimeUnit: utils.GetStringPointer(incrementalFrequencyUnit),
			TimeBeforeDelete:                   utils.GetInt64Pointer(timeBeforeDelete),
		}

		tflog.Info(ctx, fmt.Sprintf("Updating backup schedule %s", d.Id()))

		r, response, err := c.ScheduleManagementAPI.EditBackupScheduleAsync(
			ctx, cUUID, universeUUID, d.Id()).Body(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Backup Schedule", "Update")
			return diag.FromErr(errMessage)
		}

		taskUUID := r.GetTaskUUID()
		if taskUUID == "" {
			return diag.Errorf("no task UUID returned from edit backup schedule async API")
		}

		tflog.Info(ctx, fmt.Sprintf("Waiting for backup schedule update task %s", taskUUID))
		err = utils.WaitForTask(ctx, taskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			return diag.FromErr(fmt.Errorf("backup schedule update failed: %w", err))
		}
	}
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	if d.Get("delete_backup").(bool) {
		backupsList, response, err := c.BackupsAPI.ListOfBackups(ctx, cUUID,
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
			_, response, err = c.BackupsAPI.DeleteBackupsV2(ctx, cUUID).DeleteBackup(req).Execute()
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

	// Delete schedule async
	universeUUID := d.Get("universe_uuid").(string)
	tflog.Info(ctx, fmt.Sprintf("Deleting backup schedule %s", d.Id()))

	r, response, err := c.ScheduleManagementAPI.DeleteBackupScheduleAsync(
		ctx, cUUID, universeUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Backup Schedule", "Delete")
		return diag.FromErr(errMessage)
	}

	taskUUID := r.GetTaskUUID()
	if taskUUID == "" {
		return diag.Errorf("no task UUID returned from delete backup schedule async API")
	}

	tflog.Info(ctx, fmt.Sprintf("Waiting for backup schedule deletion task %s", taskUUID))
	err = utils.WaitForTask(ctx, taskUUID, cUUID, c, d.Timeout(schema.TimeoutDelete))
	if err != nil {
		return diag.FromErr(fmt.Errorf("backup schedule deletion failed: %w", err))
	}

	d.SetId("")
	return nil
}
