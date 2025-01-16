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

package cloud_provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ImageBundleSchema manages Image bundle level information of cloud providers
func ImageBundleSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Image bundles associated with cloud providers. " +
			"Supported from YugabyteDB Anywhere version: " + utils.YBAAllowImageBundlesMinVersion,
		Type:     schema.TypeList,
		Optional: true,
		Computed: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					ForceNew:    true,
					Description: "Image bundle UUID.",
				},
				"active": {
					Type:        schema.TypeBool,
					Computed:    true,
					ForceNew:    true,
					Description: "Is the image bundle active.",
				},
				"details": {
					Type:     schema.TypeList,
					Required: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"arch": {
								Type:        schema.TypeString,
								Required:    true,
								ForceNew:    true,
								Description: "Image bundle architecture.",
							},
							"global_yb_image": {
								Type:        schema.TypeString,
								Optional:    true,
								ForceNew:    true,
								Description: "Global YB image for the bundle.",
							},
							"region_overrides": {
								Type:     schema.TypeMap,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
								Description: "Region overrides for the bundle. " +
									"Provide region code as the key and override image as the value.",
							},
							"ssh_user": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "SSH user for the image.",
							},
							"ssh_port": {
								Type:        schema.TypeInt,
								Optional:    true,
								Default:     22,
								Description: "SSH port for the image. Default is 22.",
							},
						},
					},
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					ForceNew:    true,
					Description: "Name of the image bundle.",
				},
				"use_as_default": {
					Type:     schema.TypeBool,
					Optional: true,
					Description: "Flag indicating if the image bundle should be " +
						"used as default for this archietecture.",
				},
			},
		},
	}
}

func buildImageBundles(bundles []interface{}) (res []client.ImageBundle) {
	for _, b := range bundles {
		bundle := b.(map[string]interface{})
		r := client.ImageBundle{
			Details:      buildImageBundleDetails(bundle["details"].([]interface{})),
			Name:         utils.GetStringPointer(bundle["name"].(string)),
			UseAsDefault: utils.GetBoolPointer(bundle["use_as_default"].(bool)),
		}

		res = append(res, r)
	}
	return res
}

func buildImageBundleDetails(details []interface{}) *client.ImageBundleDetails {
	d := utils.MapFromSingletonList(details)
	res := client.ImageBundleDetails{
		Arch:          utils.GetStringPointer(d["arch"].(string)),
		GlobalYbImage: utils.GetStringPointer(d["global_yb_image"].(string)),
		Regions: buildImageBundleRegionOverrides(
			d["region_overrides"].(map[string]interface{}),
			d["ssh_user"].(string),
			int32(d["ssh_port"].(int))),
	}
	return &res
}

func buildImageBundleRegionOverrides(
	overrides map[string]interface{},
	sshUser string,
	sshPort int32,
) *map[string]client.BundleInfo {
	res := make(map[string]client.BundleInfo)
	for k, v := range overrides {
		res[k] = client.BundleInfo{
			YbImage:         utils.GetStringPointer(v.(string)),
			SshPortOverride: utils.GetInt32Pointer(sshPort),
			SshUserOverride: utils.GetStringPointer(sshUser),
		}
	}
	return &res
}

func flattenImageBundles(
	imageBundles []client.ImageBundle,
	imageBundlesInput []interface{}) []map[string]interface{} {
	res := make([]map[string]interface{}, 0)
	for _, bundle := range imageBundles {
		bundleName := bundle.GetName()
		bundleDetails := bundle.GetDetails()
		arch := bundleDetails.GetArch()
		sshUser := ""
		sshPort := int32(22)
		for _, b := range imageBundlesInput {
			bundle := b.(map[string]interface{})
			details := buildImageBundleDetails(bundle["details"].([]interface{}))
			name := bundle["name"].(string)
			if details.GetArch() == arch && name == bundleName {
				regions := details.GetRegions()
				for _, r := range regions {
					sshPort = r.GetSshPortOverride()
					sshUser = r.GetSshUserOverride()
				}
			}
		}
		r := map[string]interface{}{
			"details":        flattenImageBundleDetails(bundle.GetDetails(), sshUser, sshPort),
			"name":           bundle.GetName(),
			"use_as_default": bundle.GetUseAsDefault(),
			"uuid":           bundle.GetUuid(),
		}
		res = append(res, r)
	}

	return res
}

func flattenImageBundleDetails(
	details client.ImageBundleDetails,
	sshUser string, sshPort int32,
) []map[string]interface{} {
	res := make([]map[string]interface{}, 0)
	r := map[string]interface{}{
		"arch":             details.GetArch(),
		"global_yb_image":  details.GetGlobalYbImage(),
		"region_overrides": flattenImageBundleRegionOverrides(details.GetRegions()),
		"ssh_user":         sshUser,
		"ssh_port":         sshPort,
	}
	res = append(res, r)
	return res
}

func flattenImageBundleRegionOverrides(
	overrides map[string]client.BundleInfo,
) map[string]interface{} {
	res := make(map[string]interface{})
	for k, v := range overrides {
		res[k] = v.GetYbImage()
	}
	return res
}
