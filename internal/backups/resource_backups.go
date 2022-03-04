package backups

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
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
			"customer_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"uni_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"cron_expression": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"frequency": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"keyspace": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"storage_config_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"time_before_delete": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
			},
			"sse": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
			"transactional_backup": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
			"parallelism": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
			},
			"backup_type": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourceBackupsCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
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

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
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

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	req := client.EditBackupScheduleParams{
		CronExpression: utils.GetStringPointer(d.Get("cron_expression").(string)),
		Frequency:      utils.GetInt64Pointer(int64(d.Get("frequency").(int))),
	}
	_, _, err := c.ScheduleManagementApi.EditBackupSchedule(ctx, cUUID, d.Id()).Body(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	_, _, err := c.ScheduleManagementApi.DeleteSchedule(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
