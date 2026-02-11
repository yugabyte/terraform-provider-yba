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

// buildAWSAccessKeys builds access keys for AWS provider
// Mirrors yba-cli access key construction
func buildAWSAccessKeys(d *schema.ResourceData) []client.AccessKey {
	keyPairName := d.Get("ssh_keypair_name").(string)
	sshContent := d.Get("ssh_private_key_content").(string)
	skipValidation := d.Get("skip_keypair_validation").(bool)

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
		regionName := regionMap["name"].(string)

		// Build zones for this region
		zones := buildAWSZones(regionMap["zones"].([]interface{}))

		region := client.Region{
			Code:  utils.GetStringPointer(regionName),
			Name:  utils.GetStringPointer(regionName),
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
		zoneName := zoneMap["name"].(string)

		zone := client.AvailabilityZone{
			Code:            utils.GetStringPointer(zoneName),
			Name:            zoneName,
			Subnet:          utils.GetStringPointer(zoneMap["subnet"].(string)),
			SecondarySubnet: utils.GetStringPointer(zoneMap["secondary_subnet"].(string)),
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
			"name":  region.GetCode(), // Use code as name for consistency
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
			Arch:    utils.GetStringPointer(detailsMap["arch"].(string)),
			SshUser: utils.GetStringPointer(detailsMap["ssh_user"].(string)),
			SshPort: utils.GetInt32Pointer(int32(detailsMap["ssh_port"].(int))),
		}

		// Note: use_imds_v2 is NOT set here - YBA enforces IMDSv2=true for
		// all AWS image bundles as a security requirement. The field is
		// computed/read-only in the schema.

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
		bundle := map[string]interface{}{
			"uuid":           ib.GetUuid(),
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
