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
// It handles:
// 1. Marking version as computed when real changes occur
// 2. Preserving YBA-managed bundles in plan output
// 3. Validating no duplicate region/zone codes
func validateNoDuplicateRegionsOrZones(
	ctx context.Context,
	d *schema.ResourceDiff,
	meta interface{},
) error {
	// Mark version as computed when there are real changes that will trigger an update.
	if d.Id() != "" {
		hasRealChange := false

		// Check simple scalar fields - no computed sub-fields
		simpleFields := []string{
			"name", "air_gap_install", "ntp_servers", "set_up_chrony",
			"access_key_id", "secret_access_key", "use_iam_instance_profile",
			"hosted_zone_id", "ssh_keypair_name", "ssh_private_key_content",
			"skip_ssh_keypair_validation",
			"skip_keypair_validation",
		}
		for _, field := range simpleFields {
			if d.HasChange(field) {
				hasRealChange = true
				break
			}
		}

		// Check regions - only compare user-provided code values
		// (HasChange sees computed sub-fields like uuid, name, vpc_id)
		// NOTE: TypeList shows positional diffs if user reorders regions/zones in config.
		// DiffSuppressFunc handles order-only diffs, but real changes are detected here.
		if !hasRealChange && d.HasChange("regions") && hasRegionCodesChanged(d) {
			hasRealChange = true
		}

		// Special handling for image_bundles: filter out YBA-managed bundles
		// Also check hasImageBundleRealChange unconditionally because d.HasChange
		// may return false when user removes the entire image_bundles block
		// (Optional+Computed fields with missing config are treated as "computed")
		if !hasRealChange && hasImageBundleRealChange(d) {
			hasRealChange = true
		}

		if hasRealChange {
			if err := d.SetNewComputed("version"); err != nil {
				return err
			}
		}

		// Preserve YBA-managed bundles in plan output
		// This ensures the plan shows both user bundles AND YBA-managed bundles,
		// not a misleading "remove YBA bundle, add user bundle" diff
		if err := preserveYBAManagedBundlesInPlan(ctx, d); err != nil {
			return err
		}
	}

	// TypeList returns []interface{} directly
	regionsRaw, _ := d.Get("regions").([]interface{})

	// Track region codes to detect duplicates
	regionCodes := make(map[string]bool)

	for _, r := range regionsRaw {
		regionMap := r.(map[string]interface{})
		regionCode := regionMap["code"].(string)

		// Check for duplicate region code
		if regionCodes[regionCode] {
			return fmt.Errorf(
				"duplicate region code %q found: each region must have a unique code",
				regionCode,
			)
		}
		regionCodes[regionCode] = true

		// Check for duplicate zone codes within this region (TypeList)
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

	return nil
}

// hasRegionCodesChanged checks if user-provided region/zone fields changed.
// Compares: code, vpc_id, security_group_id for regions
// Compares: code, subnet, secondary_subnet for zones
// Ignores computed fields: uuid, name
func hasRegionCodesChanged(d *schema.ResourceDiff) bool {
	oldRaw, newRaw := d.GetChange("regions")
	return regionsContentChanged(oldRaw, newRaw)
}

// regionsContentChanged compares two region lists by content (ignoring order).
// Returns true if there are real changes (different codes, vpc_id, zones, etc.)
// Returns false if the only difference is ordering.
func regionsContentChanged(oldRaw, newRaw interface{}) bool {
	oldRegions := extractRegionData(oldRaw)
	newRegions := extractRegionData(newRaw)

	// Compare region count
	if len(oldRegions) != len(newRegions) {
		return true
	}

	for regionCode, oldRegion := range oldRegions {
		newRegion, exists := newRegions[regionCode]
		if !exists {
			return true
		}
		// Compare region-level user fields
		if oldRegion.vpcID != newRegion.vpcID ||
			oldRegion.securityGroupID != newRegion.securityGroupID {
			return true
		}
		// Compare zone count
		if len(oldRegion.zones) != len(newRegion.zones) {
			return true
		}
		// Compare zone-level user fields
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

// extractRegionData extracts user-provided fields from regions for comparison
// Works with TypeList ([]interface{})
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
			vpcID:           getString(regionMap, "vpc_id"),
			securityGroupID: getString(regionMap, "security_group_id"),
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
				subnet:          getString(zoneMap, "subnet"),
				secondarySubnet: getString(zoneMap, "secondary_subnet"),
			}
		}

		result[regionCode] = rd
	}

	return result
}

