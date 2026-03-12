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

func flattenImageBundleDetails(details client.ImageBundleDetails) []map[string]interface{} {
	d := map[string]interface{}{
		"arch":             details.GetArch(),
		"ssh_user":         details.GetSshUser(),
		"ssh_port":         details.GetSshPort(),
		"global_yb_image":  details.GetGlobalYbImage(),
		"region_overrides": flattenRegionOverrides(details.GetRegions()),
	}
	return []map[string]interface{}{d}
}

func flattenRegionOverrides(overrides map[string]client.BundleInfo) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range overrides {
		result[k] = v.GetYbImage()
	}
	return result
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
