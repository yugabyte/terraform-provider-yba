// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package onprem

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// RegionsSchema manages Region level information of cloud providers
func RegionsSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Description of regions associated with the onprem provider.",
		Type:        schema.TypeList,
		Required:    true,
		MinItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Region UUID.",
				},
				"code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Region code.",
				},
				"latitude": {
					Type:             schema.TypeFloat,
					Optional:         true,
					Default:          0.0,
					DiffSuppressFunc: suppressRegionLocationDiff,
					Description:      "Latitude of the region. 0 by default.",
				},
				"longitude": {
					Type:             schema.TypeFloat,
					Optional:         true,
					Default:          0.0,
					DiffSuppressFunc: suppressRegionLocationDiff,
					Description:      "Longitude of the region. 0 by default.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Name of the region. Same as the code.",
				},
				"config": {
					Type:        schema.TypeMap,
					Elem:        &schema.Schema{Type: schema.TypeString},
					Computed:    true,
					Description: "Region related configuration.",
				},
				"zones": {
					Type:        schema.TypeList,
					Description: "Description of zones associated with the region.",
					Required:    true,
					MinItems:    1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"uuid": {
								Type:        schema.TypeString,
								Computed:    true,
								Description: "Zone UUID",
							},
							"active": {
								Type:        schema.TypeBool,
								Computed:    true,
								Description: "Flag indicating if the zone is active",
							},
							"code": {
								Type:        schema.TypeString,
								Computed:    true,
								Description: "Code of the zone.",
							},
							"name": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "Name of the zone. Varies by cloud provider.",
							},
						},
					},
				},
			},
		},
	}
}

func suppressRegionLocationDiff(k, old, new string, d *schema.ResourceData) bool {
	// API returns -90 for default, but default is set to 0
	// If old is -90 or 0, ignore difference
	oldFloat, err := strconv.ParseFloat(old, 64)
	if err != nil {
		return false
	}
	newFloat, err := strconv.ParseFloat(new, 64)
	if err != nil {
		return false
	}
	if (oldFloat == 0 || oldFloat == -90) && (newFloat == 0 || newFloat == -90) {
		return true
	}
	return false
}

func buildRegions(regions []interface{}) (res []client.Region) {
	for _, v := range regions {
		region := v.(map[string]interface{})
		r := client.Region{
			Code:      utils.GetStringPointer(region["name"].(string)),
			Name:      utils.GetStringPointer(region["name"].(string)),
			Zones:     buildZones(region["zones"].([]interface{})),
			Latitude:  utils.GetFloat64Pointer(region["latitude"].(float64)),
			Longitude: utils.GetFloat64Pointer(region["longitude"].(float64)),
		}
		res = append(res, r)
	}
	return res
}

func buildZones(zones []interface{}) (res []client.AvailabilityZone) {
	for _, v := range zones {
		zone := v.(map[string]interface{})
		z := client.AvailabilityZone{
			Code: utils.GetStringPointer(zone["name"].(string)),
			Name: zone["name"].(string),
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
			"name":      region.Code,
			"zones":     flattenZones(region.Zones),
		}
		res = append(res, r)
	}
	return res
}

func flattenZones(zones []client.AvailabilityZone) (res []map[string]interface{}) {
	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid":   zone.Uuid,
			"active": zone.Active,
			"name":   zone.Name,
			"code":   zone.Code,
		}
		res = append(res, z)
	}
	return res
}

func fetchRegionUUIDFromRegionName(ctx context.Context, c *client.APIClient,
	cUUID, pUUID, rName string) (string, error) {
	var err error
	r, response, err := c.RegionManagementApi.GetRegion(ctx, cUUID, pUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Onprem Node Instances Fetch Region UUID", "Create")
		return "", errMessage
	}
	for _, region := range r {
		if region.GetName() == rName {
			return region.GetUuid(), nil
		}
	}
	return "", fmt.Errorf("No region %s found in provider %s", rName, pUUID)
}

func fetchZoneUUIDFromZoneName(ctx context.Context, c *client.APIClient,
	cUUID, pUUID, rUUID string, azName string) (string, error) {
	r, response, err := c.AvailabilityZonesApi.ListOfAZ(ctx, cUUID, pUUID, rUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Onprem Node Instances Fetch Availability Zone UUID", "Create")
		return "", errMessage
	}
	for _, az := range r {
		if az.GetName() == azName {
			return az.GetUuid(), nil
		}
	}
	return "", fmt.Errorf("No availability zone %s found in region "+
		" %s of provider %s", azName, rUUID, pUUID)
}

func createRequestForEditRegions(old, new []client.Region) (req []client.Region) {
	newRegionNamesList := make([]string, 0)
	for _, n := range new {
		newRegionNamesList = append(newRegionNamesList, n.GetName())
	}
	// length old == new --> old stays the same, completely run over, new = 0
	// length old < new --> old stays the same, new will have a few elements left --> add to req
	// length old > new --> new will run out, old will remain --> delete from provider, set to false

	for _, o := range old {
		index := slices.Index(newRegionNamesList, o.GetName())
		if index == -1 {
			o.SetActive(false)
			req = append(req, o)
		} else {
			n := new[index]
			r := buildRegionForEditProvider(o, n)
			req = append(req, r)
			new = append(new[:index], new[index+1:]...)
			newRegionNamesList = append(newRegionNamesList[:index], newRegionNamesList[index+1:]...)
		}
	}
	if len(new) > 0 {
		for _, n := range new {
			req = append(req, n)
		}
	}
	return req
}

func buildRegionForEditProvider(old, new client.Region) client.Region {
	new.SetUuid(old.GetUuid())
	new.SetActive(old.GetActive())
	new.SetCode(new.GetName())
	oldZones := old.GetZones()
	newZones := new.GetZones()
	newAZNamesList := make([]string, 0)
	for _, n := range newZones {
		newAZNamesList = append(newAZNamesList, n.GetName())
	}
	reqZones := make([]client.AvailabilityZone, 0)
	for _, o := range oldZones {
		index := slices.Index(newAZNamesList, o.GetName())
		if index == -1 {
			o.SetActive(false)
			reqZones = append(reqZones, o)
		} else {
			n := newZones[index]
			r := buildAZForEditProvider(o, n)
			reqZones = append(reqZones, r)
			newZones = append(newZones[:index], newZones[index+1:]...)
			newAZNamesList = append(newAZNamesList[:index], newAZNamesList[index+1:]...)
		}
	}
	if len(newZones) > 0 {
		for _, n := range newZones {
			reqZones = append(reqZones, n)
		}
	}

	new.SetZones(reqZones)
	return new
}

func buildAZForEditProvider(oz, nz client.AvailabilityZone) client.AvailabilityZone {
	nz.SetCode(nz.GetCode())
	nz.SetUuid(oz.GetUuid())
	nz.SetActive(oz.GetActive())
	return nz
}
