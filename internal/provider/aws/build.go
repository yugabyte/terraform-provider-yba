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
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// buildAWSCloudInfo builds AWS cloud info from schema
// Mirrors yba-cli: awsCloudInfo construction in create_provider.go
func buildAWSCloudInfo(d *schema.ResourceData) (*client.AWSCloudInfo, error) {
	awsCloudInfo := &client.AWSCloudInfo{}

	isIAM := d.Get("use_iam_instance_profile").(bool)

	// Set hosted zone ID if provided
	if v, ok := d.GetOk("hosted_zone_id"); ok {
		awsCloudInfo.SetAwsHostedZoneId(v.(string))
	}

	// If not using IAM, credentials are required
	if !isIAM {
		accessKeyID := d.Get("access_key_id").(string)
		secretAccessKey := d.Get("secret_access_key").(string)

		if accessKeyID == "" || secretAccessKey == "" {
			return nil, fmt.Errorf("AWS credentials required: set access_key_id and " +
				"secret_access_key, or use use_iam_instance_profile=true")
		}
		awsCloudInfo.SetAwsAccessKeyID(accessKeyID)
		awsCloudInfo.SetAwsAccessKeySecret(secretAccessKey)
	}

	return awsCloudInfo, nil
}

// buildAWSAccessKeys builds access keys for AWS provider.
// Returns nil when both ssh_keypair_name and ssh_private_key_content are empty,
// which causes allAccessKeys to be omitted from the request and lets YBA generate
// a managed keypair - matching UI behavior for the YBA-managed mode.
func buildAWSAccessKeys(d *schema.ResourceData) []client.AccessKey {
	keyPairName := d.Get("ssh_keypair_name").(string)
	sshContent := d.Get("ssh_private_key_content").(string)
	// Support both the current name and the deprecated alias.
	skipValidation := d.Get("skip_ssh_keypair_validation").(bool)
	if !skipValidation {
		skipValidation = d.Get("skip_keypair_validation").(bool)
	}

	if keyPairName == "" && sshContent == "" {
		return nil
	}

	return []client.AccessKey{
		{
			KeyInfo: client.KeyInfo{
				KeyPairName:              utils.GetStringPointer(keyPairName),
				SshPrivateKeyContent:     utils.GetStringPointer(sshContent),
				SkipKeyValidateAndUpload: utils.GetBoolPointer(skipValidation),
			},
		},
	}
}

// buildAWSRegions builds AWS regions from schema
// Mirrors yba-cli: buildAWSRegions in create_provider.go
func buildAWSRegions(regions []interface{}) []client.Region {
	result := make([]client.Region, 0)

	for _, r := range regions {
		regionMap := r.(map[string]interface{})
		regionCode := regionMap["code"].(string)

		// Build zones for this region (TypeList returns []interface{})
		zonesList, _ := regionMap["zones"].([]interface{})
		zones := buildAWSZones(zonesList)

		region := client.Region{
			Code:  utils.GetStringPointer(regionCode),
			Name:  utils.GetStringPointer(regionCode),
			Zones: zones,
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Aws: &client.AWSRegionCloudInfo{
						SecurityGroupId: utils.GetStringPointer(
							regionMap["security_group_id"].(string),
						),
						Vnet: utils.GetStringPointer(regionMap["vpc_id"].(string)),
					},
				},
			},
		}

		// Include UUID for existing regions (needed for updates)
		if uuid, ok := regionMap["uuid"].(string); ok && uuid != "" {
			region.Uuid = utils.GetStringPointer(uuid)
		}

		result = append(result, region)
	}

	return result
}

// buildAWSZones builds zones for a region
// Mirrors yba-cli: buildAWSZones in create_provider.go
func buildAWSZones(zones []interface{}) []client.AvailabilityZone {
	result := make([]client.AvailabilityZone, 0)

	for _, z := range zones {
		zoneMap := z.(map[string]interface{})
		zoneCode := zoneMap["code"].(string)

		zone := client.AvailabilityZone{
			Code:            utils.GetStringPointer(zoneCode),
			Name:            zoneCode,
			Subnet:          utils.GetStringPointer(zoneMap["subnet"].(string)),
			SecondarySubnet: utils.GetStringPointer(zoneMap["secondary_subnet"].(string)),
		}

		// Include UUID for existing zones (needed for updates)
		if uuid, ok := zoneMap["uuid"].(string); ok && uuid != "" {
			zone.Uuid = utils.GetStringPointer(uuid)
		}

		result = append(result, zone)
	}

	return result
}

