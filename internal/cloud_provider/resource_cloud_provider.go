package cloud_provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/customer"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
	"net/http"
	"time"
)

func ResourceCloudProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Cloud Provider Resource",

		CreateContext: resourceCloudProviderCreate,
		ReadContext:   resourceCloudProviderRead,
		UpdateContext: resourceCloudProviderUpdate,
		DeleteContext: resourceCloudProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"connection_info": customer.ConnectionInfoSchema(),
			"active": {
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "Flag indicating if the provider is active",
			},
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
				Type:        schema.TypeMap,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true,
				Optional:    true,
				Description: "Configuration values to be set for the provider. AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY must be set for AWS providers. The contents of your google credentials must be included here for GCP providers. AZURE_SUBSCRIPTION_ID, AZURE_RG, AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET must be set for AZURE providers.",
			},
			"computed_config": {
				Type:        schema.TypeMap,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Optional:    true,
				ForceNew:    true,
				Computed:    true,
				Description: "Same as config field except some additional values may have been returned by the server.",
			},
			"custom_host_cidrs": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
				Description: "", // TODO: document
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
			"hosted_zone_id": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "", // TODO: document
			},
			"hosted_zone_name": {
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
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "User to use for ssh commands",
			},
		},
	}
}

func resourceCloudProviderCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	req := client.Provider{
		AirGapInstall:        utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
		Code:                 utils.GetStringPointer(d.Get("code").(string)),
		Config:               utils.StringMap(d.Get("config").(map[string]interface{})),
		CustomHostCidrs:      utils.StringSlice(d.Get("custom_host_cidrs").([]interface{})),
		DestVpcId:            utils.GetStringPointer(d.Get("dest_vpc_id").(string)),
		HostVpcId:            utils.GetStringPointer(d.Get("host_vpc_id").(string)),
		HostVpcRegion:        utils.GetStringPointer(d.Get("host_vpc_region").(string)),
		HostedZoneId:         utils.GetStringPointer(d.Get("hosted_zone_id").(string)),
		HostedZoneName:       utils.GetStringPointer(d.Get("hosted_zone_name").(string)),
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
	tflog.Debug(ctx, fmt.Sprintf("Waiting for provider %s to be active", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, time.Minute)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceCloudProviderRead(ctx, d, meta)
}

func findProvider(providers []client.Provider, uuid string) (*client.Provider, error) {
	for _, p := range providers {
		if *p.Uuid == uuid {
			return &p, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("could not find provider %s", uuid))
}

func resourceCloudProviderRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	cUUID, token := api.GetConnectionInfo(d)
	ctx = api.SetContextApiKey(ctx, token)
	r, _, err := c.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	p, err := findProvider(r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("active", p.Active); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("air_gap_install", p.AirGapInstall); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("code", p.Code); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("computed_config", p.Config); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("custom_host_cidrs", p.CustomHostCidrs); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("hosted_zone_id", p.HostedZoneId); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("active", p.Active); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("hosted_zone_name", p.HostedZoneName); err != nil {
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

func resourceCloudProviderUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// do nothing; this is here so that we can chang eth api token without forcing recreation
	return diag.Diagnostics{}
}

func resourceCloudProviderDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// TODO: this uses a non-public API
	var diags diag.Diagnostics

	vc := meta.(*api.ApiClient).VanillaClient
	cUUID, token := api.GetConnectionInfo(d)
	pUUID := d.Id()
	_, err := vc.MakeRequest(http.MethodDelete, fmt.Sprintf("api/v1/customers/%s/providers/%s", cUUID, pUUID), nil, token)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
