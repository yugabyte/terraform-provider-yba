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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// buildAzureCloudInfo builds Azure cloud info from schema.
//
// Authentication mode is determined solely by client_secret:
//   - client_secret set     -> ClientSecretCredential (service principal)
//   - client_secret not set -> DefaultAzureCredential (managed identity / env vars)
//
// client_id, subscription_id, tenant_id, and resource_group are sent in both
// modes when provided: client_id doubles as the user-assigned managed identity
// client ID, and the remaining fields identify the Azure subscription/RG.
func buildAzureCloudInfo(d *schema.ResourceData) (*client.AzureCloudInfo, error) {
	azureCloudInfo := &client.AzureCloudInfo{}

	useManagedIdentity := d.Get("use_managed_identity").(bool)

	// Set optional network/DNS fields
	if v, ok := d.GetOk("hosted_zone_id"); ok {
		azureCloudInfo.SetAzuHostedZoneId(v.(string))
	}
	if v, ok := d.GetOk("network_subscription_id"); ok {
		azureCloudInfo.SetAzuNetworkSubscriptionId(v.(string))
	}
	if v, ok := d.GetOk("network_resource_group"); ok {
		azureCloudInfo.SetAzuNetworkRG(v.(string))
	}

	// Required in both auth modes: client_id, subscription_id, tenant_id, resource_group.
	// The schema enforces their presence so no extra validation is needed here.
	azureCloudInfo.SetAzuClientId(d.Get("client_id").(string))
	azureCloudInfo.SetAzuSubscriptionId(d.Get("subscription_id").(string))
	azureCloudInfo.SetAzuTenantId(d.Get("tenant_id").(string))
	azureCloudInfo.SetAzuRG(d.Get("resource_group").(string))

	// client_secret is the sole auth differentiator:
	// present = service principal, absent = managed identity
	if !useManagedIdentity {
		azureCloudInfo.SetAzuClientSecret(d.Get("client_secret").(string))
	}

	return azureCloudInfo, nil
}

// buildAzureAccessKeys builds access keys for Azure provider.
// Returns nil when both ssh_keypair_name and ssh_private_key_content are empty,
// which causes allAccessKeys to be omitted from the request and lets YBA generate
// a managed keypair - matching UI behavior for the YBA-managed mode.
func buildAzureAccessKeys(d *schema.ResourceData) []client.AccessKey {
	keyPairName := d.Get("ssh_keypair_name").(string)
	sshContent := d.Get("ssh_private_key_content").(string)

	if keyPairName == "" && sshContent == "" {
		return nil
	}

	return []client.AccessKey{
		{
			KeyInfo: client.KeyInfo{
				KeyPairName:          utils.GetStringPointer(keyPairName),
				SshPrivateKeyContent: utils.GetStringPointer(sshContent),
			},
		},
	}
}

// buildAzureRegions builds Azure regions from schema
// Mirrors yba-cli: buildAzureRegions pattern
func buildAzureRegions(regions []interface{}) []client.Region {
	result := make([]client.Region, 0)

	for _, r := range regions {
		regionMap := r.(map[string]interface{})
		regionCode := regionMap["code"].(string)

		// Build zones for this region
		zones := buildAzureZones(regionMap["zones"].([]interface{}))

		azuInfo := &client.AzureRegionCloudInfo{
			Vnet:            utils.GetStringPointer(regionMap["vnet"].(string)),
			SecurityGroupId: utils.GetStringPointer(regionMap["security_group_id"].(string)),
		}
		if v, ok := regionMap["resource_group"].(string); ok && v != "" {
			azuInfo.SetAzuRGOverride(v)
		}
		if v, ok := regionMap["network_resource_group"].(string); ok && v != "" {
			azuInfo.SetAzuNetworkRGOverride(v)
		}
		region := client.Region{
			Code:  utils.GetStringPointer(regionCode),
			Name:  utils.GetStringPointer(regionCode),
			Zones: zones,
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{Azu: azuInfo},
			},
		}
		result = append(result, region)
	}

	return result
}

// buildAzureZones builds zones for a region
func buildAzureZones(zones []interface{}) []client.AvailabilityZone {
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
		result = append(result, zone)
	}

	return result
}

// mergeZoneUUIDs merges state UUIDs into config zones and deactivates removed ones,
// preserving subnet so the Azure validator never receives an empty subnet ID.
func mergeZoneUUIDs(
	oldZones []interface{},
	newZones []interface{},
) []client.AvailabilityZone {
	oldByCode := make(map[string]map[string]interface{})
	for _, z := range oldZones {
		if zoneMap, ok := z.(map[string]interface{}); ok {
			if code, _ := zoneMap["code"].(string); code != "" {
				oldByCode[code] = zoneMap
			}
		}
	}

	newZoneCodes := make(map[string]bool)
	result := make([]client.AvailabilityZone, 0, len(newZones))

	for _, nz := range newZones {
		newMap, ok := nz.(map[string]interface{})
		if !ok {
			continue
		}
		zoneCode, _ := newMap["code"].(string)
		newZoneCodes[zoneCode] = true

		zone := client.AvailabilityZone{
			Code:            utils.GetStringPointer(zoneCode),
			Name:            zoneCode,
			Subnet:          utils.GetStringPointer(newMap["subnet"].(string)),
			SecondarySubnet: utils.GetStringPointer(newMap["secondary_subnet"].(string)),
		}
		if old, exists := oldByCode[zoneCode]; exists {
			if uuid, _ := old["uuid"].(string); uuid != "" {
				zone.Uuid = utils.GetStringPointer(uuid)
			}
		}
		result = append(result, zone)
	}

	// Deactivate removed zones; subnet preserved for the Azure validator.
	for code, oldZone := range oldByCode {
		if !newZoneCodes[code] {
			uuid, _ := oldZone["uuid"].(string)
			subnet, _ := oldZone["subnet"].(string)
			secondary, _ := oldZone["secondary_subnet"].(string)
			zone := client.AvailabilityZone{
				Code:            utils.GetStringPointer(code),
				Name:            code,
				Active:          utils.GetBoolPointer(false),
				Subnet:          utils.GetStringPointer(subnet),
				SecondarySubnet: utils.GetStringPointer(secondary),
			}
			if uuid != "" {
				zone.Uuid = utils.GetStringPointer(uuid)
			}
			result = append(result, zone)
		}
	}

	return result
}

