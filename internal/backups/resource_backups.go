package backups

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/backup_schedule_management"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/backups"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/models"
)

func ResourceBackups() *schema.Resource {
	return &schema.Resource{
		Description: "Scheduled Backups for Universe",

		CreateContext: resourceBackupsCreate,
		ReadContext:   resourceBackupsRead,
		DeleteContext: resourceBackupsDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"uni_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"action_type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
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
	tb := &models.MultiTableBackupRequestParams{
		ActionType:          utils.GetStringPointer(d.Get("action_type").(string)),
		Keyspace:            d.Get("keyspace").(string),
		StorageConfigUUID:   utils.GetUUIDPointer(d.Get("storage_config_uuid").(string)),
		TimeBeforeDelete:    int64(d.Get("time_before_delete").(int)),
		Sse:                 d.Get("sse").(bool),
		TransactionalBackup: d.Get("transactional_backup").(bool),
		Parallelism:         int32(d.Get("parallelism").(int)),
		BackupType:          d.Get("backup_type").(string),
	}
	r, err := c.PlatformAPIs.Backups.CreateMultiTableBackup(
		&backups.CreateMultiTableBackupParams{
			TableBackup: tb,
			CUUID:       c.CustomerUUID(),
			UniUUID:     strfmt.UUID(d.Get("uni_uuid").(string)),
			Context:     ctx,
			HTTPClient:  c.Session(),
		},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(string(r.Payload.ScheduleUUID))
	return resourceBackupsRead(ctx, d, meta)
}

func resourceBackupsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	r, err := c.PlatformAPIs.BackupScheduleManagement.ListBackupSchedules(
		&backup_schedule_management.ListBackupSchedulesParams{
			CUUID:      c.CustomerUUID(),
			Context:    ctx,
			HTTPClient: c.Session(),
		},
		c.SwaggerAuth,
	)
	b, err := findBackup(r.Payload, strfmt.UUID(d.Id()))
	if err != nil {
		return diag.FromErr(err)
	}

	s := b.TaskParams
	if err = d.Set("action_type", s.ActionType); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("keyspace", s.Keyspace); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("storage_config_uuid", s.StorageConfigUUID); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("time_before_delete", s.TimeBeforeDelete); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("sse", s.Sse); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("transactional_backup", s.TransactionalBackup); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("parallelism", s.Parallelism); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("backup_type", s.BackupType); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func findBackup(backups []*models.Schedule, sUUID strfmt.UUID) (*models.Schedule, error) {
	for _, b := range backups {
		if b.ScheduleUUID == sUUID {
			return b, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("Can't find backup schedule %s", sUUID))
}

func resourceBackupsDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
	_, err := c.PlatformAPIs.BackupScheduleManagement.DeleteBackupSchedule(
		&backup_schedule_management.DeleteBackupScheduleParams{
			CUUID:      c.CustomerUUID(),
			SUUID:      strfmt.UUID(d.Id()),
			Context:    ctx,
			HTTPClient: c.Session(),
		},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
