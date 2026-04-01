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

package aws

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
)

// regionData holds user-provided region fields for comparison
type regionData struct {
	vpcID           string
	securityGroupID string
	zones           map[string]zoneData
}

// zoneData holds user-provided zone fields for comparison
type zoneData struct {
	subnet          string
	secondarySubnet string
}

// validateNoDuplicateRegionsOrZones is the CustomizeDiff function for AWS providers.
func validateNoDuplicateRegionsOrZones(
	ctx context.Context,
	d *schema.ResourceDiff,
	meta interface{},
) error {
	if err := providerutil.MarkVersionComputedIfChanged(ctx, d,
		[]string{
			"access_key_id", "secret_access_key", "use_iam_instance_profile",
			"hosted_zone_id", "skip_ssh_keypair_validation",
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
		regionCode := regionMap["code"].(string)

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
			zoneCode := zoneMap["code"].(string)

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

	if ybaBundles, ok := d.Get("yba_managed_image_bundles").([]interface{}); ok {
		seenArch := make(map[string]bool)
		for _, b := range ybaBundles {
			bundleMap, ok := b.(map[string]interface{})
			if !ok {
				continue
			}
			arch, _ := bundleMap["arch"].(string)
			if arch == "" {
				continue
			}
			if seenArch[arch] {
				return fmt.Errorf(
					"duplicate architecture %q in yba_managed_image_bundles: "+
						"each architecture must appear at most once",
					arch,
				)
			}
			seenArch[arch] = true
		}
	}

	if err := providerutil.ValidateImageBundles(d); err != nil {
		return err
	}

	if err := validateAWSImageBundleRegionCoverage(d); err != nil {
		return err
	}

	return nil
}

// validateAWSImageBundleRegionCoverage ensures that every custom image bundle provides
// a non-empty AMI in region_overrides for every region configured in the provider.
//
// This enforces correctness for all mutating operations:
//   - Provider creation: all configured regions must be covered from the start.
//   - Adding a region: all existing bundles must be updated with the new region's AMI.
//   - Adding a new bundle: the bundle must cover all currently configured regions.
//   - Modifying a bundle: region_overrides must still cover all configured regions.
//
// Empty string "" is not a valid AMI and is rejected alongside missing keys.
func validateAWSImageBundleRegionCoverage(d *schema.ResourceDiff) error {
	bundlesRaw, _ := d.Get("image_bundles").([]interface{})
	if len(bundlesRaw) == 0 {
		return nil
	}

	regionsRaw, _ := d.Get("regions").([]interface{})
	regionCodes := collectRegionCodes(regionsRaw)
	if len(regionCodes) == 0 {
		return nil
	}

	return checkImageBundleRegionCoverage(bundlesRaw, regionCodes)
}

// collectRegionCodes extracts non-empty region codes from the raw regions list.
func collectRegionCodes(regionsRaw []interface{}) []string {
	codes := make([]string, 0, len(regionsRaw))
	for _, r := range regionsRaw {
		regionMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := regionMap["code"].(string)
		if code != "" {
			codes = append(codes, code)
		}
	}
	return codes
}

// checkImageBundleRegionCoverage is the pure validation logic, extracted so it can
// be exercised directly in unit tests without a live schema.ResourceDiff.
func checkImageBundleRegionCoverage(
	bundlesRaw []interface{},
	regionCodes []string,
) error {
	for _, b := range bundlesRaw {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := bundleMap["name"].(string)

		details, ok := bundleMap["details"].([]interface{})
		if !ok || len(details) == 0 {
			continue
		}
		det, ok := details[0].(map[string]interface{})
		if !ok {
			continue
		}

		overrides, _ := det["region_overrides"].(map[string]interface{})

		for _, regionCode := range regionCodes {
			ami, exists := overrides[regionCode]
			amiStr, _ := ami.(string)
			if !exists || amiStr == "" {
				return fmt.Errorf(
					"image bundle %q must specify a non-empty AMI for region %q in "+
						"region_overrides: all custom image bundles must provide a valid "+
						"AMI for every region configured in the provider "+
						"(empty string \"\" is not accepted)",
					name, regionCode,
				)
			}
		}
	}
	return nil
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
		if oldRegion.vpcID != newRegion.vpcID ||
			oldRegion.securityGroupID != newRegion.securityGroupID {
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

// extractRegionData extracts user-provided fields from regions for comparison.
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
			continue
		}

		rd := regionData{
			vpcID:           providerutil.GetString(regionMap, "vpc_id"),
			securityGroupID: providerutil.GetString(regionMap, "security_group_id"),
			zones:           make(map[string]zoneData),
		}

		zones, _ := regionMap["zones"].([]interface{})

		for _, z := range zones {
			zoneMap, ok := z.(map[string]interface{})
			if !ok {
				continue
			}
			zoneCode, _ := zoneMap["code"].(string)
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

// suppressIfAWSRegionsPureReorder suppresses positional diffs when regions are only reordered.
func suppressIfAWSRegionsPureReorder(k, old, new string, d *schema.ResourceData) bool {
	if old == new {
		return true
	}
	o, n := d.GetChange("regions")
	return !regionsContentChanged(o, n)
}
