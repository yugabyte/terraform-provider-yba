package backups

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
				Description: "The UUID of the universe that this backup schedule targets",
			},
			"schedule_name": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Required:    true, //compulsory for V2 schedules
				Description: "Backup schedule name",
			},
			"cron_expression": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"cron_expression", "frequency"},
				Description:  "A cron expression to use",
			},
			"frequency": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"cron_expression", "frequency"},
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// verify if the duration string for frequency represent the
					// same value, if so, ignore diff, else, show difference in plan
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
				Description: "Keyspace to backup",
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
					" duration in the standard format https://pkg.go.dev/time#Duration.",
			},
			"sse": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Description: "Is SSE",
			},
			"transactional_backup": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Description: "Flag for indicating if backup is transactional across tables",
			},
			"parallelism": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Description: "Number of concurrent commands to run on nodes over SSH",
			},
			"backup_type": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"YQL_TABLE_TYPE", "REDIS_TABLE_TYPE", "PGSQL_TABLE_TYPE",
						"TRANSACTION_STATUS_TABLE_TYPE"}, false)),
				Description: "Type of the backup. Permitted values: YQL_TABLE_TYPE, " +
					"REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE, TRANSACTION_STATUS_TABLE_TYPE",
			},
			"table_uuid_list": {
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true,
				Description: "List of Table UUIDs, required if backup_type = REDIS_TABLE_TYPE",
			},
			"delete_backup": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Delete backup while deleting schedule",
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

				if frequency < utils.ConvertUnitToMs(1, "HOURS") {
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
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	var r client.Schedule
	if !allowed {

		return diag.FromErr(fmt.Errorf("Scheduling backups below version 2.17.3.0-b43 is not"+
			" supported, currently on %s", version))

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

	timeBeforeDelete, timeBeforeDeleteUnit, _, err := utils.GetMsFromDurationString(
		d.Get("time_before_delete").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	frequency, frequencyUnit, _, err := utils.GetMsFromDurationString(d.Get("frequency").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	if frequency < utils.ConvertUnitToMs(1, "HOURS") {
		return diag.Errorf("Frequency of backups cannot be less than 1 hour")
	}

	req := client.BackupRequestParams{
		StorageConfigUUID:   d.Get("storage_config_uuid").(string),
		TimeBeforeDelete:    utils.GetInt64Pointer(timeBeforeDelete),
		ExpiryTimeUnit:      utils.GetStringPointer(timeBeforeDeleteUnit),
		Sse:                 utils.GetBoolPointer(d.Get("sse").(bool)),
		Parallelism:         utils.GetInt32Pointer(int32(d.Get("parallelism").(int))),
		BackupType:          utils.GetStringPointer(d.Get("backup_type").(string)),
		CronExpression:      utils.GetStringPointer(d.Get("cron_expression").(string)),
		SchedulingFrequency: utils.GetInt64Pointer(frequency),
		FrequencyTimeUnit:   utils.GetStringPointer(frequencyUnit),
		KeyspaceTableList:   &keyspaceTableList,
		ScheduleName:        utils.GetStringPointer(d.Get("schedule_name").(string)),
		UniverseUUID:        d.Get("universe_uuid").(string),
	}

	// V2 Schedule Backup
	tflog.Info(ctx, fmt.Sprintf("Current version %s, using V2 Create Schedule Backup API",
		version))

	r, _, err = c.BackupsApi.CreatebackupSchedule(ctx, cUUID).Backup(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(r.GetScheduleUUID())
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	var b client.Schedule
	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {
		return diag.FromErr(fmt.Errorf("Reading backups below version 2.17.3.0-b43 is not"+
			" supported, currently on %s", version))
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
	r, _, err := c.ScheduleManagementApi.ListSchedulesV2(ctx, cUUID).
		PageScheduleRequest(req).Execute()
	if err != nil {
		return diag.FromErr(err)
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
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

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

	frequency, frequencyUnit, _, err := utils.GetMsFromDurationString(d.Get("frequency").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	if frequency < utils.ConvertUnitToMs(1, "HOURS") {
		return diag.Errorf("Frequency of backups cannot be less than 1 hour")
	}

	req := client.EditBackupScheduleParams{
		CronExpression:    utils.GetStringPointer(d.Get("cron_expression").(string)),
		Frequency:         utils.GetInt64Pointer(frequency),
		FrequencyTimeUnit: utils.GetStringPointer(frequencyUnit),
	}
	_, _, err = c.ScheduleManagementApi.EditBackupScheduleV2(ctx,
		cUUID, d.Id()).Body(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId
	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed && d.Get("delete_backup").(bool) {
		return diag.FromErr(fmt.Errorf("Deleting backups along with schedules "+
			"below version 2.17.3.0-b43 is not supported, currently on %s", version))
	}
	if d.Get("delete_backup").(bool) {

		backupsList, _, err := c.BackupsApi.ListOfBackups(ctx, cUUID,
			d.Get("universe_uuid").(string)).Execute()
		if err != nil {
			return diag.FromErr(err)
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
			_, _, err = c.BackupsApi.DeleteBackupsV2(ctx, cUUID).DeleteBackup(req).Execute()
			if err != nil {
				return diag.FromErr(err)
			}
			tflog.Info(ctx, fmt.Sprintf("Deleted backups with scheduleUUID %s", d.Id()))
		} else {
			tflog.Info(ctx, fmt.Sprintf("No backups to delete with scheduleUUID %s", d.Id()))
		}
	}
	if !allowed {
		return diag.FromErr(fmt.Errorf("Deleting backup schedules below version 2.17.3.0-b43 "+
			"is not supported, currently on %s", version))
	}
	// V2 schedule delete
	tflog.Info(ctx, fmt.Sprintf("Current version %s, using V2 Delete Schedule Backup API",
		version))

	_, _, err = c.ScheduleManagementApi.DeleteScheduleV2(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