// mergeRegionUUIDs merges state UUIDs into config regions, deactivates removed regions
// with zones/subnets preserved, and deactivates removed zones within active regions.
func mergeRegionUUIDs(
	oldRegions []interface{},
	newRegions []interface{},
) []client.Region {
	oldByCode := make(map[string]map[string]interface{})
	for _, r := range oldRegions {
		if regionMap, ok := r.(map[string]interface{}); ok {
			if code, _ := regionMap["code"].(string); code != "" {
				oldByCode[code] = regionMap
			}
		}
	}

	newRegionCodes := make(map[string]bool)
	result := make([]client.Region, 0, len(newRegions))

	for _, nr := range newRegions {
		newMap, ok := nr.(map[string]interface{})
		if !ok {
			continue
		}
		regionCode, _ := newMap["code"].(string)
		newRegionCodes[regionCode] = true

		var oldZones []interface{}
		if old, exists := oldByCode[regionCode]; exists {
			oldZones, _ = old["zones"].([]interface{})
		}
		newZonesRaw, _ := newMap["zones"].([]interface{})

		azuInfo := &client.AzureRegionCloudInfo{
			Vnet:            utils.GetStringPointer(newMap["vnet"].(string)),
			SecurityGroupId: utils.GetStringPointer(newMap["security_group_id"].(string)),
		}
		if v := providerutil.GetString(newMap, "resource_group"); v != "" {
			azuInfo.SetAzuRGOverride(v)
		}
		if v := providerutil.GetString(newMap, "network_resource_group"); v != "" {
			azuInfo.SetAzuNetworkRGOverride(v)
		}
		region := client.Region{
			Code:  utils.GetStringPointer(regionCode),
			Name:  utils.GetStringPointer(regionCode),
			Zones: mergeZoneUUIDs(oldZones, newZonesRaw),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{Azu: azuInfo},
			},
		}
		if old, exists := oldByCode[regionCode]; exists {
			if uuid, _ := old["uuid"].(string); uuid != "" {
				region.Uuid = utils.GetStringPointer(uuid)
			}
		}
		result = append(result, region)
	}

	// Deactivate removed regions; zones with subnets preserved for the Azure validator.
	for code, oldRegion := range oldByCode {
		if !newRegionCodes[code] {
			uuid, _ := oldRegion["uuid"].(string)
			oldZonesRaw, _ := oldRegion["zones"].([]interface{})
			azuInfo := &client.AzureRegionCloudInfo{
				Vnet: utils.GetStringPointer(providerutil.GetString(oldRegion, "vnet")),
				SecurityGroupId: utils.GetStringPointer(
					providerutil.GetString(oldRegion, "security_group_id"),
				),
			}
			if v := providerutil.GetString(oldRegion, "resource_group"); v != "" {
				azuInfo.SetAzuRGOverride(v)
			}
			if v := providerutil.GetString(oldRegion, "network_resource_group"); v != "" {
				azuInfo.SetAzuNetworkRGOverride(v)
			}
			region := client.Region{
				Code:   utils.GetStringPointer(code),
				Name:   utils.GetStringPointer(code),
				Active: utils.GetBoolPointer(false),
				Zones:  mergeZoneUUIDs(oldZonesRaw, oldZonesRaw), // preserve all zones as-is
				Details: &client.RegionDetails{
					CloudInfo: &client.RegionCloudInfo{Azu: azuInfo},
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

// flattenAzureRegions converts API regions to schema format
func flattenAzureRegions(regions []client.Region) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, region := range regions {
		details := region.GetDetails()
		cloudInfo := details.GetCloudInfo()
		azureInfo := cloudInfo.GetAzu()

		r := map[string]interface{}{
			"uuid":                   region.GetUuid(),
			"code":                   region.GetCode(),
			"name":                   region.GetCode(),
			"zones":                  flattenAzureZones(region.GetZones()),
			"vnet":                   azureInfo.GetVnet(),
			"security_group_id":      azureInfo.GetSecurityGroupId(),
			"resource_group":         azureInfo.GetAzuRGOverride(),
			"network_resource_group": azureInfo.GetAzuNetworkRGOverride(),
		}

		result = append(result, r)
	}

	return result
}

// flattenAzureZones converts API zones to schema format
func flattenAzureZones(zones []client.AvailabilityZone) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid":             zone.GetUuid(),
			"code":             zone.GetCode(),
			"name":             zone.GetCode(), // name is Computed and mirrors code
			"subnet":           zone.GetSubnet(),
			"secondary_subnet": zone.GetSecondarySubnet(),
		}
		result = append(result, z)
	}

	return result
}
