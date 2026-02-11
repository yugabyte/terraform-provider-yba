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
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// BuildAccessKeys builds access keys from schema
// Mirrors yba-cli pattern for access key construction
func BuildAccessKeys(accessKeys []interface{}) []client.AccessKey {
	if len(accessKeys) == 0 {
		return nil
	}

	result := make([]client.AccessKey, 0)
	for _, ak := range accessKeys {
		akMap := ak.(map[string]interface{})
		accessKey := client.AccessKey{
			KeyInfo: client.KeyInfo{
				KeyPairName: utils.GetStringPointer(akMap["key_pair_name"].(string)),
				SshPrivateKeyContent: utils.GetStringPointer(
					akMap["ssh_private_key_content"].(string),
				),
				SkipKeyValidateAndUpload: utils.GetBoolPointer(akMap["skip_key_validation"].(bool)),
			},
		}
		result = append(result, accessKey)
	}
	return result
}

// BuildZones builds zones from schema
// Mirrors yba-cli buildAWSZones/buildOnpremZones patterns
func BuildZones(zones []interface{}) []client.AvailabilityZone {
	result := make([]client.AvailabilityZone, 0)
	for _, z := range zones {
		zone := z.(map[string]interface{})
		az := client.AvailabilityZone{
			Code:            utils.GetStringPointer(zone["code"].(string)),
			Name:            zone["name"].(string),
			Subnet:          utils.GetStringPointer(zone["subnet"].(string)),
			SecondarySubnet: utils.GetStringPointer(zone["secondary_subnet"].(string)),
		}
		result = append(result, az)
	}
	return result
}

// BuildImageBundles builds image bundles from schema
// Mirrors yba-cli buildAWSImageBundles pattern
func BuildImageBundles(bundles []interface{}) []client.ImageBundle {
	result := make([]client.ImageBundle, 0)
	for _, b := range bundles {
		bundle := b.(map[string]interface{})
		ib := client.ImageBundle{
			Name:         utils.GetStringPointer(bundle["name"].(string)),
			UseAsDefault: utils.GetBoolPointer(bundle["use_as_default"].(bool)),
			Details:      buildImageBundleDetails(bundle["details"].([]interface{})),
		}
		result = append(result, ib)
	}
	return result
}

func buildImageBundleDetails(details []interface{}) *client.ImageBundleDetails {
	if len(details) == 0 {
		return nil
	}

	d := details[0].(map[string]interface{})
	result := &client.ImageBundleDetails{
		Arch:    utils.GetStringPointer(d["arch"].(string)),
		SshUser: utils.GetStringPointer(d["ssh_user"].(string)),
		SshPort: utils.GetInt32Pointer(int32(d["ssh_port"].(int))),
	}

	// Handle optional fields safely
	if v, ok := d["use_imds_v2"].(bool); ok {
		result.SetUseIMDSv2(v)
	}
	if v, ok := d["global_yb_image"].(string); ok && v != "" {
		result.SetGlobalYbImage(v)
	}
	if v, ok := d["region_overrides"].(map[string]interface{}); ok && len(v) > 0 {
		result.SetRegions(buildRegionOverrides(v))
	}

	return result
}

func buildRegionOverrides(overrides map[string]interface{}) map[string]client.BundleInfo {
	if overrides == nil {
		return nil
	}
	result := make(map[string]client.BundleInfo)
	for k, v := range overrides {
		if str, ok := v.(string); ok {
			result[k] = client.BundleInfo{
				YbImage: utils.GetStringPointer(str),
			}
		}
	}
	return result
}

// BuildProviderDetails builds common provider details
// Mirrors yba-cli ProviderDetails construction
func BuildProviderDetails(
	airGapInstall bool,
	ntpServers []string,
	setUpChrony bool,
	cloudInfo *client.CloudInfo,
) *client.ProviderDetails {
	return &client.ProviderDetails{
		AirGapInstall: utils.GetBoolPointer(airGapInstall),
		NtpServers:    ntpServers,
		SetUpChrony:   utils.GetBoolPointer(setUpChrony),
		CloudInfo:     cloudInfo,
	}
}

// GetNTPServers extracts NTP servers from schema
func GetNTPServers(d interface{}) []string {
	ntpServersInterface := d.([]interface{})
	ntpServers := make([]string, 0)
	for _, s := range ntpServersInterface {
		ntpServers = append(ntpServers, s.(string))
	}
	return ntpServers
}
