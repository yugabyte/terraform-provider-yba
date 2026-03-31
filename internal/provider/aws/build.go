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
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// buildAWSCloudInfo builds AWS cloud info from schema
// Mirrors yba-cli: awsCloudInfo construction in create_provider.go
func buildAWSCloudInfo(d *schema.ResourceData) (*client.AWSCloudInfo, error) {
	// Disable the deprecated provider-level IMDSv2 flag (Java default = true).
	// CloudImageBundleSetup backwards-compat forces all bundles to true when set.
	awsCloudInfo := &client.AWSCloudInfo{
		UseIMDSv2: utils.GetBoolPointer(false),
	}

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
	skipValidation := d.Get("skip_ssh_keypair_validation").(bool)

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

	return providerutil.EnsureImageBundleDefaults(result)
}

// flattenAWSImageBundles converts AWS custom image bundles to schema format.
// YBA-managed bundles (YBA_ACTIVE, YBA_DEPRECATED) are excluded; they are
// tracked separately via the yba_managed_image_bundles field, consistent with
// the GCP/Azure design.
func flattenAWSImageBundles(imageBundles []client.ImageBundle) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, ib := range imageBundles {
		metadata := ib.GetMetadata()
		metaType := metadata.GetType()
		if metaType == ImageBundleTypeYBAActive ||
			metaType == providerutil.ImageBundleTypeYBADeprecated {
			continue
		}

		bundle := map[string]interface{}{
			"uuid":           ib.GetUuid(),
			"metadata_type":  metaType,
			"name":           ib.GetName(),
			"use_as_default": ib.GetUseAsDefault(),
		}

		details := ib.GetDetails()
		detailsMap := map[string]interface{}{
			"arch":        details.GetArch(),
			"ssh_user":    details.GetSshUser(),
			"ssh_port":    details.GetSshPort(),
			"use_imds_v2": details.GetUseIMDSv2(),
		}

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

// ImageBundleType constants - re-exported from providerutil for local use
const (
	ImageBundleTypeYBAActive = providerutil.ImageBundleTypeYBAActive
)

// ensureAWSRegionEntries populates each bundle's regions map with an empty BundleInfo{}
// for any provider region that is missing. verifyImageBundleDetails requires all regions
// to be present; ybImage=null signals YBA to auto-fetch the default AMI (YBA_ACTIVE).
func ensureAWSRegionEntries(
	bundles []client.ImageBundle,
	regionCodes []string,
) []client.ImageBundle {
	for i := range bundles {
		if bundles[i].Details == nil {
			continue
		}
		if bundles[i].Details.Regions == nil {
			empty := make(map[string]client.BundleInfo)
			bundles[i].Details.Regions = &empty
		}
		regions := *bundles[i].Details.Regions
		for _, code := range regionCodes {
			if _, exists := regions[code]; !exists {
				regions[code] = client.BundleInfo{}
			}
		}
	}
	return bundles
}

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

	// Deactivate removed regions; preserve VPC/SG details and zones from state
	// so AWSProviderValidator does not fail on missing fields.
	for code, oldRegion := range oldByCode {
		if !newRegionCodes[code] {
			uuid, _ := oldRegion["uuid"].(string)
			oldZones, _ := oldRegion["zones"].([]interface{})
			region := client.Region{
				Code:   utils.GetStringPointer(code),
				Name:   utils.GetStringPointer(code),
				Active: utils.GetBoolPointer(false),
				Zones:  mergeZoneUUIDs(oldZones, oldZones), // preserve all zones as-is
				Details: &client.RegionDetails{
					CloudInfo: &client.RegionCloudInfo{
						Aws: &client.AWSRegionCloudInfo{
							SecurityGroupId: utils.GetStringPointer(
								providerutil.GetString(oldRegion, "security_group_id"),
							),
							Vnet: utils.GetStringPointer(
								providerutil.GetString(oldRegion, "vpc_id"),
							),
						},
					},
				},
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

	// Deactivate removed zones; preserve subnet so validateSubnets does not
	// receive an empty ID (it validates inactive zones too).
	for code, oldZone := range oldByCode {
		if !newZoneCodes[code] {
			uuid, _ := oldZone["uuid"].(string)
			subnet, _ := oldZone["subnet"].(string)
			secondarySubnet, _ := oldZone["secondary_subnet"].(string)
			zone := client.AvailabilityZone{
				Code:            utils.GetStringPointer(code),
				Name:            code,
				Active:          utils.GetBoolPointer(false),
				Subnet:          utils.GetStringPointer(subnet),
				SecondarySubnet: utils.GetStringPointer(secondarySubnet),
			}
			if uuid != "" {
				zone.Uuid = utils.GetStringPointer(uuid)
			}
			result = append(result, zone)
		}
	}

	return result
}
