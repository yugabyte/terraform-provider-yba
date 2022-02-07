package provider

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	utils "github.com/yugabyte/terraform-provider-yugabyte-platform/internal"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/cloud_providers"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/customer_tasks"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/models"
	"net/http"
	"time"
)

func resourceCloudProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Cloud Provider Resource",

		CreateContext: resourceCloudProviderCreate,
		ReadContext:   resourceCloudProviderRead,
		UpdateContext: resourceCloudProviderUpdate,
		DeleteContext: resourceCloudProviderDelete,

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
			"config": {
				Type:     schema.TypeMap,
				Elem:     schema.TypeString,
				Optional: true,
			},
			"custom_host_cidrs": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
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
			"regions": {
				Type:     schema.TypeList,
				Required: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"code": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"config": {
							Type:     schema.TypeMap,
							Elem:     schema.TypeString,
							Optional: true,
						},
						"latitude": {
							Type:     schema.TypeFloat,
							Computed: true,
						},
						"longitude": {
							Type:     schema.TypeFloat,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"security_group_id": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"vnet_name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"yb_image": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"zones": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"active": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"code": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"config": {
										Type:     schema.TypeMap,
										Elem:     schema.TypeString,
										Optional: true,
									},
									"kube_config_path": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"name": {
										Type:     schema.TypeString,
										Required: true,
									},
									"secondary_subnet": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"subnet": {
										Type:     schema.TypeString,
										Optional: true,
									},
								},
							},
						},
					},
				},
			},
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
	c := meta.(*ApiClient).YugawareClient
	req := buildCloudProvider(d)
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
	err = waitForProviderToBeActive(ctx, p.Payload.TaskUUID, c)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceCloudProviderRead(ctx, d, meta)
}

func waitForProviderToBeActive(ctx context.Context, tUUID strfmt.UUID, c *client.YugawareClient) error {
	wait := &resource.StateChangeConf{
		Delay:   1 * time.Second,
		Pending: utils.PendingTaskStates,
		Target:  utils.SuccessTaskStates,
		Timeout: 1 * time.Minute,

		Refresh: func() (result interface{}, state string, err error) {
			r, err := c.PlatformAPIs.CustomerTasks.TaskStatus(&customer_tasks.TaskStatusParams{
				CUUID:      c.CustomerUUID(),
				TUUID:      tUUID,
				Context:    ctx,
				HTTPClient: c.Session(),
			},
				c.SwaggerAuth,
			)
			if err != nil {
				return nil, "", err
			}

			s := r.Payload["status"].(string)
			return s, s, nil
		},
	}

	if _, err := wait.WaitForStateContext(ctx); err != nil {
		return err
	}

	return nil
}

func buildCloudProvider(d *schema.ResourceData) *models.Provider {
	p := models.Provider{
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
	}

	p.Regions = buildRegions(d.Get("regions").([]interface{}))
	return &p
}

func buildRegions(regions []interface{}) (res []*models.Region) {
	for _, v := range regions {
		region := v.(map[string]interface{})
		r := &models.Region{
			Config:          utils.StringMap(region["config"].(map[string]interface{})),
			Name:            region["name"].(string),
			SecurityGroupID: region["security_group_id"].(string),
			VnetName:        region["vnet_name"].(string),
			YbImage:         region["yb_image"].(string),
			Code:            region["code"].(string),
		}
		r.Zones = buildZones(region["zones"].([]interface{}))
		res = append(res, r)
	}
	return res
}

func buildZones(zones []interface{}) (res []*models.AvailabilityZone) {
	for _, v := range zones {
		zone := v.(map[string]interface{})
		z := &models.AvailabilityZone{
			Code:            zone["code"].(string),
			Config:          utils.StringMap(zone["config"].(map[string]interface{})),
			Name:            zone["name"].(*string),
			SecondarySubnet: zone["secondary_subnet"].(string),
			Subnet:          zone["subnet"].(string),
		}
		res = append(res, z)
	}
	return res
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

	c := meta.(*ApiClient).YugawareClient
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
	if err = d.Set("config", p.Config); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("custom_host_cidrs", p.CustomHostCidrs); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("dest_vpc_id", p.DestVpcID); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("host_vpc_id", p.HostVpcID); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("host_vpc_region", p.HostVpcRegion); err != nil {
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
	if err = d.Set("key_pair_name", p.KeyPairName); err != nil {
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

func flattenRegions(regions []*models.Region) (res []map[string]interface{}) {
	for _, region := range regions {

		r := make(map[string]interface{})
		r["uuid"] = region.UUID
		r["code"] = region.Code
		r["config"] = region.Config
		r["latitude"] = region.Latitude
		r["longitude"] = region.Longitude
		r["name"] = region.Name
		r["security_group_id"] = region.SecurityGroupID
		r["vnet_name"] = region.VnetName
		r["yb_image"] = region.YbImage
		r["zones"] = flattenZones(region.Zones)
		res = append(res, r)
	}
	return res
}

func flattenZones(zones []*models.AvailabilityZone) (res []map[string]interface{}) {
	for _, zone := range zones {
		z := make(map[string]interface{})
		z["uuid"] = zone.UUID
		z["active"] = zone.Active
		z["code"] = zone.Code
		z["config"] = zone.Config
		z["kube_config_path"] = zone.KubeconfigPath
		z["secondary_subnet"] = zone.SecondarySubnet
		z["subnet"] = zone.Subnet
		res = append(res, z)
	}
	return res
}

func resourceCloudProviderUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// update provider API in platform has very limited functionality
	// most likely that the provider will be recreated

	c := meta.(*ApiClient).YugawareClient
	_, err := c.PlatformAPIs.CloudProviders.EditProvider(&cloud_providers.EditProviderParams{
		EditProviderFormData: &models.EditProviderRequest{
			Config:       d.Get("config").(map[string]string),
			HostedZoneID: d.Get("hosted_zone_id").(*string),
		},
		CUUID:      c.CustomerUUID(),
		PUUID:      strfmt.UUID(d.Id()),
		Context:    ctx,
		HTTPClient: c.Session(),
	},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceCloudProviderRead(ctx, d, meta)
}

func resourceCloudProviderDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// TODO: this uses a non-public API
	var diags diag.Diagnostics

	vc := meta.(*ApiClient).VanillaClient
	ywc := meta.(*ApiClient).YugawareClient
	pUUID := d.Id()
	_, err := vc.MakeRequest(http.MethodDelete, fmt.Sprintf("api/v1/customers/%s/providers/%s", ywc.CustomerUUID(), pUUID), nil)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
