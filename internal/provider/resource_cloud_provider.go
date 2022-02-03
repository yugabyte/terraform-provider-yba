package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	utils "github.com/yugabyte/terraform-provider-yugabyte-platform/internal"
	"log"
	"net/http"
	"strconv"
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
			"config": {
				Type:      schema.TypeMap,
				Elem:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
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
							Type:     schema.TypeString,
							Computed: true,
						},
						"longitude": {
							Type:     schema.TypeString,
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
										Type:     schema.TypeBool,
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
										Type:     schema.TypeBool,
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
				Type:      schema.TypeString,
				Optional:  true,
				ForceNew:  true,
				Sensitive: true,
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
	p := buildCloudProvider(d)
	body, err := json.Marshal(p)
	if err != nil {
		return diag.FromErr(err)
	}

	c := meta.(*ApiClient)
	cUUID := d.Get("customer_id").(string)
	r, err := c.MakeRequest(http.MethodPost, fmt.Sprintf("api/v1/customers/%s/providers", cUUID), bytes.NewBuffer(body))
	if err != nil {
		return diag.FromErr(err)
	}
	defer r.Body.Close()

	res := make(map[string]interface{})
	if err = json.NewDecoder(r.Body).Decode(&res); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(res["resourceUUID"].(string))

	err = waitForProviderToBeActive(ctx, cUUID, d.Id(), c)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceCloudProviderRead(ctx, d, meta)
}

func waitForProviderToBeActive(ctx context.Context, cUUID string, pUUID string, c *ApiClient) error {
	wait := &resource.StateChangeConf{
		Delay:   1 * time.Second,
		Pending: []string{"false"},
		Target:  []string{"true"},
		Timeout: 1 * time.Minute,

		Refresh: func() (result interface{}, state string, err error) {
			log.Printf("[DEBUG] Waiting for provider %d to be active", pUUID)
			r, err := c.MakeRequest(http.MethodGet, fmt.Sprintf("api/v1/customers/%s/providers", cUUID), nil)
			if err != nil {
				return nil, "", err
			}
			defer r.Body.Close()

			var providers []map[string]interface{}
			if err = json.NewDecoder(r.Body).Decode(&providers); err != nil {
				return nil, "", err
			}
			p, err := findProvider(providers, pUUID)
			if err != nil {
				return nil, "", err
			}
			s := strconv.FormatBool(p["active"].(bool))
			return s, s, nil
		},
	}

	if _, err := wait.WaitForStateContext(ctx); err != nil {
		return err
	}

	return nil
}