// flattenAWSRegions converts API regions to schema format
func flattenAWSRegions(regions []client.Region) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, region := range regions {
		r := map[string]interface{}{
			"uuid":  region.GetUuid(),
			"code":  region.GetCode(),
			"name":  region.GetName(),
			"zones": flattenAWSZones(region.GetZones()),
		}

		// Extract AWS-specific region info
		details := region.GetDetails()
		cloudInfo := details.GetCloudInfo()
		awsInfo := cloudInfo.GetAws()
		r["vpc_id"] = awsInfo.GetVnet()
		r["security_group_id"] = awsInfo.GetSecurityGroupId()

		result = append(result, r)
	}

	return result
}

// flattenAWSZones converts API zones to schema format
func flattenAWSZones(zones []client.AvailabilityZone) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid":             zone.GetUuid(),
			"code":             zone.GetCode(),
			"name":             zone.GetName(),
			"subnet":           zone.GetSubnet(),
			"secondary_subnet": zone.GetSecondarySubnet(),
		}
		result = append(result, z)
	}

	return result
}

// buildAWSImageBundles builds AWS image bundles with region overrides
func buildAWSImageBundles(imageBundles []interface{}) []client.ImageBundle {
	result := make([]client.ImageBundle, 0)

	for _, ib := range imageBundles {
		bundleMap := ib.(map[string]interface{})
		name := bundleMap["name"].(string)
		useAsDefault := bundleMap["use_as_default"].(bool)

		detailsList := bundleMap["details"].([]interface{})
		if len(detailsList) == 0 {
			continue
		}
		detailsMap := detailsList[0].(map[string]interface{})

		details := client.ImageBundleDetails{
			Arch:      utils.GetStringPointer(detailsMap["arch"].(string)),
			SshUser:   utils.GetStringPointer(detailsMap["ssh_user"].(string)),
			SshPort:   utils.GetInt32Pointer(int32(detailsMap["ssh_port"].(int))),
			UseIMDSv2: utils.GetBoolPointer(detailsMap["use_imds_v2"].(bool)),
		}

		// Global YB image
		if v, ok := detailsMap["global_yb_image"].(string); ok && v != "" {
			details.SetGlobalYbImage(v)
		}

		// AWS-specific: Region overrides
		if v, ok := detailsMap["region_overrides"].(map[string]interface{}); ok && len(v) > 0 {
			regionOverrides := make(map[string]client.BundleInfo)
			for regionCode, amiID := range v {
				regionOverrides[regionCode] = client.BundleInfo{
					YbImage: utils.GetStringPointer(amiID.(string)),
				}
			}
			details.SetRegions(regionOverrides)
		}

		bundle := client.ImageBundle{
			Name:         utils.GetStringPointer(name),
			UseAsDefault: utils.GetBoolPointer(useAsDefault),
			Details:      &details,
		}
		result = append(result, bundle)
	}

	return result
}

// flattenAWSImageBundles converts AWS image bundles with region overrides to schema format
func flattenAWSImageBundles(imageBundles []client.ImageBundle) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, ib := range imageBundles {
		metadata := ib.GetMetadata()
		bundle := map[string]interface{}{
			"uuid":           ib.GetUuid(),
			"metadata_type":  metadata.GetType(),
			"name":           ib.GetName(),
			"use_as_default": ib.GetUseAsDefault(),
		}

		details := ib.GetDetails()
		detailsMap := map[string]interface{}{
			"arch":            details.GetArch(),
			"ssh_user":        details.GetSshUser(),
			"ssh_port":        details.GetSshPort(),
			"use_imds_v2":     details.GetUseIMDSv2(),
			"global_yb_image": details.GetGlobalYbImage(),
		}

		// AWS-specific: Region overrides
		regionOverrides := details.GetRegions()
		if len(regionOverrides) > 0 {
			overridesMap := make(map[string]interface{})
			for regionCode, bundleInfo := range regionOverrides {
				overridesMap[regionCode] = bundleInfo.GetYbImage()
			}
			detailsMap["region_overrides"] = overridesMap
		}

		bundle["details"] = []interface{}{detailsMap}
		result = append(result, bundle)
	}

	return result
}

// ImageBundleType constants matching YBA's ImageBundleType enum
const (
	ImageBundleTypeYBAActive     = "YBA_ACTIVE"
	ImageBundleTypeYBADeprecated = "YBA_DEPRECATED"
	ImageBundleTypeCustom        = "CUSTOM"
)

