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

package oci

import (
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func buildOCICloudInfo(in ociCloudInput) (*client.OCICloudInfo, error) {
	info := &client.OCICloudInfo{}
	info.SetOciCompartmentId(in.CompartmentID)
	info.SetOciRegion(in.DefaultRegion)
	if in.HostedZoneID != "" {
		info.SetOciHostedZoneId(in.HostedZoneID)
	}
	if err := setOCIAuthForCreate(info, in); err != nil {
		return nil, err
	}
	return info, nil
}

// flattenOCICredentials returns the fields Read writes back from the provider
// list. Provider responses mask ociPrivateKeyContent. private_key_file_path
// is never returned: YBA does not receive the path.
func flattenOCICredentials(info client.OCICloudInfo) map[string]interface{} {
	return map[string]interface{}{
		"compartment_id":         info.GetOciCompartmentId(),
		"default_region":         info.GetOciRegion(),
		"hosted_zone_id":         info.GetOciHostedZoneId(),
		"use_instance_principal": info.GetOciAuthType() == ociAuthInstancePrincipal,
		"tenancy_id":             info.GetOciTenancyId(),
		"user_id":                info.GetOciUserId(),
		"fingerprint":            info.GetOciFingerprint(),
		"vpc_type":               info.GetVpcType(),
	}
}

func buildOCIAccessKeys(name, content string) []client.AccessKey {
	if name == "" && content == "" {
		return nil
	}
	return []client.AccessKey{{
		KeyInfo: client.KeyInfo{
			KeyPairName:          utils.GetStringPointer(name),
			SshPrivateKeyContent: utils.GetStringPointer(content),
		},
	}}
}

func ociRegionCloudInfo(vcnID, instanceConfigurationID string) *client.OCIRegionCloudInfo {
	info := &client.OCIRegionCloudInfo{
		Vnet: utils.GetStringPointer(vcnID),
	}
	if instanceConfigurationID != "" {
		info.SetInstanceTemplate(instanceConfigurationID)
	}
	return info
}

func buildOCIRegions(regions []interface{}) []client.Region {
	result := make([]client.Region, 0, len(regions))
	for _, r := range regions {
		regionMap := r.(map[string]interface{})
		regionCode := regionMap["code"].(string)
		zonesList, _ := regionMap["zones"].([]interface{})
		region := client.Region{
			Code:  utils.GetStringPointer(regionCode),
			Name:  utils.GetStringPointer(regionCode),
			Zones: buildOCIZones(zonesList),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Oci: ociRegionCloudInfo(
						regionMap["vcn_id"].(string),
						providerutil.GetString(regionMap, "instance_configuration_id"),
					),
				},
			},
		}
		if uuid, ok := regionMap["uuid"].(string); ok && uuid != "" {
			region.Uuid = utils.GetStringPointer(uuid)
		}
		result = append(result, region)
	}
	return result
}

func buildOCIZones(zones []interface{}) []client.AvailabilityZone {
	result := make([]client.AvailabilityZone, 0, len(zones))
	for _, z := range zones {
		zoneMap := z.(map[string]interface{})
		zoneCode := zoneMap["code"].(string)
		zone := client.AvailabilityZone{
			Code:   utils.GetStringPointer(zoneCode),
			Name:   zoneCode,
			Subnet: utils.GetStringPointer(zoneMap["subnet"].(string)),
		}
		if uuid, ok := zoneMap["uuid"].(string); ok && uuid != "" {
			zone.Uuid = utils.GetStringPointer(uuid)
		}
		result = append(result, zone)
	}
	return result
}

func flattenOCIRegions(regions []client.Region) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, region := range regions {
		if region.HasActive() && !region.GetActive() {
			continue
		}
		details := region.GetDetails()
		cloudInfo := details.GetCloudInfo()
		oci := cloudInfo.GetOci()
		result = append(result, map[string]interface{}{
			"uuid":                      region.GetUuid(),
			"code":                      region.GetCode(),
			"name":                      region.GetName(),
			"vcn_id":                    oci.GetVnet(),
			"instance_configuration_id": oci.GetInstanceTemplate(),
			"zones":                     flattenOCIZones(region.GetZones()),
		})
	}
	return result
}

func flattenOCIZones(zones []client.AvailabilityZone) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, zone := range zones {
		if zone.HasActive() && !zone.GetActive() {
			continue
		}
		result = append(result, map[string]interface{}{
			"uuid":   zone.GetUuid(),
			"code":   zone.GetCode(),
			"name":   zone.GetName(),
			"subnet": zone.GetSubnet(),
		})
	}
	return result
}

func buildOCIImageBundles(imageBundles []interface{}) []client.ImageBundle {
	result := make([]client.ImageBundle, 0, len(imageBundles))
	for _, ib := range imageBundles {
		bundleMap := ib.(map[string]interface{})
		detailsList := bundleMap["details"].([]interface{})
		if len(detailsList) == 0 {
			continue
		}
		detailsMap := detailsList[0].(map[string]interface{})
		sshPort := int32(detailsMap["ssh_port"].(int))
		if sshPort == 0 {
			sshPort = 22
		}
		details := client.ImageBundleDetails{
			Arch:    utils.GetStringPointer(detailsMap["arch"].(string)),
			SshUser: utils.GetStringPointer(detailsMap["ssh_user"].(string)),
			SshPort: utils.GetInt32Pointer(sshPort),
		}
		if v, ok := detailsMap["region_overrides"].(map[string]interface{}); ok && len(v) > 0 {
			regionOverrides := make(map[string]client.BundleInfo, len(v))
			for regionCode, imageID := range v {
				regionOverrides[regionCode] = client.BundleInfo{
					YbImage: utils.GetStringPointer(imageID.(string)),
				}
			}
			details.SetRegions(regionOverrides)
		}
		result = append(result, client.ImageBundle{
			Name:         utils.GetStringPointer(bundleMap["name"].(string)),
			UseAsDefault: utils.GetBoolPointer(bundleMap["use_as_default"].(bool)),
			Details:      &details,
		})
	}
	return result
}