// getString safely extracts string from map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// hasImageBundleRealChange checks if user actually changed image_bundles.
// Uses metadata_type from state to identify YBA-managed bundles (no API call).
func hasImageBundleRealChange(d *schema.ResourceDiff) bool {
	oldRaw, newRaw := d.GetChange("image_bundles")

	// Build set of YBA-managed names from state
	ybaManagedNames := collectYBAManagedNames(oldRaw)

	// Extract user bundles as maps (name -> bundle details)
	oldUserBundles := extractUserBundlesMap(oldRaw, ybaManagedNames)
	newUserBundles := extractUserBundlesMap(newRaw, ybaManagedNames)

	// If user bundle count changed, it's a real change
	if len(oldUserBundles) != len(newUserBundles) {
		return true
	}

	// Check if all old user bundle names exist in new
	for name := range oldUserBundles {
		if _, exists := newUserBundles[name]; !exists {
			return true
		}
	}

	// Check if any new user bundle names were added
	for name := range newUserBundles {
		if _, exists := oldUserBundles[name]; !exists {
			return true
		}
	}

	// Compare content of existing bundles (set-like comparison ignoring order)
	for name, oldBundle := range oldUserBundles {
		newBundle := newUserBundles[name]
		if bundleContentChanged(oldBundle, newBundle) {
			return true
		}
	}

	return false
}

// bundleContentChanged compares editable fields between two bundles
func bundleContentChanged(old, new map[string]interface{}) bool {
	// Compare use_as_default
	oldDefault, _ := old["use_as_default"].(bool)
	newDefault, _ := new["use_as_default"].(bool)
	if oldDefault != newDefault {
		return true
	}

	// Compare details
	oldDetails := getBundleDetails(old)
	newDetails := getBundleDetails(new)

	if oldDetails["arch"] != newDetails["arch"] ||
		oldDetails["ssh_user"] != newDetails["ssh_user"] ||
		oldDetails["ssh_port"] != newDetails["ssh_port"] ||
		oldDetails["global_yb_image"] != newDetails["global_yb_image"] {
		return true
	}

	// Compare region_overrides
	oldOverrides, _ := oldDetails["region_overrides"].(map[string]interface{})
	newOverrides, _ := newDetails["region_overrides"].(map[string]interface{})
	if len(oldOverrides) != len(newOverrides) {
		return true
	}
	for k, v := range oldOverrides {
		if newOverrides[k] != v {
			return true
		}
	}

	return false
}

// getBundleDetails extracts details map from a bundle
func getBundleDetails(bundle map[string]interface{}) map[string]interface{} {
	if details, ok := bundle["details"].([]interface{}); ok && len(details) > 0 {
		if det, ok := details[0].(map[string]interface{}); ok {
			return det
		}
	}
	return make(map[string]interface{})
}

// collectYBAManagedNames finds all YBA-managed bundle names from state
func collectYBAManagedNames(bundlesRaw interface{}) map[string]bool {
	ybaManagedNames := make(map[string]bool)

	bundles, _ := bundlesRaw.([]interface{})

	for _, b := range bundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType == ImageBundleTypeYBAActive {
			if name, _ := bundleMap["name"].(string); name != "" {
				ybaManagedNames[name] = true
			}
		}
	}

	return ybaManagedNames
}

// extractUserBundlesMap extracts user bundles as a map (name -> bundle)
func extractUserBundlesMap(
	bundlesRaw interface{},
	ybaManagedNames map[string]bool,
) map[string]map[string]interface{} {
	userBundles := make(map[string]map[string]interface{})

	bundles, _ := bundlesRaw.([]interface{})

	for _, b := range bundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := bundleMap["name"].(string)
		if name == "" {
			continue
		}

		// Skip YBA-managed bundles
		if ybaManagedNames[name] {
			continue
		}

		userBundles[name] = bundleMap
	}

	return userBundles
}

