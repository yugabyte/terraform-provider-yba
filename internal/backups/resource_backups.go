package backups

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/customer"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

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

		Schema: map[string]*schema.Schema{
			"connection_info": customer.ConnectionInfoSchema(),
			"uni_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The UUID of the universe that this backup schedule targets",
			},
			"cron_expression": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"cron_expression", "frequency"},
				Description:  "A cron expression to use",
			},
			"frequency": {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"cron_expression", "frequency"},
				Description:  "Frequency to run the backup, in milliseconds",
			},
			"keyspace": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Keyspace to backup",
			},
			"storage_config_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the storage configuration to use. Can be retrieved from the storage config data source.",
			},
			"time_before_delete": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Description: "Time before deleting the backup from storage, in milliseconds",
			},
			"sse": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Description: "", // TODO: document
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
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Type of the backup. Permitted values: YQL_TABLE_TYPE, REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE, TRANSACTION_STATUS_TABLE_TYPE",
			},
		},
	}
}

func resourceBackupsCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	req := client.MultiTableBackupRequestParams{
		Keyspace:            utils.GetStringPointer(d.Get("keyspace").(string)),
		StorageConfigUUID:   d.Get("storage_config_uuid").(string),
		TimeBeforeDelete:    utils.GetInt64Pointer(int64(d.Get("time_before_delete").(int))),
		Sse:                 utils.GetBoolPointer(d.Get("sse").(bool)),
		TransactionalBackup: utils.GetBoolPointer(d.Get("transactional_backup").(bool)),
		Parallelism:         utils.GetInt32Pointer(int32(d.Get("parallelism").(int))),
		BackupType:          utils.GetStringPointer(d.Get("backup_type").(string)),
		CronExpression:      utils.GetStringPointer(d.Get("cron_expression").(string)),
		SchedulingFrequency: utils.GetInt64Pointer(int64(d.Get("frequency").(int))),
	}
	r, _, err := c.BackupsApi.CreateMultiTableBackup(ctx, cUUID, d.Get("uni_uuid").(string)).TableBackup(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.ScheduleUUID)
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	r, _, err := c.ScheduleManagementApi.ListSchedules(ctx, cUUID).Execute()
	b, err := findBackup(r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("cron_expression", b.CronExpression); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("frequency", b.Frequency); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func findBackup(backups []client.Schedule, sUUID string) (*client.Schedule, error) {
	for _, b := range backups {
		if *b.ScheduleUUID == sUUID {
			return &b, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("Can't find backup schedule %s", sUUID))
}

func resourceBackupsUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	req := client.EditBackupScheduleParams{
		CronExpression: utils.GetStringPointer(d.Get("cron_expression").(string)),
		Frequency:      utils.GetInt64Pointer(int64(d.Get("frequency").(int))),
	}
	_, _, err := c.ScheduleManagementApi.EditBackupScheduleV2(ctx, cUUID, d.Id()).Body(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	_, _, err := c.ScheduleManagementApi.DeleteSchedule(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
