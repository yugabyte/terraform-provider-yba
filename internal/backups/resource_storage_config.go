package backups

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// ResourceStorageConfig defines the schema to maintain the storage config resources
func ResourceStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Create Storage Configurations" +
			"\nRequires AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY env variables to be set for" +
			" S3 storage config." +
			"\nRequires GOOGLE_APPLICATION_CREDENTIALS env variable for GCS storage config" +
			"\nRequires AZURE_STORAGE_SAS_TOKEN env variable for Azure storage config",

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

		CustomizeDiff: resourceStorageConfigDiff(),

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"S3", "GCS", "AZ", "NFS"}, false)),
				Description: "Name of config provider. Allowed values: S3, GCS, NFS, AZ",
			},
			"data": {
				Type:        schema.TypeMap,
				Computed:    true,
				Description: "Location and Credentials",
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

func resourceStorageConfigDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateValue("name", func(ctx context.Context, value,
			meta interface{}) error {
			errorMessage := "Empty env variable: "
			switch code := value.(string); code {
			case "GCS":
				_, isPresent := os.LookupEnv(utils.GCPCredentialsEnv)
				if !isPresent {
					return fmt.Errorf("%s%s", errorMessage, utils.GCPCredentialsEnv)
				}
			case "S3":
				var errorString string
				_, isPresentAccessKeyID := os.LookupEnv(utils.AWSAccessKeyEnv)
				if !isPresentAccessKeyID {
					errorString = fmt.Sprintf("%s%s ", errorString, utils.AWSAccessKeyEnv)
				}
				_, isPresentSecretAccessKey := os.LookupEnv(utils.AWSSecretAccessKeyEnv)
				if !isPresentSecretAccessKey {
					errorString = fmt.Sprintf("%s%s ", errorString, utils.AWSSecretAccessKeyEnv)
				}
				if !(isPresentAccessKeyID && isPresentSecretAccessKey) {
					errorString = fmt.Sprintf("%s%s", errorMessage, errorString)
					return fmt.Errorf(errorString)
				}
			case "AZ":
				if _, isPresent := os.LookupEnv(utils.AzureStorageSasTokenEnv); !isPresent {
					return fmt.Errorf("%s%s", errorMessage, utils.AzureStorageSasTokenEnv)
				}
			}
			return nil
		}),
	)
}

func buildData(ctx context.Context, d *schema.ResourceData) (map[string]interface{}, error) {
	data := map[string]interface{}{
		"BACKUP_LOCATION": d.Get("backup_location").(string),
	}

	if d.Get("name").(string) == "GCS" {
		gcsCredString, err := utils.GcpGetCredentialsAsString()
		if err != nil {
			return nil, err
		}
		data["GCS_CREDENTIALS_JSON"] = gcsCredString
	}

	if d.Get("name").(string) == "S3" {
		awsCreds, err := utils.AwsCredentialsFromEnv()
		if err != nil {
			return nil, err
		}
		data[utils.AWSAccessKeyEnv] = awsCreds.AccessKeyID
		data[utils.AWSSecretAccessKeyEnv] = awsCreds.SecretAccessKey
	}
	if d.Get("name").(string) == "AZ" {
		azureCreds, err := utils.AzureStorageCredentialsFromEnv()
		if err != nil {
			return nil, err
		}
		data[utils.AzureStorageSasTokenEnv] = azureCreds
	}
	return data, nil
}

func resourceStorageConfigCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
		diag.Diagnostics) {
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

func resourceStorageConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
		diag.Diagnostics) {
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

func findCustomerConfig(configs []client.CustomerConfigUI, uuid string) (
	*client.CustomerConfigUI, error) {
	for _, c := range configs {
		if *c.ConfigUUID == uuid {
			return &c, nil
		}
	}
	return nil, errors.New("Could not find config with id " + uuid)
}

func resourceStorageConfigUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
		diag.Diagnostics) {
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

func resourceStorageConfigDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
		diag.Diagnostics) {
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
