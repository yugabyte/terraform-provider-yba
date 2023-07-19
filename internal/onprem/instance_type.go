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

package onprem

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// InstanceTypesSchema manages Instance type level information of on prem cloud provider
func InstanceTypesSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Describe the instance types for the provider.",
		Optional:    true,
		Type:        schema.TypeList,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"active": {
					Type:        schema.TypeBool,
					Computed:    true,
					Description: "True if instance type is Active.",
				},
				"instance_type_key": {
					Type:        schema.TypeList,
					Required:    true,
					MinItems:    1,
					MaxItems:    1,
					Description: "Instance Type Key.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"instance_type_code": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "Instance type code assigned by user.",
							},
							"provider_uuid": {
								Type:        schema.TypeString,
								Computed:    true,
								Description: "Provider UUID.",
							},
						},
					},
				},
				"provider_code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Provider code of instance type.",
				},
				"provider_uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Provider uuid of instance type.",
				},
				"instance_type_code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Instance type code assigned by user.",
				},
				"instance_type_details": {
					Type:        schema.TypeList,
					Required:    true,
					MaxItems:    1,
					Description: "Instance Type Key.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"tenancy": {
								Type:        schema.TypeString,
								Computed:    true,
								Optional:    true,
								Description: "Tenancy.",
							},
							"volume_details_list": {
								Type:        schema.TypeList,
								Required:    true,
								Description: "Details of Volumes attached.",
								Elem: &schema.Resource{
									Schema: map[string]*schema.Schema{
										"mount_path": {
											Type:        schema.TypeString,
											Required:    true,
											Description: "Mount Path, separated by commas.",
										},
										"volume_size_gb": {
											Type:        schema.TypeInt,
											Required:    true,
											Description: "Volume Size in GB attached to instance.",
										},
										"volume_type": {
											Type:        schema.TypeString,
											Optional:    true,
											Default:     "SSD",
											Description: "Volume Type attached to instance. SSD by default.",
										},
									},
								},
							},
						},
					},
				},
				"mem_size_gb": {
					Type:        schema.TypeFloat,
					Required:    true,
					Description: "Memory size in GB.",
				},
				"num_cores": {
					Type:        schema.TypeFloat,
					Required:    true,
					Description: "Number of cores per instance.",
				},
			},
		},
	}
}

func instanceTypesCreate(ctx context.Context, c *client.APIClient, cUUID, pUUID string,
	instanceTypeList []interface{}) ([]map[string]interface{}, error) {
	iTRL := make([]client.InstanceTypeResp, 0)
	// this is to ensure that the order of instance types in the state
	// file is the same as the one in the config file to avoid difference
	// during subsequent terraform apply commands
	instanceTypeCodeList := make([]string, 0)
	for _, i := range instanceTypeList {
		req := buildInstanceType(i, pUUID)
		instanceTypeCodeList = append(instanceTypeCodeList, req.IdKey.GetInstanceTypeCode())
		var response *http.Response
		r, response, err := c.InstanceTypesApi.CreateInstanceType(
			ctx, cUUID, pUUID).InstanceType(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Onprem Instance Types", "Create")
			return nil, errMessage
		}
		iTRL = append(iTRL, r)
	}
	instanceTypeRespList := flattenInstanceTypes(iTRL, instanceTypeCodeList)
	return instanceTypeRespList, nil
}

func instanceTypesRead(ctx context.Context, c *client.APIClient, cUUID, pUUID string) (
	[]client.InstanceTypeResp, error) {
	r, response, err := c.InstanceTypesApi.ListOfInstanceType(ctx, cUUID, pUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Onprem Instance Types", "Read")
		return nil, errMessage
	}
	return r, nil
}

func instanceTypeDelete(ctx context.Context, c *client.APIClient, cUUID, pUUID,
	instanceTypeCode string) error {
	_, response, err := c.InstanceTypesApi.DeleteInstanceType(ctx, cUUID, pUUID,
		instanceTypeCode).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Onprem Instance Types", "Delete")
		return errMessage
	}
	return nil
}

