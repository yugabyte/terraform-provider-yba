package backups

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// Lists fetches the backups within the given set of conditions
func Lists() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve list of backups",

		ReadContext: dataSourceBackupsListRead,

		Schema: map[string]*schema.Schema{
			// accept date range and check backups between that time to be chosen. Pick the latest
			// backup. Accept universe name or uuid to select backup
			"universe_name": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"universe_name", "universe_uuid"},
				Description:  "Latest backup from this universe is stored in the ID",
			},
			"universe_uuid": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"universe_name", "universe_uuid"},
				Description:  "Latest backup from this universe is stored in the ID",
			},
			"date_range_start": {
				Type:             schema.TypeString,
				Optional:         true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.IsRFC3339Time),
				Description: "Start of range for create time of the backup used for " +
					"filtering",
			},

			"date_range_end": {
				Type:             schema.TypeString,
				Optional:         true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.IsRFC3339Time),
				Description:      "End of range for create time of the backup used for filtering",
			},
			"storage_location": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Storage location of the backup",
			},
			"backup_type": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Type of the backup. Permitted values: YQL_TABLE_TYPE, " +
					"REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE, TRANSACTION_STATUS_TABLE_TYPE",
			},
			"storage_config_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the storage configuration used for backup",
			},
		},
	}
}

func dataSourceBackupsListRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId
	var err error

	allowed, version, err := backupYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {
		return diag.FromErr(fmt.Errorf("Listing backups below version 2.17.3.0-b43 is not"+
			" supported, currently on %s", version))
	}
	req := client.BackupPagedApiQuery{
		Filter: client.BackupApiFilter{
			UniverseNameList: *utils.StringSlice(utils.CreateSingletonList(d.Get(
				"universe_name"))),
			UniverseUUIDList: *utils.StringSlice(utils.CreateSingletonList(d.Get(
				"universe_uuid"))),
		},
		SortBy:    "createTime",
		Direction: "DESC",
		Limit:     *utils.GetInt32Pointer(10),
	}

	var minTime = time.Unix(-2208988800, 0) // Jan 1, 1900
	var maxTime = minTime.Add(1<<63 - 1)

	startDateString := d.Get("date_range_start").(string)
	if startDateString != "" {
		startDate, err := time.Parse(time.RFC3339, startDateString)
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeStart = &startDate
	} else {
		startDate, err := time.Parse(time.RFC3339, minTime.Format(time.RFC3339))
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeStart = &startDate
	}

	endDateString := d.Get("date_range_end").(string)
	if endDateString != "" {
		endDate, err := time.Parse(time.RFC3339, endDateString)
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeEnd = &endDate

	} else {
		endDate, err := time.Parse(time.RFC3339, maxTime.Format(time.RFC3339))
		if err != nil {
			return diag.FromErr(err)
		}
		req.Filter.DateRangeEnd = &endDate
	}

	r, _, err := c.BackupsApi.ListBackupsV2(ctx, cUUID).PageBackupsRequest(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	// Get the first entity from r
	if len(r.Entities) > 0 {
		chosenBackup := r.Entities[0]
		err = d.Set("storage_config_uuid", chosenBackup.GetCommonBackupInfo().StorageConfigUUID)
		if err != nil {
			return diag.FromErr(err)
		}
		if err = d.Set("backup_type", chosenBackup.BackupType); err != nil {
			return diag.FromErr(err)
		}
		responseList := chosenBackup.GetCommonBackupInfo().ResponseList
		if len(responseList) > 0 {
			err = d.Set("storage_location", responseList[0].DefaultLocation)
			if err != nil {
				return diag.FromErr(err)
			}
		}
		if err = d.Set("universe_name", chosenBackup.UniverseName); err != nil {
			return diag.FromErr(err)
		}
		if err = d.Set("universe_uuid", chosenBackup.UniverseUUID); err != nil {
			return diag.FromErr(err)
		}
		d.SetId(chosenBackup.CommonBackupInfo.BackupUUID)
		return diags
	}
	d.Set("universe_uuid", d.Get("universe_uuid"))
	d.Set("universe_name", d.Get("universe_name"))

	d.SetId("")
	return diags
}
