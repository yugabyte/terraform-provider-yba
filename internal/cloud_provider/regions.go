package cloud_provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

func RegionsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Required: true,
		ForceNew: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					ForceNew:    true,
					Description: "Region UUID",
				},
				"code": {
					Type:        schema.TypeString,
					Computed:    true,
					Optional:    true,
					ForceNew:    true,
					Description: "Region code. Varies by cloud provider",
				},
				"config": {
					Type:        schema.TypeMap,
					Elem:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					ForceNew:    true,
					Description: "", // TODO: document
				},
				"latitude": {
					Type:        schema.TypeFloat,
					ForceNew:    true,
					Computed:    true,
					Optional:    true,
					Description: "", // TODO: document
				},
				"longitude": {
					Type:        schema.TypeFloat,
					ForceNew:    true,
					Optional:    true,
					Computed:    true,
					Description: "", // TODO: document
				},
				"name": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					ForceNew:    true,
					Description: "Name of the region. Varies by cloud provider",
				},
				"security_group_id": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					ForceNew:    true,
					Description: "Security group ID to use for this region. Only set for AWS/Azure providers",
				},
				"vnet_name": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					ForceNew:    true,
					Description: "Name of the virtual network/VPC ID to use for this region. Only set for AWS/Azure providers",
				},
				"yb_image": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					ForceNew:    true,
					Description: "", // TODO: document
				},
				"zones": {
					Type:     schema.TypeList,
					Optional: true,
					Computed: true,
					ForceNew: true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"uuid": {
								Type:        schema.TypeString,
								Computed:    true,
								ForceNew:    true,
								Description: "Zone UUID",
							},
							"active": {
								Type:        schema.TypeBool,
								Computed:    true,
								ForceNew:    true,
								Description: "Flag indicating if the zone is active",
							},
							"code": {
								Type:        schema.TypeString,
								Optional:    true,
								Computed:    true,
								ForceNew:    true,
								Description: "Code of the zone. Varies by cloud provider.",
							},
							"config": {
								Type:        schema.TypeMap,
								Elem:        schema.TypeString,
								Optional:    true,
								Computed:    true,
								ForceNew:    true,
								Description: "", // TODO: document
							},
							"kube_config_path": {
								Type:        schema.TypeString,
								Computed:    true,
								ForceNew:    true,
								Description: "", // TODO: document
							},
							"name": {
								Type:        schema.TypeString,
								Optional:    true,
								Computed:    true,
								ForceNew:    true,
								Description: "Name of the zone. Varies by cloud provider.",
							},
							"secondary_subnet": {
								Type:        schema.TypeString,
								Optional:    true,
								Computed:    true,
								ForceNew:    true,
								Description: "", // TODO: document
							},
							"subnet": {
								Type:        schema.TypeString,
								Optional:    true,
								Computed:    true,
								ForceNew:    true,
								Description: "Subnet to use for this zone",
							},
						},
					},
				},
			},
		},
	}
}

func buildRegions(regions []interface{}) (res []client.Region) {
	for _, v := range regions {
		region := v.(map[string]interface{})
		r := client.Region{
			Config:          utils.StringMap(region["config"].(map[string]interface{})),
			Code:            utils.GetStringPointer(region["code"].(string)),
			Name:            utils.GetStringPointer(region["name"].(string)),
			SecurityGroupId: utils.GetStringPointer(region["security_group_id"].(string)),
			VnetName:        utils.GetStringPointer(region["vnet_name"].(string)),
			YbImage:         utils.GetStringPointer(region["yb_image"].(string)),
			Zones:           buildZones(region["zones"].([]interface{})),
			Latitude:        utils.GetFloat64Pointer(region["latitude"].(float64)),
			Longitude:       utils.GetFloat64Pointer(region["longitude"].(float64)),
		}
		res = append(res, r)
	}
	return res
}

func buildZones(zones []interface{}) (res []client.AvailabilityZone) {
	for _, v := range zones {
		zone := v.(map[string]interface{})
		z := client.AvailabilityZone{
			Code:            utils.GetStringPointer(zone["code"].(string)),
			Config:          utils.StringMap(zone["config"].(map[string]interface{})),
			Name:            zone["name"].(string),
			SecondarySubnet: utils.GetStringPointer(zone["secondary_subnet"].(string)),
			Subnet:          utils.GetStringPointer(zone["subnet"].(string)),
		}
		res = append(res, z)
	}
	return res
}

func flattenRegions(regions []client.Region) (res []map[string]interface{}) {
	for _, region := range regions {
		r := map[string]interface{}{
			"uuid":      region.Uuid,
			"code":      region.Code,
			"config":    region.GetConfig(),
			"latitude":  region.Latitude,
			"longitude": region.Longitude,
			// TODO: the region name is being changed by the server, which messes with terraform state
			// stop-gap fix is to use the code value
			// https://yugabyte.atlassian.net/browse/PLAT-3034
			"name":              region.Code,
			"security_group_id": region.SecurityGroupId,
			"vnet_name":         region.VnetName,
			"yb_image":          region.YbImage,
			"zones":             flattenZones(region.Zones),
		}
		res = append(res, r)
	}
	return res
}

func flattenZones(zones []client.AvailabilityZone) (res []map[string]interface{}) {
	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid":             zone.Uuid,
			"active":           zone.Active,
			"config":           zone.GetConfig(),
			"kube_config_path": zone.KubeconfigPath,
			"secondary_subnet": zone.SecondarySubnet,
			"subnet":           zone.Subnet,
			// TODO: the region name/code is being changed by the server, which messes with terraform state
			// https://yugabyte.atlassian.net/browse/PLAT-3034
			"name": zone.Name,
			"code": zone.Code,
		}
		res = append(res, z)
	}
	return res
}