func flattenOCIImageBundles(imageBundles []client.ImageBundle) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, ib := range imageBundles {
		metadata := ib.GetMetadata()
		metaType := metadata.GetType()
		if metaType == providerutil.ImageBundleTypeYBAActive ||
			metaType == providerutil.ImageBundleTypeYBADeprecated {
			continue
		}
		details := ib.GetDetails()
		detailsMap := map[string]interface{}{
			"arch":     details.GetArch(),
			"ssh_user": details.GetSshUser(),
			"ssh_port": details.GetSshPort(),
		}
		if regionOverrides := details.GetRegions(); len(regionOverrides) > 0 {
			overridesMap := make(map[string]interface{}, len(regionOverrides))
			for regionCode, bundleInfo := range regionOverrides {
				overridesMap[regionCode] = bundleInfo.GetYbImage()
			}
			detailsMap["region_overrides"] = overridesMap
		}
		result = append(result, map[string]interface{}{
			"uuid":           ib.GetUuid(),
			"metadata_type":  metaType,
			"name":           ib.GetName(),
			"use_as_default": ib.GetUseAsDefault(),
			"details":        []interface{}{detailsMap},
		})
	}
	return result
}

// ensureOCIRegionEntries adds an empty BundleInfo for every configured region
// that a bundle does not already name. YBA's image check requires the entry;
// an empty YbImage tells YBA to fetch its own image for a YBA-managed bundle.
func ensureOCIRegionEntries(
	bundles []client.ImageBundle, regionCodes []string,
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

func mergeRegionUUIDs(oldRegions, newRegions []interface{}) []client.Region {
	oldByCode := make(map[string]map[string]interface{}, len(oldRegions))
	for _, r := range oldRegions {
		regionMap := r.(map[string]interface{})
		oldByCode[regionMap["code"].(string)] = regionMap
	}
	newRegionCodes := make(map[string]bool, len(newRegions))
	result := make([]client.Region, 0, len(newRegions))
	for _, nr := range newRegions {
		newMap := nr.(map[string]interface{})
		regionCode := newMap["code"].(string)
		newRegionCodes[regionCode] = true
		newZones, _ := newMap["zones"].([]interface{})
		region := client.Region{
			Code: utils.GetStringPointer(regionCode),
			Name: utils.GetStringPointer(regionCode),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Oci: ociRegionCloudInfo(
						newMap["vcn_id"].(string),
						providerutil.GetString(newMap, "instance_configuration_id"),
					),
				},
			},
		}
		if oldRegion, exists := oldByCode[regionCode]; exists {
			if uuid, ok := oldRegion["uuid"].(string); ok && uuid != "" {
				region.Uuid = utils.GetStringPointer(uuid)
			}
			oldZones, _ := oldRegion["zones"].([]interface{})
			region.Zones = mergeZoneUUIDs(oldZones, newZones)
		} else {
			region.Zones = buildOCIZones(newZones)
		}
		result = append(result, region)
	}
	for code, oldRegion := range oldByCode {
		if newRegionCodes[code] {
			continue
		}
		oldZones, _ := oldRegion["zones"].([]interface{})
		region := client.Region{
			Code:   utils.GetStringPointer(code),
			Name:   utils.GetStringPointer(code),
			Active: utils.GetBoolPointer(false),
			Zones:  mergeZoneUUIDs(oldZones, oldZones),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Oci: ociRegionCloudInfo(
						providerutil.GetString(oldRegion, "vcn_id"),
						providerutil.GetString(oldRegion, "instance_configuration_id"),
					),
				},
			},
		}
		if uuid, _ := oldRegion["uuid"].(string); uuid != "" {
			region.Uuid = utils.GetStringPointer(uuid)
		}
		result = append(result, region)
	}
	return result
}

func mergeZoneUUIDs(oldZones, newZones []interface{}) []client.AvailabilityZone {
	oldByCode := make(map[string]map[string]interface{}, len(oldZones))
	for _, z := range oldZones {
		zoneMap := z.(map[string]interface{})
		oldByCode[zoneMap["code"].(string)] = zoneMap
	}
	newZoneCodes := make(map[string]bool, len(newZones))
	result := make([]client.AvailabilityZone, 0, len(newZones))
	for _, nz := range newZones {
		newMap := nz.(map[string]interface{})
		zoneCode := newMap["code"].(string)
		newZoneCodes[zoneCode] = true
		zone := client.AvailabilityZone{
			Code:   utils.GetStringPointer(zoneCode),
			Name:   zoneCode,
			Subnet: utils.GetStringPointer(newMap["subnet"].(string)),
		}
		if oldZone, exists := oldByCode[zoneCode]; exists {
			if uuid, ok := oldZone["uuid"].(string); ok && uuid != "" {
				zone.Uuid = utils.GetStringPointer(uuid)
			}
		}
		result = append(result, zone)
	}
	for code, oldZone := range oldByCode {
		if newZoneCodes[code] {
			continue
		}
		zone := client.AvailabilityZone{
			Code:   utils.GetStringPointer(code),
			Name:   code,
			Active: utils.GetBoolPointer(false),
			Subnet: utils.GetStringPointer(providerutil.GetString(oldZone, "subnet")),
		}
		if uuid, _ := oldZone["uuid"].(string); uuid != "" {
			zone.Uuid = utils.GetStringPointer(uuid)
		}
		result = append(result, zone)
	}
	return result
}
