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

package providerutil

import (
	client "github.com/yugabyte/platform-go-client"
)

// FlattenZones converts API zones to schema format
func FlattenZones(zones []client.AvailabilityZone) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid":             zone.Uuid,
			"code":             zone.Code,
			"name":             zone.Name,
			"subnet":           zone.Subnet,
			"secondary_subnet": zone.SecondarySubnet,
		}
		result = append(result, z)
	}
	return result
}

// FlattenImageBundles converts API image bundles to schema format
func FlattenImageBundles(bundles []client.ImageBundle) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, bundle := range bundles {
		metadata := bundle.GetMetadata()
		if metadata.GetType() == "YBA_ACTIVE" {
			continue // Skip YBA managed bundles
		}

		b := map[string]interface{}{
			"uuid":           bundle.GetUuid(),
			"name":           bundle.GetName(),
			"use_as_default": bundle.GetUseAsDefault(),
			"details":        flattenImageBundleDetails(bundle.GetDetails()),
		}
		result = append(result, b)
	}
	return result
}

// FlattenYBADefaultImageBundles converts YBA managed API image bundles to schema format
func FlattenYBADefaultImageBundles(bundles []client.ImageBundle) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, bundle := range bundles {
		metadata := bundle.GetMetadata()
		if metadata.GetType() != "YBA_ACTIVE" {
			continue // Skip custom bundles
		}

		details := bundle.GetDetails()
		b := map[string]interface{}{
			"uuid":           bundle.GetUuid(),
			"name":           bundle.GetName(),
			"arch":           details.GetArch(),
			"use_as_default": bundle.GetUseAsDefault(),
		}
		result = append(result, b)
	}
	return result
}

func flattenImageBundleDetails(details client.ImageBundleDetails) []map[string]interface{} {
	d := map[string]interface{}{
		"ssh_user":        details.GetSshUser(),
		"ssh_port":        details.GetSshPort(),
		"global_yb_image": details.GetGlobalYbImage(),
	}
	return []map[string]interface{}{d}
}

// FlattenAccessKeys converts API access keys to schema format
func FlattenAccessKeys(keys []client.AccessKey) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, key := range keys {
		keyInfo := key.GetKeyInfo()
		k := map[string]interface{}{
			"key_pair_name":           keyInfo.GetKeyPairName(),
			"ssh_private_key_content": keyInfo.GetSshPrivateKeyContent(),
			"skip_key_validation":     keyInfo.GetSkipKeyValidateAndUpload(),
		}
		result = append(result, k)
	}
	return result
}

// FlattenProviderDetails extracts common provider details fields
func FlattenProviderDetails(details client.ProviderDetails) map[string]interface{} {
	return map[string]interface{}{
		"air_gap_install": details.GetAirGapInstall(),
		"ntp_servers":     details.GetNtpServers(),
		"set_up_chrony":   details.GetSetUpChrony(),
	}
}

// AlignRegions aligns the order of the API regions to match the order in the configuration/state.
func AlignRegions(
	apiRegions []map[string]interface{},
	stateRegions []interface{},
) []map[string]interface{} {
	if len(stateRegions) == 0 {
		return apiRegions
	}

	apiMap := make(map[string]map[string]interface{})
	for _, r := range apiRegions {
		code, _ := r["code"].(string)
		apiMap[code] = r
	}

	result := make([]map[string]interface{}, 0, len(apiRegions))

	for _, sr := range stateRegions {
		stateMap, ok := sr.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := stateMap["code"].(string)

		if apiRegion, exists := apiMap[code]; exists {
			// Align zones
			if stateZonesIf, ok := stateMap["zones"].([]interface{}); ok {
				if apiZonesIf, ok := apiRegion["zones"].([]map[string]interface{}); ok {
					apiRegion["zones"] = AlignZones(apiZonesIf, stateZonesIf)
				}
			}

			result = append(result, apiRegion)
			delete(apiMap, code)
		}
	}

	// Append any remaining (new/unmatched) regions
	for _, r := range apiRegions {
		code, _ := r["code"].(string)
		if _, exists := apiMap[code]; exists {
			result = append(result, r)
		}
	}

	return result
}

// AlignZones aligns the order of zones to match the configuration/state.
func AlignZones(
	apiZones []map[string]interface{},
	stateZones []interface{},
) []map[string]interface{} {
	if len(stateZones) == 0 {
		return apiZones
	}

	apiMap := make(map[string]map[string]interface{})
	for _, z := range apiZones {
		code, _ := z["code"].(string)
		apiMap[code] = z
	}

	result := make([]map[string]interface{}, 0, len(apiZones))

	for _, sz := range stateZones {
		stateMap, ok := sz.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := stateMap["code"].(string)
		if apiZone, exists := apiMap[code]; exists {
			result = append(result, apiZone)
			delete(apiMap, code)
		}
	}

	// Append any remaining (new/unmatched) zones
	for _, z := range apiZones {
		code, _ := z["code"].(string)
		if _, exists := apiMap[code]; exists {
			result = append(result, z)
		}
	}

	return result
}

// AlignImageBundles aligns the order of image bundles to match the configuration/state.
func AlignImageBundles(
	apiBundles []map[string]interface{},
	stateBundles []interface{},
) []map[string]interface{} {
	if len(stateBundles) == 0 {
		return apiBundles
	}

	apiMap := make(map[string]map[string]interface{})
	for _, b := range apiBundles {
		name, _ := b["name"].(string)
		apiMap[name] = b
	}

	result := make([]map[string]interface{}, 0, len(apiBundles))

	for _, sb := range stateBundles {
		stateMap, ok := sb.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := stateMap["name"].(string)
		if apiBundle, exists := apiMap[name]; exists {
			result = append(result, apiBundle)
			delete(apiMap, name)
		}
	}

	// Append any remaining (new/unmatched) bundles
	for _, b := range apiBundles {
		name, _ := b["name"].(string)
		if _, exists := apiMap[name]; exists {
			result = append(result, b)
		}
	}

	return result
}

// AlignYBADefaultImageBundles aligns the order of YBA managed image bundles
// to match the configuration/state.
func AlignYBADefaultImageBundles(
	apiBundles []map[string]interface{},
	stateBundles []interface{},
) []map[string]interface{} {
	if len(stateBundles) == 0 {
		return apiBundles
	}

	apiMap := make(map[string]map[string]interface{})
	for _, b := range apiBundles {
		arch, _ := b["arch"].(string)
		apiMap[arch] = b
	}

	result := make([]map[string]interface{}, 0, len(apiBundles))

	for _, sb := range stateBundles {
		stateMap, ok := sb.(map[string]interface{})
		if !ok {
			continue
		}
		arch, _ := stateMap["arch"].(string)
		if apiBundle, exists := apiMap[arch]; exists {
			result = append(result, apiBundle)
			delete(apiMap, arch)
		}
	}

	// Append any remaining (new/unmatched) bundles
	for _, b := range apiBundles {
		arch, _ := b["arch"].(string)
		if _, exists := apiMap[arch]; exists {
			result = append(result, b)
		}
	}

	return result
}