// preserveYBAManagedBundlesInPlan ensures YBA-managed bundles are preserved in the plan.
// It also handles the case where user removes their custom bundles.
// Without this, the plan may not show bundle changes correctly due to Optional+Computed behavior.
func preserveYBAManagedBundlesInPlan(ctx context.Context, d *schema.ResourceDiff) error {
	oldRaw, newRaw := d.GetChange("image_bundles")

	// TypeList returns []interface{} directly
	oldBundles, _ := oldRaw.([]interface{})
	newBundles, _ := newRaw.([]interface{})

	// Find YBA-managed bundles and user bundles in old state
	var ybaManagedBundles []interface{}
	var oldUserBundleCount int
	for _, b := range oldBundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType == ImageBundleTypeYBAActive {
			ybaManagedBundles = append(ybaManagedBundles, bundleMap)
		} else if metadataType == "CUSTOM" || metadataType == "" {
			oldUserBundleCount++
		}
	}

	// Count user bundles in new value
	newUserBundleCount := 0
	for _, b := range newBundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		// New bundles from config won't have metadata_type yet
		if metadataType != ImageBundleTypeYBAActive {
			newUserBundleCount++
		}
	}

	// Detect if user removed bundles from config:
	// When d.HasChange() is true but old and new have the same CUSTOM bundles,
	// it means Terraform populated "new" from state (because it's Computed),
	// but the user's actual config has no bundles.
	userBundlesRemoved := false
	if d.HasChange("image_bundles") && oldUserBundleCount > 0 &&
		oldUserBundleCount == newUserBundleCount {
		// Check if old and new have the same user bundle names
		oldUserNames := make(map[string]bool)
		newUserNames := make(map[string]bool)
		for _, b := range oldBundles {
			if bundleMap, ok := b.(map[string]interface{}); ok {
				metadataType, _ := bundleMap["metadata_type"].(string)
				if metadataType != ImageBundleTypeYBAActive {
					if name, _ := bundleMap["name"].(string); name != "" {
						oldUserNames[name] = true
					}
				}
			}
		}
		for _, b := range newBundles {
			if bundleMap, ok := b.(map[string]interface{}); ok {
				metadataType, _ := bundleMap["metadata_type"].(string)
				if metadataType != ImageBundleTypeYBAActive {
					if name, _ := bundleMap["name"].(string); name != "" {
						newUserNames[name] = true
					}
				}
			}
		}
		// If same names, user likely removed bundles from config
		sameNames := len(oldUserNames) == len(newUserNames)
		for name := range oldUserNames {
			if !newUserNames[name] {
				sameNames = false
				break
			}
		}
		if sameNames {
			userBundlesRemoved = true
		}
	} else if oldUserBundleCount > 0 && newUserBundleCount < oldUserBundleCount {
		userBundlesRemoved = true
	}

	// If no YBA-managed bundles and no user bundles being removed, nothing to do
	if len(ybaManagedBundles) == 0 && !userBundlesRemoved {
		return nil
	}

	// If user bundles are being removed, set the new value to just YBA-managed bundles
	if userBundlesRemoved {
		if err := d.SetNew("image_bundles", ybaManagedBundles); err != nil {
			return err
		}
		return nil
	}

	// Check if YBA-managed bundles are already in the new value (by name)
	newBundleNames := make(map[string]bool)
	for _, b := range newBundles {
		if bundleMap, ok := b.(map[string]interface{}); ok {
			if name, _ := bundleMap["name"].(string); name != "" {
				newBundleNames[name] = true
			}
		}
	}

	// Track architectures with user defaults
	userDefaultArch := make(map[string]bool)
	for _, b := range newBundles {
		if bundleMap, ok := b.(map[string]interface{}); ok {
			if useAsDefault, _ := bundleMap["use_as_default"].(bool); useAsDefault {
				if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
					if det, ok := details[0].(map[string]interface{}); ok {
						if arch, _ := det["arch"].(string); arch != "" {
							userDefaultArch[arch] = true
						}
					}
				}
			}
		}
	}

	// Check if we need to add any YBA-managed bundles (by name)
	needsMerge := false
	for _, ybaBundle := range ybaManagedBundles {
		bundleMap := ybaBundle.(map[string]interface{})
		name, _ := bundleMap["name"].(string)
		if !newBundleNames[name] {
			needsMerge = true
			break
		}
	}

	if !needsMerge {
		return nil
	}

	// Build merged bundle list (using slice for TypeList)
	var mergedList []interface{}

	// Add all new bundles first (user-provided bundles)
	for _, b := range newBundles {
		mergedList = append(mergedList, b)
	}

	// Add YBA-managed bundles that aren't already in new value (by name)
	for _, ybaBundle := range ybaManagedBundles {
		bundleMap := ybaBundle.(map[string]interface{})
		name, _ := bundleMap["name"].(string)

		if newBundleNames[name] {
			continue
		}

		// Check if user has a default for this arch - unmark YBA bundle default
		shouldUnmarkDefault := false
		if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
			if det, ok := details[0].(map[string]interface{}); ok {
				arch, _ := det["arch"].(string)
				if userDefaultArch[arch] {
					shouldUnmarkDefault = true
				}
			}
		}

		// Always create a copy to avoid modifying original state
		bundleCopy := copyBundleMap(bundleMap)
		if shouldUnmarkDefault {
			bundleCopy["use_as_default"] = false
		}

		mergedList = append(mergedList, bundleCopy)
	}

	if err := d.SetNew("image_bundles", mergedList); err != nil {
		return err
	}

	return nil
}

// copyBundleMap creates a deep copy of a bundle map
func copyBundleMap(original map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{})
	for k, v := range original {
		if k == "details" {
			if details, ok := v.([]interface{}); ok && len(details) > 0 {
				if detMap, ok := details[0].(map[string]interface{}); ok {
					detCopy := make(map[string]interface{})
					for dk, dv := range detMap {
						detCopy[dk] = dv
					}
					cp[k] = []interface{}{detCopy}
					continue
				}
			}
		}
		cp[k] = v
	}
	return cp
}
