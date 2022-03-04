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
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
	"net/http"
	"time"
)

func ResourceCloudProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Cloud Provider Resource",

		CreateContext: resourceCloudProviderCreate,
		ReadContext:   resourceCloudProviderRead,
		DeleteContext: resourceCloudProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"customer_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"active": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"air_gap_install": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},
			"code": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"config":          ConfigSchema(),
			"computed_config": ComputedConfigSchema(),
			"custom_host_cidrs": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"dest_vpc_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"host_vpc_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"host_vpc_region": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"hosted_zone_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"hosted_zone_name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"key_pair_name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"regions": RegionsSchema(),
			"ssh_port": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
			},
			"ssh_private_key_content": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"ssh_user": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourceCloudProviderCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	tflog.Debug(ctx, ctx.Value("apiKeys").(map[string]client.APIKey)["apiKeyAuth"].Key)
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

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
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

func resourceCloudProviderDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// TODO: this uses a non-public API
	var diags diag.Diagnostics

	vc := meta.(*api.ApiClient).VanillaClient
	cUUID := d.Get("customer_id").(string)
	pUUID := d.Id()
	_, err := vc.MakeRequest(http.MethodDelete, fmt.Sprintf("api/v1/customers/%s/providers/%s", cUUID, pUUID), nil)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
