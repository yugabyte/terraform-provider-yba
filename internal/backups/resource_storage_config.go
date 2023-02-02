package backups

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
)

func ResourceStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Create Storage Configurations",

		CreateContext: resourceStorageConfigCreate,
		ReadContext:   resourceStorageConfigRead,
		UpdateContext: resourceStorageConfigUpdate,
		DeleteContext: resourceStorageConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of config provider. Allowed values: S3, GCS, NFS, AZ",
			},
			"data": {
				Type:        schema.TypeMap,
				Computed:    true,
				Description: "Location and Credentials",
			},
			"s3_access_key_id": &schema.Schema{
				Type:          schema.TypeString,
				Optional:      true,
				Sensitive:     true,
				ConflictsWith: []string{"gcs_creds_json", "azure_sas_token"},
				RequiredWith:  []string{"s3_secret_access_key"},
				Description:   "S3 Access Key ID",
			},
			"s3_secret_access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: "S3 Secret Access Key",
			},
			"gcs_creds_json": {
				Type:          schema.TypeMap,
				Optional:      true,
				Elem:          schema.TypeString,
				Sensitive:     true,
				ConflictsWith: []string{"s3_access_key_id", "azure_sas_token"},
				Description:   "Credentials for GCS, contents of GCE JSON file",
			},
			"azure_sas_token": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"s3_access_key_id", "gcs_creds_json"},
				Description:   "Credentials for AZURE, requires Azure SAS Token",
			},
			"backup_location": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Backup Location",
			},
			"config_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the Storage Configuration",
			},
		},
	}
}

func buildData(ctx context.Context, d *schema.ResourceData) (map[string]interface{}, error) {
	data := map[string]interface{}{
		"BACKUP_LOCATION": d.Get("backup_location").(string),
	}

	if d.Get("name").(string) == "GCS" {
		if len(d.Get("gcs_creds_json").(map[string]interface{})) == 0 {
			return nil, errors.New(fmt.Sprintf("GCE JSON Credentials not provided when name = 'GCS'"))
		}
		var gcs_cred_string string
		gcs_cred_string = "{ "
		for key, val := range d.Get("gcs_creds_json").(map[string]interface{}) {
			var s string
			if key == "private_key" {
				val_string := strings.Replace(val.(string), "\n", "\\n", -1)
				s = "\"" + key + "\"" + ": " + "\"" + val_string + "\""

			} else {
				s = "\"" + key + "\"" + ": " + "\"" + val.(string) + "\""
			}
			if gcs_cred_string[len(gcs_cred_string)-2] != '{' {
				gcs_cred_string = gcs_cred_string + " , " + s
			} else {
				gcs_cred_string = gcs_cred_string + s
			}
		}
		gcs_cred_string = gcs_cred_string + "}"

		data["GCS_CREDENTIALS_JSON"] = gcs_cred_string
	}

	if d.Get("name").(string) == "S3" {
		if d.Get("s3_access_key_id").(string) == "" {
			return nil, errors.New(fmt.Sprintf("AWS Access Key ID and Secret Key not provided when name = 'S3'"))
		}
		data["AWS_ACCESS_KEY_ID"] = d.Get("s3_access_key_id").(string)
		data["AWS_SECRET_ACCESS_KEY"] = d.Get("s3_secret_access_key").(string)
	}

	if d.Get("name").(string) == "AZ" {
		if d.Get("azure_sas_token").(string) == "" {
			return nil, errors.New(fmt.Sprintf("Azure SAS Token not provided when name = 'AZ'"))
		}
		data["AZURE_STORAGE_SAS_TOKEN"] = d.Get("azure_sas_token").(string)
	}
	return data, nil
}

func resourceStorageConfigCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	// type, name, config name, data [backup__location and credentials]
	data, err := buildData(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}
	req := client.CustomerConfig{
		ConfigName:   d.Get("config_name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         d.Get("name").(string),
		Type:         "STORAGE",
	}
	r, _, err := c.CustomerConfigurationApi.CreateCustomerConfig(ctx, cUUID).Config(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.ConfigUUID)
	return resourceStorageConfigRead(ctx, d, meta)
}

func resourceStorageConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.CustomerConfigurationApi.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		return diag.FromErr(err)
	}
	config, err := findCustomerConfig(r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("config_name", config.ConfigName); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("data", config.Data); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("name", config.Name); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*config.ConfigUUID)
	return diags
}

func findCustomerConfig(configs []client.CustomerConfigUI, uuid string) (*client.CustomerConfigUI, error) {
	for _, c := range configs {
		if *c.ConfigUUID == uuid {
			return &c, nil
		}
	}
	return nil, errors.New("Could not find config with id " + uuid)
}

func resourceStorageConfigUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	data, err := buildData(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   d.Get("config_name").(string),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         d.Get("name").(string),
		Type:         "STORAGE",
	}

	_, _, err = c.CustomerConfigurationApi.EditCustomerConfig(ctx, cUUID, d.Id()).Config(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceStorageConfigRead(ctx, d, meta)
}

func resourceStorageConfigDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	_, _, err := c.CustomerConfigurationApi.DeleteCustomerConfig(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