// mergeRegionUUIDs merges UUIDs from old state into new config regions.
// Works with TypeList ([]interface{}).
func mergeRegionUUIDs(
	oldRegions []interface{},
	newRegions []interface{},
) []client.Region {
	// Build a map of old regions by code to get UUIDs
	oldByCode := make(map[string]map[string]interface{})
	for _, r := range oldRegions {
		regionMap := r.(map[string]interface{})
		code := regionMap["code"].(string)
		oldByCode[code] = regionMap
	}

	// Track which old regions are still in config
	newRegionCodes := make(map[string]bool)

	result := make([]client.Region, 0, len(newRegions))

	for _, nr := range newRegions {
		newMap := nr.(map[string]interface{})
		regionCode := newMap["code"].(string)
		newRegionCodes[regionCode] = true

		// Get zones from new config (TypeList returns []interface{})
		newZones, _ := newMap["zones"].([]interface{})

		// Build the region
		region := client.Region{
			Code: utils.GetStringPointer(regionCode),
			Name: utils.GetStringPointer(regionCode),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Aws: &client.AWSRegionCloudInfo{
						SecurityGroupId: utils.GetStringPointer(
							newMap["security_group_id"].(string),
						),
						Vnet: utils.GetStringPointer(newMap["vpc_id"].(string)),
					},
				},
			},
		}

		// If this region exists in old state, copy UUID
		if oldRegion, exists := oldByCode[regionCode]; exists {
			if uuid, ok := oldRegion["uuid"].(string); ok && uuid != "" {
				region.Uuid = utils.GetStringPointer(uuid)
			}

			// Get old zones for UUID lookup (TypeList returns []interface{})
			oldZones, _ := oldRegion["zones"].([]interface{})
			region.Zones = mergeZoneUUIDs(oldZones, newZones)
		} else {
			// New region - no UUIDs to preserve
			region.Zones = buildAWSZones(newZones)
		}

		result = append(result, region)
	}

	// Handle removed regions - mark them as inactive (like yba-cli does)
	for code, oldRegion := range oldByCode {
		if !newRegionCodes[code] {
			uuid, _ := oldRegion["uuid"].(string)
			region := client.Region{
				Code:   utils.GetStringPointer(code),
				Name:   utils.GetStringPointer(code),
				Active: utils.GetBoolPointer(false),
			}
			if uuid != "" {
				region.Uuid = utils.GetStringPointer(uuid)
			}
			result = append(result, region)
		}
	}

	return result
}

// mergeZoneUUIDs merges UUIDs from old state zones into new config zones.
// Also marks removed zones as inactive (like yba-cli does).
func mergeZoneUUIDs(
	oldZones []interface{},
	newZones []interface{},
) []client.AvailabilityZone {
	// Build a map of old zones by code to get UUIDs
	oldByCode := make(map[string]map[string]interface{})
	for _, z := range oldZones {
		zoneMap := z.(map[string]interface{})
		code := zoneMap["code"].(string)
		oldByCode[code] = zoneMap
	}

	// Track which old zones are still in config
	newZoneCodes := make(map[string]bool)

	result := make([]client.AvailabilityZone, 0, len(newZones))

	for _, nz := range newZones {
		newMap := nz.(map[string]interface{})
		zoneCode := newMap["code"].(string)
		newZoneCodes[zoneCode] = true

		zone := client.AvailabilityZone{
			Code:            utils.GetStringPointer(zoneCode),
			Name:            zoneCode,
			Subnet:          utils.GetStringPointer(newMap["subnet"].(string)),
			SecondarySubnet: utils.GetStringPointer(newMap["secondary_subnet"].(string)),
		}

		// If this zone exists in old state, copy UUID
		if oldZone, exists := oldByCode[zoneCode]; exists {
			if uuid, ok := oldZone["uuid"].(string); ok && uuid != "" {
				zone.Uuid = utils.GetStringPointer(uuid)
			}
		}

		result = append(result, zone)
	}

	// Handle removed zones - mark them as inactive (like yba-cli does)
	for code, oldZone := range oldByCode {
		if !newZoneCodes[code] {
			uuid, _ := oldZone["uuid"].(string)
			zone := client.AvailabilityZone{
				Code:   utils.GetStringPointer(code),
				Name:   code,
				Active: utils.GetBoolPointer(false),
			}
			if uuid != "" {
				zone.Uuid = utils.GetStringPointer(uuid)
			}
			result = append(result, zone)
		}
	}

	return result
}

