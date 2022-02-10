package cloud_provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/models"
)

func RegionsSchema() *schema.Schema {
	r := regionsSchema()
	r.Required = true
	return r
}

func ComputedRegionsSchema() *schema.Schema {
	r := regionsSchema()
	r.Computed = true
	return r
}

func regionsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
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
	}
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
			Zones:           buildZones(region["zones"].([]interface{})),
		}
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
			Name:            utils.GetStringPointer(zone["name"].(string)),
			SecondarySubnet: zone["secondary_subnet"].(string),
			Subnet:          zone["subnet"].(string),
		}
		res = append(res, z)
	}
	return res
}

func flattenRegions(regions []*models.Region) (res []map[string]interface{}) {
	for _, region := range regions {
		r := map[string]interface{}{
			"uuid":              region.UUID,
			"code":              region.Code,
			"config":            region.Config,
			"latitude":          region.Latitude,
			"longitude":         region.Longitude,
			"name":              region.Name,
			"security_group_id": region.SecurityGroupID,
			"vnet_name":         region.VnetName,
			"yb_image":          region.YbImage,
			"zones":             flattenZones(region.Zones),
		}
		res = append(res, r)
	}
	return res
}

func flattenZones(zones []*models.AvailabilityZone) (res []map[string]interface{}) {
	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid":             zone.UUID,
			"active":           zone.Active,
			"code":             zone.Code,
			"config":           zone.Config,
			"kube_config_path": zone.KubeconfigPath,
			"secondary_subnet": zone.SecondarySubnet,
			"subnet":           zone.Subnet,
		}
		res = append(res, z)
	}
	return res
}
