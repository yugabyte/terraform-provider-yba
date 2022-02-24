package cloud_provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/cloud_providers"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/models"
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
	req := &models.Provider{
		AirGapInstall:        d.Get("air_gap_install").(bool),
		Code:                 d.Get("code").(string),
		Config:               utils.StringMap(d.Get("config").(map[string]interface{})),
		CustomHostCidrs:      utils.StringSlice(d.Get("custom_host_cidrs").([]interface{})),
		DestVpcID:            d.Get("dest_vpc_id").(string),
		HostVpcID:            d.Get("host_vpc_id").(string),
		HostVpcRegion:        d.Get("host_vpc_region").(string),
		HostedZoneID:         d.Get("hosted_zone_id").(string),
		HostedZoneName:       d.Get("hosted_zone_name").(string),
		KeyPairName:          d.Get("key_pair_name").(string),
		Name:                 d.Get("name").(string),
		SSHPort:              int32(d.Get("ssh_port").(int)),
		SSHPrivateKeyContent: d.Get("ssh_private_key_content").(string),
		SSHUser:              d.Get("ssh_user").(string),
		Regions:              buildRegions(d.Get("regions").([]interface{})),
	}
	p, err := c.PlatformAPIs.CloudProviders.CreateProviders(&cloud_providers.CreateProvidersParams{
		CreateProviderRequest: req,
		CUUID:                 c.CustomerUUID(),
		Context:               ctx,
		HTTPClient:            c.Session(),
	},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(string(p.Payload.ResourceUUID))
	tflog.Debug(ctx, fmt.Sprintf("Waiting for provider %s to be active", d.Id()))
	err = utils.WaitForTask(ctx, p.Payload.TaskUUID, c, time.Minute)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceCloudProviderRead(ctx, d, meta)
}

func findProvider(providers []*models.Provider, uuid strfmt.UUID) (*models.Provider, error) {
	for _, p := range providers {
		if p.UUID == uuid {
			return p, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("could not find provider %s", uuid))
}

func resourceCloudProviderRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	r, err := c.PlatformAPIs.CloudProviders.GetListOfProviders(&cloud_providers.GetListOfProvidersParams{
		CUUID:      c.CustomerUUID(),
		Context:    ctx,
		HTTPClient: c.Session(),
	},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	p, err := findProvider(r.Payload, strfmt.UUID(d.Id()))
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
	if err = d.Set("hosted_zone_id", p.HostedZoneID); err != nil {
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
	if err = d.Set("ssh_port", p.SSHPort); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_private_key_content", p.SSHPrivateKeyContent); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_user", p.SSHUser); err != nil {
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
	ywc := meta.(*api.ApiClient).YugawareClient
	pUUID := d.Id()
	_, err := vc.MakeRequest(http.MethodDelete, fmt.Sprintf("api/v1/customers/%s/providers/%s", ywc.CustomerUUID(), pUUID), nil)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