// buildInstanceType per block in the provider definition
// Only a single instance type create call can be made
func buildInstanceType(instanceType interface{}, pUUID string) client.InstanceType {
	if instanceType == nil {
		return client.InstanceType{}
	}
	i := instanceType.(map[string]interface{})
	iT := client.InstanceType{
		IdKey:               buildInstanceTypeKey(i["instance_type_key"].([]interface{}), pUUID),
		InstanceTypeDetails: buildInstanceTypeDetails(i["instance_type_details"].([]interface{})),
		MemSizeGB:           utils.GetFloat64Pointer(i["mem_size_gb"].(float64)),
		NumCores:            utils.GetFloat64Pointer(i["num_cores"].(float64)),
	}

	return iT
}

func buildInstanceTypeKey(id []interface{}, pUUID string) client.InstanceTypeKey {
	idKey := id[0].(map[string]interface{})
	return client.InstanceTypeKey{
		InstanceTypeCode: idKey["instance_type_code"].(string),
		ProviderUuid:     pUUID,
	}
}

func buildInstanceTypeDetails(details []interface{}) *client.InstanceTypeDetails {
	typeDetails := details[0].(map[string]interface{})
	return &client.InstanceTypeDetails{
		VolumeDetailsList: buildVolumeDetails(typeDetails["volume_details_list"].([]interface{})),
	}
}

func buildVolumeDetails(list []interface{}) []client.VolumeDetails {
	volumeDetailsList := make([]client.VolumeDetails, 0)
	for _, v := range list {
		details := v.(map[string]interface{})
		vD := client.VolumeDetails{
			MountPath:    details["mount_path"].(string),
			VolumeSizeGB: int32(details["volume_size_gb"].(int)),
			VolumeType:   "SSD",
		}
		volumeDetailsList = append(volumeDetailsList, vD)
	}
	return volumeDetailsList
}

func buildInstanceTypeFromInstanceTypeResp(instanceTypeList []client.InstanceTypeResp) (
	[]client.InstanceType) {
	instanceTypes := make([]client.InstanceType, 0)
	for _, i := range instanceTypeList {
		details := i.GetInstanceTypeDetails()
		instance := client.InstanceType{
			Active:              utils.GetBoolPointer(i.GetActive()),
			IdKey:               i.GetIdKey(),
			InstanceTypeCode:    utils.GetStringPointer(i.GetInstanceTypeCode()),
			InstanceTypeDetails: &details,
			MemSizeGB:           utils.GetFloat64Pointer(i.GetMemSizeGB()),
			NumCores:            utils.GetFloat64Pointer(i.GetNumCores()),
		}
		instanceTypes = append(instanceTypes, instance)
	}
	return instanceTypes
}

func flattenInstanceTypes(instanceTypeList []client.InstanceTypeResp, order []string) (
	res []map[string]interface{}) {
	orderLength := len(order)
	res = make([]map[string]interface{}, orderLength)
	for _, v := range instanceTypeList {
		instanceTypeKey := make([](map[string]interface{}), 0)
		iT := map[string]interface{}{
			"instance_type_code": v.GetInstanceTypeCode(),
			"provider_uuid":      v.GetProviderUuid(),
		}
		instanceTypeKey = append(instanceTypeKey, iT)
		i := map[string]interface{}{
			"active":                v.GetActive(),
			"instance_type_key":     instanceTypeKey,
			"instance_type_code":    v.GetInstanceTypeCode(),
			"instance_type_details": flattenInstanceTypeDetails(v.GetInstanceTypeDetails()),
			"mem_size_gb":           v.GetMemSizeGB(),
			"num_cores":             v.GetNumCores(),
			"provider_code":         v.GetProviderCode(),
			"provider_uuid":         v.GetProviderUuid(),
		}
		index := slices.Index(order, v.GetInstanceTypeCode())
		if index != -1 {
			res[index] = i
		} else {
			res = append(res, i)
		}
	}
	return res
}

func flattenInstanceTypeDetails(instanceTypeDetails client.InstanceTypeDetails) (
	res []map[string]interface{}) {
	i := map[string]interface{}{
		"tenancy":             instanceTypeDetails.GetTenancy(),
		"volume_details_list": flattenVolumeDetails(instanceTypeDetails.GetVolumeDetailsList()),
	}
	res = append(res, i)
	return res
}

func flattenVolumeDetails(volumeDetailsList []client.VolumeDetails) (res []map[string]interface{}) {
	for _, v := range volumeDetailsList {
		i := map[string]interface{}{
			"mount_path":     v.GetMountPath(),
			"volume_size_gb": v.GetVolumeSizeGB(),
			"volume_type":    v.GetVolumeType(),
		}
		res = append(res, i)
	}
	return res
}
