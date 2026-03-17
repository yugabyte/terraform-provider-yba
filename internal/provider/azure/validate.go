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

package azure

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
)

// validateAzureProvider is the CustomizeDiff function for Azure providers.
func validateAzureProvider(
	ctx context.Context,
	d *schema.ResourceDiff,
	meta interface{},
) error {
	if err := providerutil.MarkVersionComputedIfChanged(ctx, d,
		[]string{
			"client_id", "client_secret", "subscription_id", "tenant_id",
			"resource_group", "hosted_zone_id", "network_subscription_id",
			"network_resource_group",
		},
		regionsContentChanged,
	); err != nil {
		return err
	}

	if err := providerutil.ValidateAtLeastOneImageBundle(d); err != nil {
		return err
	}

	if err := providerutil.ValidateNewBundlesNotDefault(d); err != nil {
		return err
	}

	regionsRaw, _ := d.Get("regions").([]interface{})

	regionCodes := make(map[string]bool)

	for _, r := range regionsRaw {
		regionMap := r.(map[string]interface{})
		regionCode := providerutil.GetString(regionMap, "code")
		if regionCode == "" {
			regionCode = providerutil.GetString(regionMap, "name")
		}
		if regionCode == "" {
			continue
		}

		if regionCodes[regionCode] {
			return fmt.Errorf(
				"duplicate region code %q found: each region must have a unique code",
				regionCode,
			)
		}
		regionCodes[regionCode] = true

		zonesList, _ := regionMap["zones"].([]interface{})
		zoneCodes := make(map[string]bool)
		for _, z := range zonesList {
			zoneMap := z.(map[string]interface{})
			zoneCode := providerutil.GetString(zoneMap, "code")
			if zoneCode == "" {
				zoneCode = providerutil.GetString(zoneMap, "name")
			}
			if zoneCode == "" {
				continue
			}
			if zoneCodes[zoneCode] {
				return fmt.Errorf(
					"duplicate zone code %q found in region %q: "+
						"each zone within a region must have a unique code",
					zoneCode, regionCode,
				)
			}
			zoneCodes[zoneCode] = true
		}
	}

	if err := providerutil.ValidateImageBundles(d); err != nil {
		return err
	}

	return nil
}

// regionData holds user-provided region fields for comparison
type regionData struct {
	vnet                 string
	securityGroupID      string
	resourceGroup        string
	networkResourceGroup string
	zones                map[string]zoneData
}

// zoneData holds user-provided zone fields for comparison
type zoneData struct {
	subnet          string
	secondarySubnet string
}

// regionsContentChanged reports whether two region lists differ in content (ignoring order).
func regionsContentChanged(oldRaw, newRaw interface{}) bool {
	oldRegions := extractRegionData(oldRaw)
	newRegions := extractRegionData(newRaw)

	if len(oldRegions) != len(newRegions) {
		return true
	}

	for regionCode, oldRegion := range oldRegions {
		newRegion, exists := newRegions[regionCode]
		if !exists {
			return true
		}
		if oldRegion.vnet != newRegion.vnet ||
			oldRegion.securityGroupID != newRegion.securityGroupID ||
			oldRegion.resourceGroup != newRegion.resourceGroup ||
			oldRegion.networkResourceGroup != newRegion.networkResourceGroup {
			return true
		}
		if len(oldRegion.zones) != len(newRegion.zones) {
			return true
		}
		for zoneCode, oldZone := range oldRegion.zones {
			newZone, exists := newRegion.zones[zoneCode]
			if !exists {
				return true
			}
			if oldZone.subnet != newZone.subnet ||
				oldZone.secondarySubnet != newZone.secondarySubnet {
				return true
			}
		}
	}

	return false
}

// extractRegionData extracts user-provided fields from Azure regions for comparison.
func extractRegionData(regionsRaw interface{}) map[string]regionData {
	result := make(map[string]regionData)

	regions, _ := regionsRaw.([]interface{})
	for _, r := range regions {
		regionMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		regionCode, _ := regionMap["code"].(string)
		if regionCode == "" {
			regionCode, _ = regionMap["name"].(string)
		}
		if regionCode == "" {
			continue
		}

		rd := regionData{
			vnet:                 providerutil.GetString(regionMap, "vnet"),
			securityGroupID:      providerutil.GetString(regionMap, "security_group_id"),
			resourceGroup:        providerutil.GetString(regionMap, "resource_group"),
			networkResourceGroup: providerutil.GetString(regionMap, "network_resource_group"),
			zones:                make(map[string]zoneData),
		}

		zones, _ := regionMap["zones"].([]interface{})
		for _, z := range zones {
			zoneMap, ok := z.(map[string]interface{})
			if !ok {
				continue
			}
			zoneCode, _ := zoneMap["code"].(string)
			if zoneCode == "" {
				zoneCode, _ = zoneMap["name"].(string)
			}
			if zoneCode == "" {
				continue
			}
			rd.zones[zoneCode] = zoneData{
				subnet:          providerutil.GetString(zoneMap, "subnet"),
				secondarySubnet: providerutil.GetString(zoneMap, "secondary_subnet"),
			}
		}

		result[regionCode] = rd
	}

	return result
}

// suppressIfAzureRegionsPureReorder suppresses positional diffs when regions are only reordered.
func suppressIfAzureRegionsPureReorder(k, old, new string, d *schema.ResourceData) bool {
	if old == new {
		return true
	}
	o, n := d.GetChange("regions")
	return !regionsContentChanged(o, n)
}