func findProvider(providers []map[string]interface{}, pUUID string) (map[string]interface{}, error) {
	for _, p := range providers {
		if p["uuid"] == pUUID {
			return p, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("could not find provider %s", pUUID))
}

func buildCloudProvider(d *schema.ResourceData) map[string]interface{} {
	p := make(map[string]interface{})

	utils.ResourceSetIfExists(p, d, "air_gap_install", "airGapInstall")
	utils.ResourceSetIfExists(p, d, "code", "code")
	utils.ResourceSetIfExists(p, d, "custom_host_cidrs", "customHostCidrs")
	utils.ResourceSetIfExists(p, d, "dest_vpc_id", "destVpcId")
	utils.ResourceSetIfExists(p, d, "host_vpc_id", "hostVpcId")
	utils.ResourceSetIfExists(p, d, "host_vpc_region", "hostVpcRegion")
	utils.ResourceSetIfExists(p, d, "hosted_zone_id", "hostedZoneId")
	utils.ResourceSetIfExists(p, d, "hosted_zone_name", "hostedZoneName")
	utils.ResourceSetIfExists(p, d, "key_pair_name", "keyPairName")
	utils.ResourceSetIfExists(p, d, "name", "name")
	utils.ResourceSetIfExists(p, d, "ssh_port", "sshPort")
	utils.ResourceSetIfExists(p, d, "ssh_private_key_content", "sshPrivateKeyContent")
	utils.ResourceSetIfExists(p, d, "ssh_user", "sshUser")

	if v, exists := d.GetOk("regions"); exists {
		p["regions"] = buildRegions(v.([]interface{}))
	}

	return p
}

func buildRegions(regions []interface{}) []map[string]interface{} {
	var res []map[string]interface{}

	for _, v := range regions {
		region := v.(map[string]interface{})
		r := make(map[string]interface{})

		utils.MapSetIfExists(r, region, "config", "config")
		utils.MapSetIfExists(r, region, "name", "name")
		utils.MapSetIfExists(r, region, "security_group_id", "securityGroupId")
		utils.MapSetIfExists(r, region, "vnet_name", "vnetName")
		utils.MapSetIfExists(r, region, "yb_image", "ybImage")
		utils.MapSetIfExists(r, region, "code", "code")

		if w, exists := region["zones"]; exists {
			r["zones"] = buildZones(w.([]interface{}))
		} else {
			r["zones"] = []interface{}{}
		}
	}

	return res
}

func buildZones(zones []interface{}) []map[string]interface{} {
	var res []map[string]interface{}

	for _, v := range zones {
		zone := v.(map[string]interface{})
		z := make(map[string]interface{})

		utils.MapSetIfExists(z, zone, "code", "code")
		utils.MapSetIfExists(z, zone, "config", "config")
		utils.MapSetIfExists(z, zone, "name", "name")
		utils.MapSetIfExists(z, zone, "secondary_subnet", "secondarySubnet")
		utils.MapSetIfExists(z, zone, "subnet", "subnet")

		res = append(res, z)
	}

	return res
}

func resourceCloudProviderRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*ApiClient)

	cUUID := d.Get("customer_id").(string)
	pUUID := d.Id()
	r, err := c.MakeRequest(http.MethodGet, fmt.Sprintf("api/v1/customers/%s/providers", cUUID), nil)
	if err != nil {
		return diag.FromErr(err)
	}
	defer r.Body.Close()

	var providers []map[string]interface{}
	if err = json.NewDecoder(r.Body).Decode(&providers); err != nil {
		return diag.FromErr(err)
	}
	p, err := findProvider(providers, pUUID)

	if err = utils.SetInResourceIfExists(d, p, "active", "active"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "airGapInstall", "air_gap_install"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "code", "code"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "config", "config"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "customHostCidrs", "custom_host_cidrs"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "destVpcId", "dest_vpc_id"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "hostVpcId", "host_vpc_id"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "hostVpcRegion", "host_vpc_region"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "hostedZoneId", "hosted_zone_id"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "hostedZoneName", "hosted_zone_name"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "keyPairName", "key_pair_name"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "name", "name"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "sshPort", "ssh_port"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "sshPrivateKeyContent", "ssh_private_key_content"); err != nil {
		return diag.FromErr(err)
	}
	if err = utils.SetInResourceIfExists(d, p, "sshUser", "ssh_user"); err != nil {
		return diag.FromErr(err)
	}

	regions, err := flattenRegions(p["regions"].([]interface{}))
	if err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("regions", regions); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func flattenRegions(regions []interface{}) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	for _, x := range regions {
		region := x.(map[string]interface{})
		r := make(map[string]interface{})

		utils.MapSetIfExists(r, region, "uuid", "uuid")
		utils.MapSetIfExists(r, region, "code", "code")
		utils.MapSetIfExists(r, region, "config", "config")
		utils.MapSetIfExists(r, region, "latitude", "latitude")
		utils.MapSetIfExists(r, region, "longitude", "longitude")
		utils.MapSetIfExists(r, region, "name", "name")
		utils.MapSetIfExists(r, region, "SecurityGroupId", "security_group_id")
		utils.MapSetIfExists(r, region, "vnetName", "vnet_name")
		utils.MapSetIfExists(r, region, "ybImage", "yb_image")

		r["zones"] = flattenZones(region["zones"].([]interface{}))
		result = append(result, r)
	}
	return result, nil
}

func flattenZones(zones []interface{}) []map[string]interface{} {
	var result []map[string]interface{}
	for _, x := range zones {
		zone := x.(map[string]interface{})
		z := make(map[string]interface{})

		utils.MapSetIfExists(z, zone, "uuid", "uuid")
		utils.MapSetIfExists(z, zone, "active", "active")
		utils.MapSetIfExists(z, zone, "code", "code")
		utils.MapSetIfExists(z, zone, "config", "config")
		utils.MapSetIfExists(z, zone, "kubeConfigPath", "kube_config_path")
		utils.MapSetIfExists(z, zone, "secondarySubnet", "secondary_subnet")
		utils.MapSetIfExists(z, zone, "subnet", "subnet")

		result = append(result, z)
	}
	return result
}

func resourceCloudProviderUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// update provider API in platform has very limited functionality
	// most likely that the provider will be recreated
	m := make(map[string]interface{})
	utils.ResourceSetIfExists(m, d, "hostedZoneId", "hosted_zone_id")
	utils.ResourceSetIfExists(m, d, "config", "config")
	body, err := json.Marshal(m)
	if err != nil {
		return diag.FromErr(err)
	}

	c := meta.(*ApiClient)
	cUUID := d.Get("customer_id").(string)
	pUUID := d.Id()
	r, err := c.MakeRequest(http.MethodPut, fmt.Sprintf("api/v1/customers/%s/providers/%s/edit", cUUID, pUUID), bytes.NewBuffer(body))
	if err != nil {
		return diag.FromErr(err)
	}
	defer r.Body.Close()

	return resourceCloudProviderRead(ctx, d, meta)
}

func resourceCloudProviderDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*ApiClient)
	cUUID := d.Get("customer_id").(string)
	pUUID := d.Id()
	r, err := c.MakeRequest(http.MethodDelete, fmt.Sprintf("api/v1/customers/%s/providers/%s", cUUID, pUUID), nil)
	if err != nil {
		return diag.FromErr(err)
	}
	defer r.Body.Close()

	d.SetId("")
	return diags
}
