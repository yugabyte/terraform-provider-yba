package cloud_provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// ResourceCloudProvider creates and maintains resource for cloud providers
func ResourceCloudProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Cloud Provider Resource",

		CreateContext: resourceCloudProviderCreate,
		ReadContext:   resourceCloudProviderRead,
		DeleteContext: resourceCloudProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"air_gap_install": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Description: "Flag indicating if the universe should use an air-gapped installation",
			},
			"code": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Code of the cloud provider. Permitted values: gcp, aws, azu",
			},
			"config": {
				Type:     schema.TypeMap,
				Elem:     &schema.Schema{Type: schema.TypeString},
				ForceNew: true,
				Computed: true,
				Description: "Configuration values to be set for the provider. " +
					"AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY must be set for AWS providers." +
					" The contents of your google credentials must be included here for GCP " +
					"providers. AZURE_SUBSCRIPTION_ID, AZURE_RG, AZURE_TENANT_ID, AZURE_CLIENT_ID," +
					" AZURE_CLIENT_SECRET must be set for AZURE providers.",
			},
			"dest_vpc_id": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "", // TODO: document
			},
			"host_vpc_id": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "", // TODO: document
			},
			"host_vpc_region": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "", // TODO: document
			},
			"key_pair_name": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "", // TODO: document
			},
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Name of the provider",
			},
			"regions": RegionsSchema(),
			"ssh_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				Description: "Port to use for ssh commands",
			},
			"ssh_private_key_content": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Private key to use for ssh commands",
			},
			"ssh_user": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// ssh_user field can be empty in the configuration block of the resource
					// In that event YBA uses a default ssh user as per the cloud provider
					// The discrepency of empty field in config file and value filled in state
					// file, we check if ssh user is empty and ignore the difference if true

					return len(old) > 0 && len(new) == 0
				},
				Description: "User to use for ssh commands",
			},
		},
	}
}

func buildConfig(cloudCode string) (map[string]interface{}, error) {
	config := make(map[string]interface{})
	var err error
	if cloudCode == "gcp" {
		config, err = utils.GcpGetCredentialsAsMap()
		if err != nil {
			return nil, err
		}
	} else if cloudCode == "aws" {
		awsCreds, err := utils.AwsCredentialsFromEnv()
		if err != nil {
			return nil, err
		}
		config["AWS_ACCESS_KEY_ID"] = awsCreds.AccessKeyID
		config["AWS_SECRET_ACCESS_KEY"] = awsCreds.SecretAccessKey

	} else if cloudCode == "azu" {
		azureCreds, err := utils.AzureCredentialsFromEnv()
		if err != nil {
			return nil, err
		}
		config["AZURE_CLIENT_ID"] = azureCreds.ClientID
		config["AZURE_CLIENT_SECRET"] = azureCreds.ClientSecret
		config["AZURE_SUBSCRIPTION_ID"] = azureCreds.SubscriptionID
		config["AZURE_TENANT_ID"] = azureCreds.TenantID
		config["AZURE_RG"] = azureCreds.ResourceGroup
	}
	return config, nil
}

func resourceCloudProviderCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	config, err := buildConfig(d.Get("code").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	req := client.Provider{
		AirGapInstall:        utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
		Code:                 utils.GetStringPointer(d.Get("code").(string)),
		Config:               utils.StringMap(config),
		DestVpcId:            utils.GetStringPointer(d.Get("dest_vpc_id").(string)),
		HostVpcId:            utils.GetStringPointer(d.Get("host_vpc_id").(string)),
		HostVpcRegion:        utils.GetStringPointer(d.Get("host_vpc_region").(string)),
		KeyPairName:          utils.GetStringPointer(d.Get("key_pair_name").(string)),
		Name:                 utils.GetStringPointer(d.Get("name").(string)),
		SshPort:              utils.GetInt32Pointer(int32(d.Get("ssh_port").(int))),
		SshPrivateKeyContent: utils.GetStringPointer(d.Get("ssh_private_key_content").(string)),
		SshUser:              utils.GetStringPointer(d.Get("ssh_user").(string)),
		Regions:              buildRegions(d.Get("regions").([]interface{})),
	}
	r, _, err := c.CloudProvidersApi.CreateProviders(ctx, cUUID).CreateProviderRequest(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.ResourceUUID)
	if r.TaskUUID != nil {
		tflog.Debug(ctx, fmt.Sprintf("Waiting for provider %s to be active", d.Id()))
		err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceCloudProviderRead(ctx, d, meta)
}

func findProvider(providers []client.Provider, uuid string) (*client.Provider, error) {
	for _, p := range providers {
		if *p.Uuid == uuid {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("could not find provider %s", uuid)
}

func resourceCloudProviderRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	p, err := findProvider(r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("air_gap_install", p.AirGapInstall); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("code", p.Code); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("config", p.Config); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("name", p.Name); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_port", p.SshPort); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_private_key_content", p.SshPrivateKeyContent); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_user", p.SshUser); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("regions", flattenRegions(p.Regions)); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func resourceCloudProviderDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	pUUID := d.Id()
	_, _, err := c.CloudProvidersApi.Delete(ctx, cUUID, pUUID).Execute()

	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
