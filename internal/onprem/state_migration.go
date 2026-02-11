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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// resourceOnPremProviderV0 returns the v0 schema for state migration reference.
// This represents the old nested schema structure with access_keys and details blocks.
func resourceOnPremProviderV0() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"code": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"version": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"instance_types": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"instance_type_key": {
							Type:     schema.TypeList,
							Required: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"instance_type_code": {Type: schema.TypeString, Required: true},
									"provider_uuid":      {Type: schema.TypeString, Computed: true},
								},
							},
						},
						"instance_type_details": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"volume_details_list": {
										Type:     schema.TypeList,
										Optional: true,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"mount_path": {
													Type:     schema.TypeString,
													Optional: true,
												},
												"volume_size_gb": {
													Type:     schema.TypeInt,
													Optional: true,
												},
												"volume_type": {
													Type:     schema.TypeString,
													Optional: true,
												},
												"storage_class": {
													Type:     schema.TypeString,
													Optional: true,
												},
												"storage_type": {
													Type:     schema.TypeString,
													Optional: true,
												},
												"throughput": {
													Type:     schema.TypeInt,
													Optional: true,
												},
												"iops": {
													Type:     schema.TypeInt,
													Optional: true,
												},
												"volume_size_key": {
													Type:     schema.TypeString,
													Computed: true,
												},
											},
										},
									},
								},
							},
						},
						"mem_size_gb":   {Type: schema.TypeFloat, Required: true},
						"num_cores":     {Type: schema.TypeFloat, Required: true},
						"provider_code": {Type: schema.TypeString, Computed: true},
					},
				},
			},
			"node_instances": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"ip":            {Type: schema.TypeString, Required: true},
						"region":        {Type: schema.TypeString, Required: true},
						"zone":          {Type: schema.TypeString, Required: true},
						"instance_type": {Type: schema.TypeString, Required: true},
						"instance_name": {Type: schema.TypeString, Optional: true},
						"node_name":     {Type: schema.TypeString, Computed: true},
						"in_use":        {Type: schema.TypeBool, Computed: true},
					},
				},
			},
			"regions": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid":      {Type: schema.TypeString, Computed: true},
						"code":      {Type: schema.TypeString, Computed: true},
						"name":      {Type: schema.TypeString, Required: true},
						"latitude":  {Type: schema.TypeFloat, Optional: true},
						"longitude": {Type: schema.TypeFloat, Optional: true},
						"zones": {
							Type:     schema.TypeList,
							Required: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {Type: schema.TypeString, Computed: true},
									"code": {Type: schema.TypeString, Computed: true},
									"name": {Type: schema.TypeString, Required: true},
								},
							},
						},
					},
				},
			},
			"access_keys": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"creation_date":   {Type: schema.TypeString, Computed: true},
						"expiration_date": {Type: schema.TypeString, Computed: true},
						"access_key_id": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"keycode":       {Type: schema.TypeString, Computed: true},
									"provider_uuid": {Type: schema.TypeString, Computed: true},
								},
							},
						},
						"key_info": {
							Type:     schema.TypeList,
							Required: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"key_pair_name": {
										Type:     schema.TypeString,
										Required: true,
									},
									"ssh_private_key_file_path": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"ssh_private_key_content": {
										Type:      schema.TypeString,
										Optional:  true,
										Sensitive: true,
									},
									"vault_file": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"vault_password_file": {
										Type:     schema.TypeString,
										Optional: true,
									},
									// Computed fields
									"air_gap_install": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"delete_remote": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"install_node_exporter": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"node_exporter_port": {
										Type:     schema.TypeInt,
										Computed: true,
									},
									"node_exporter_user": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"ntp_servers": {
										Type:     schema.TypeList,
										Elem:     &schema.Schema{Type: schema.TypeString},
										Computed: true,
									},
									"passwordless_sudo_access": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"private_key": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"provision_instance_script": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"public_key": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"skip_provisioning": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"ssh_port": {
										Type:     schema.TypeInt,
										Computed: true,
									},
									"ssh_user": {
										Type:     schema.TypeString,
										Computed: true,
									},
								},
							},
						},
					},
				},
			},
			"details": {
				Type:     schema.TypeList,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"air_gap_install": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"install_node_exporter": {
							Type:     schema.TypeBool,
							Optional: true,
							Computed: true,
						},
						"node_exporter_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"node_exporter_user": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"ntp_servers": {
							Type:     schema.TypeList,
							Elem:     &schema.Schema{Type: schema.TypeString},
							Optional: true,
							Computed: true,
						},
						"passwordless_sudo_access":  {Type: schema.TypeBool, Required: true},
						"provision_instance_script": {Type: schema.TypeString, Computed: true},
						"skip_provisioning":         {Type: schema.TypeBool, Required: true},
						"ssh_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"ssh_user": {Type: schema.TypeString, Required: true},
						"yb_home_dir": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