// mergeImageBundlesForUpdate merges old state bundles with new config bundles.
// It preserves YBA-managed bundles, handles default conflicts, and copies UUIDs.
func mergeImageBundlesForUpdate(oldBundlesRaw, newBundlesRaw interface{}) []client.ImageBundle {
	// Get old bundles from state (TypeList returns []interface{} directly)
	oldBundlesList, _ := oldBundlesRaw.([]interface{})
	newBundlesList, _ := newBundlesRaw.([]interface{})

	// Build maps from state data for quick lookup
	oldByName := make(map[string]map[string]interface{})
	for _, b := range oldBundlesList {
		if bundleMap, ok := b.(map[string]interface{}); ok {
			if name, _ := bundleMap["name"].(string); name != "" {
				oldByName[name] = bundleMap
			}
		}
	}

	// Detect if user removed bundles from config:
	// Due to Optional+Computed, when user removes bundles, Terraform fills newBundles
	// with identical copies from state. Detect this by checking if all user bundles
	// in new are identical to old (same UUID = copied from state, not user-provided).
	userBundlesRemoved := detectUserBundleRemoval(oldBundlesList, newBundlesList)

	// Collect YBA-managed bundle names (to skip them in new bundles loop)
	ybaManagedNames := make(map[string]bool)
	for _, b := range oldBundlesList {
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

	// Check if user is adding a bundle with use_as_default=true
	// If so, we'll need to unmark the YBA bundle's default
	userHasDefault := false
	for _, b := range newBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := bundleMap["name"].(string)
		// Skip YBA bundles and empty names
		if name == "" || ybaManagedNames[name] {
			continue
		}
		if useAsDefault, _ := bundleMap["use_as_default"].(bool); useAsDefault {
			userHasDefault = true
			break
		}
	}

	// Build the final bundles list
	var resultBundles []client.ImageBundle

	// First, add YBA-managed bundles from state (always preserve)
	// If user has a default, unmark YBA bundle's default to avoid conflict
	for _, b := range oldBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType == ImageBundleTypeYBAActive {
			bundle := buildImageBundleFromState(bundleMap)
			// Handle default conflict: unmark YBA bundle if user has a default
			if userHasDefault && bundle.GetUseAsDefault() {
				bundle.SetUseAsDefault(false)
			}
			resultBundles = append(resultBundles, bundle)
		}
	}

	// If user bundles were removed, skip processing user bundles from new value
	// (they are just copies from state due to Optional+Computed behavior)
	if userBundlesRemoved {
		return resultBundles
	}

	// Process user bundles from new value
	for _, b := range newBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := bundleMap["name"].(string)
		if name == "" {
			continue // Skip corrupted entries
		}

		// Skip YBA-managed bundles - they're already added from old state
		if ybaManagedNames[name] {
			continue
		}

		useAsDefault, _ := bundleMap["use_as_default"].(bool)

		// Get details
		var arch, sshUser, globalYbImage string
		var sshPort int
		var regionOverrides map[string]interface{}

		if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
			if det, ok := details[0].(map[string]interface{}); ok {
				arch, _ = det["arch"].(string)
				sshUser, _ = det["ssh_user"].(string)
				sshPort, _ = det["ssh_port"].(int)
				globalYbImage, _ = det["global_yb_image"].(string)
				regionOverrides, _ = det["region_overrides"].(map[string]interface{})
			}
		}

		// If arch is empty, data might be corrupted - skip
		if arch == "" {
			continue
		}

		// Build the bundle
		bundle := client.ImageBundle{
			Name:         utils.GetStringPointer(name),
			UseAsDefault: utils.GetBoolPointer(useAsDefault),
			Details: &client.ImageBundleDetails{
				Arch:          utils.GetStringPointer(arch),
				SshUser:       utils.GetStringPointer(sshUser),
				SshPort:       utils.GetInt32Pointer(int32(sshPort)),
				UseIMDSv2:     utils.GetBoolPointer(true),
				GlobalYbImage: utils.GetStringPointer(globalYbImage),
			},
		}

		// Add region overrides if present
		if len(regionOverrides) > 0 {
			overridesMap := make(map[string]client.BundleInfo)
			for region, ami := range regionOverrides {
				overridesMap[region] = client.BundleInfo{
					YbImage: utils.GetStringPointer(ami.(string)),
				}
			}
			bundle.Details.Regions = &overridesMap
		}

		// Copy UUID from the bundle (TypeList preserves UUID when editing in-place)
		// This handles renames: user changes name but UUID is preserved at same position
		if uuid, _ := bundleMap["uuid"].(string); uuid != "" {
			bundle.Uuid = utils.GetStringPointer(uuid)
		} else if oldBundle, exists := oldByName[name]; exists {
			// Fallback: look up by name for newly added bundles that match existing names
			if uuid, _ := oldBundle["uuid"].(string); uuid != "" {
				bundle.Uuid = utils.GetStringPointer(uuid)
			}
		}

		resultBundles = append(resultBundles, bundle)
	}

	return resultBundles
}