// upgradeOnPremProviderStateV0ToV1 migrates state from the old nested schema (v0)
// to the new flat schema (v1).
//
// Migration mapping:
//   - access_keys[0].key_info[0].key_pair_name -> ssh_keypair_name
//   - access_keys[0].key_info[0].ssh_private_key_content -> ssh_private_key_content
//   - details[0].ssh_user -> ssh_user
//   - details[0].ssh_port -> ssh_port
//   - details[0].skip_provisioning -> skip_provisioning
//   - details[0].passwordless_sudo_access -> passwordless_sudo_access
//   - details[0].air_gap_install -> air_gap_install
//   - details[0].install_node_exporter -> install_node_exporter
//   - details[0].node_exporter_user -> node_exporter_user
//   - details[0].node_exporter_port -> node_exporter_port
//   - details[0].ntp_servers -> ntp_servers
//   - details[0].yb_home_dir -> yb_home_dir
//   - instance_types (restructured from nested to flat)
//   - node_instances (region/zone renamed to region_name/zone_name)
func upgradeOnPremProviderStateV0ToV1(
	ctx context.Context,
	rawState map[string]interface{},
	meta interface{},
) (map[string]interface{}, error) {

	// Migrate access_keys to flat SSH fields
	if accessKeys, ok := rawState["access_keys"].([]interface{}); ok && len(accessKeys) > 0 {
		accessKey := accessKeys[0].(map[string]interface{})
		if keyInfoList, ok := accessKey["key_info"].([]interface{}); ok && len(keyInfoList) > 0 {
			keyInfo := keyInfoList[0].(map[string]interface{})

			// Migrate key pair name
			if keyPairName, ok := keyInfo["key_pair_name"].(string); ok && keyPairName != "" {
				rawState["ssh_keypair_name"] = keyPairName
			}

			// Migrate SSH private key content
			if sshPrivateKey, ok := keyInfo["ssh_private_key_content"].(string); ok &&
				sshPrivateKey != "" {
				rawState["ssh_private_key_content"] = sshPrivateKey
			}
		}
	}
	delete(rawState, "access_keys")

	// Migrate details block to flat fields
	if detailsList, ok := rawState["details"].([]interface{}); ok && len(detailsList) > 0 {
		details := detailsList[0].(map[string]interface{})

		// SSH settings
		if sshUser, ok := details["ssh_user"].(string); ok {
			rawState["ssh_user"] = sshUser
		}
		if sshPort, ok := details["ssh_port"]; ok {
			rawState["ssh_port"] = sshPort
		}

		// Provisioning settings
		if skipProvisioning, ok := details["skip_provisioning"].(bool); ok {
			rawState["skip_provisioning"] = skipProvisioning
		}
		if passwordlessSudo, ok := details["passwordless_sudo_access"].(bool); ok {
			rawState["passwordless_sudo_access"] = passwordlessSudo
		}
		if airGapInstall, ok := details["air_gap_install"].(bool); ok {
			rawState["air_gap_install"] = airGapInstall
		}
		if provisionScript, ok := details["provision_instance_script"].(string); ok {
			rawState["provision_instance_script"] = provisionScript
		}

		// Node exporter settings
		if installNodeExporter, ok := details["install_node_exporter"].(bool); ok {
			rawState["install_node_exporter"] = installNodeExporter
		}
		if nodeExporterUser, ok := details["node_exporter_user"].(string); ok {
			rawState["node_exporter_user"] = nodeExporterUser
		}
		if nodeExporterPort, ok := details["node_exporter_port"]; ok {
			rawState["node_exporter_port"] = nodeExporterPort
		}

		// NTP servers
		if ntpServers, ok := details["ntp_servers"]; ok {
			rawState["ntp_servers"] = ntpServers
		}

		// YB home directory
		if ybHomeDir, ok := details["yb_home_dir"].(string); ok {
			rawState["yb_home_dir"] = ybHomeDir
		}
	}
	delete(rawState, "details")

	// Migrate instance_types from nested to flat structure
	if instanceTypes, ok := rawState["instance_types"].([]interface{}); ok {
		newInstanceTypes := make([]interface{}, 0, len(instanceTypes))
		for _, it := range instanceTypes {
			oldIT := it.(map[string]interface{})
			newIT := make(map[string]interface{})

			// Extract instance_type_code from nested instance_type_key
			if keyList, ok := oldIT["instance_type_key"].([]interface{}); ok && len(keyList) > 0 {
				key := keyList[0].(map[string]interface{})
				if code, ok := key["instance_type_code"].(string); ok {
					newIT["instance_type_code"] = code
				}
			}

			// Copy num_cores and mem_size_gb
			if numCores, ok := oldIT["num_cores"]; ok {
				newIT["num_cores"] = numCores
			}
			if memSizeGB, ok := oldIT["mem_size_gb"]; ok {
				newIT["mem_size_gb"] = memSizeGB
			}

			// Extract volume details from nested structure
			if detailsList, ok := oldIT["instance_type_details"].([]interface{}); ok &&
				len(detailsList) > 0 {
				details := detailsList[0].(map[string]interface{})
				if volumeList, ok := details["volume_details_list"].([]interface{}); ok &&
					len(volumeList) > 0 {
					volume := volumeList[0].(map[string]interface{})
					if volumeSizeGB, ok := volume["volume_size_gb"]; ok {
						newIT["volume_size_gb"] = volumeSizeGB
					}
					if volumeType, ok := volume["volume_type"].(string); ok {
						newIT["volume_type"] = volumeType
					}
					if mountPath, ok := volume["mount_path"].(string); ok {
						newIT["mount_paths"] = mountPath
					}
				}
			}

			newInstanceTypes = append(newInstanceTypes, newIT)
		}
		rawState["instance_types"] = newInstanceTypes
	}

	// Migrate node_instances: rename region/zone to region_name/zone_name
	if nodeInstances, ok := rawState["node_instances"].([]interface{}); ok {
		newNodeInstances := make([]interface{}, 0, len(nodeInstances))
		for _, ni := range nodeInstances {
			oldNI := ni.(map[string]interface{})
			newNI := make(map[string]interface{})

			// Copy and rename fields
			if ip, ok := oldNI["ip"].(string); ok {
				newNI["ip"] = ip
			}
			if region, ok := oldNI["region"].(string); ok {
				newNI["region_name"] = region
			}
			if zone, ok := oldNI["zone"].(string); ok {
				newNI["zone_name"] = zone
			}
			if instanceType, ok := oldNI["instance_type"].(string); ok {
				newNI["instance_type"] = instanceType
			}
			if inUse, ok := oldNI["in_use"].(bool); ok {
				newNI["in_use"] = inUse
			}

			newNodeInstances = append(newNodeInstances, newNI)
		}
		rawState["node_instances"] = newNodeInstances
	}

	// Remove deprecated fields
	delete(rawState, "code")

	return rawState, nil
}