// detectUserBundleRemoval checks if user removed bundles from config.
// Due to Optional+Computed behavior, when user removes image_bundles block,
// Terraform fills newBundles with copies from state. Detect this by checking
// if all CUSTOM bundles in new have matching UUIDs AND names in old.
// If names differ, it's an EDIT (not removal) even if UUID matches.
func detectUserBundleRemoval(oldBundles, newBundles []interface{}) bool {
	// Build map of old CUSTOM bundles: uuid -> name
	oldCustomBundles := make(map[string]string) // uuid -> name
	var oldCustomCount int
	for _, b := range oldBundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType != ImageBundleTypeYBAActive {
			oldCustomCount++
			uuid, _ := bundleMap["uuid"].(string)
			name, _ := bundleMap["name"].(string)
			if uuid != "" {
				oldCustomBundles[uuid] = name
			}
		}
	}

	// If no CUSTOM bundles in old, can't detect removal this way
	if oldCustomCount == 0 {
		return false
	}

	// Count new CUSTOM bundles and check if they all match old (same UUID AND name)
	var newCustomCount int
	allMatchOld := true
	for _, b := range newBundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType != ImageBundleTypeYBAActive {
			newCustomCount++
			uuid, _ := bundleMap["uuid"].(string)
			name, _ := bundleMap["name"].(string)
			// Check if UUID exists in old AND name matches
			// If name differs, it's an edit (user changed the name), not a removal
			if oldName, exists := oldCustomBundles[uuid]; !exists || oldName != name {
				allMatchOld = false
			}
		}
	}

	// If same count and all new CUSTOM bundles match old (UUID + name),
	// user likely removed bundles from config and Terraform copied from state
	if newCustomCount == oldCustomCount && allMatchOld {
		return true
	}

	return false
}

// buildImageBundleFromState converts state data to client.ImageBundle
func buildImageBundleFromState(bundleMap map[string]interface{}) client.ImageBundle {
	name, _ := bundleMap["name"].(string)
	useAsDefault, _ := bundleMap["use_as_default"].(bool)
	uuid, _ := bundleMap["uuid"].(string)

	bundle := client.ImageBundle{
		Name:         utils.GetStringPointer(name),
		UseAsDefault: utils.GetBoolPointer(useAsDefault),
	}

	if uuid != "" {
		bundle.Uuid = utils.GetStringPointer(uuid)
	}

	// Get details
	if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
		if det, ok := details[0].(map[string]interface{}); ok {
			arch, _ := det["arch"].(string)
			sshUser, _ := det["ssh_user"].(string)
			sshPort, _ := det["ssh_port"].(int)
			globalYbImage, _ := det["global_yb_image"].(string)
			useIMDSv2, _ := det["use_imds_v2"].(bool)

			bundle.Details = &client.ImageBundleDetails{
				Arch:          utils.GetStringPointer(arch),
				SshUser:       utils.GetStringPointer(sshUser),
				SshPort:       utils.GetInt32Pointer(int32(sshPort)),
				UseIMDSv2:     utils.GetBoolPointer(useIMDSv2),
				GlobalYbImage: utils.GetStringPointer(globalYbImage),
			}

			// Add region overrides if present
			if regionOverrides, ok := det["region_overrides"].(map[string]interface{}); ok &&
				len(regionOverrides) > 0 {
				overridesMap := make(map[string]client.BundleInfo)
				for region, ami := range regionOverrides {
					overridesMap[region] = client.BundleInfo{
						YbImage: utils.GetStringPointer(ami.(string)),
					}
				}
				bundle.Details.Regions = &overridesMap
			}
		}
	}

	return bundle
}
