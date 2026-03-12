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

// Package onprem provides Terraform resource for On-Premises provider
// following patterns from yba-cli cmd/provider/onprem
package onprem

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// ResourceOnPremProvider creates and maintains On-Premises provider resource
// Following yba-cli pattern: yba provider onprem create/update/delete
func ResourceOnPremProvider() *schema.Resource {
	return &schema.Resource{
		Description: "On-Premises Provider Resource. " +
			"Use this resource to create and manage on-premises providers in YugabyteDB Anywhere. " +
			"To utilize the provider in universes, manage instance types and node instances " +
			"using instance_types and node_instances blocks.",

		CreateContext: resourceOnPremProviderCreate,
		ReadContext:   resourceOnPremProviderRead,
		UpdateContext: resourceOnPremProviderUpdate,
		DeleteContext: resourceOnPremProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: providerutil.DefaultTimeouts,

		// Schema version for state migration
		SchemaVersion: 1,
		StateUpgraders: []schema.StateUpgrader{
			{
				Version: 0,
				Type:    resourceOnPremProviderV0().CoreConfigSchema().ImpliedType(),
				Upgrade: upgradeOnPremProviderStateV0ToV1,
			},
		},

		Schema: onpremProviderSchema(),
	}
}

func onpremProviderSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"name": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "Name of the provider.",
		},
		"version": {
			Type:        schema.TypeInt,
			Computed:    true,
			Description: "Version of the provider.",
		},
		// SSH configuration (yba-cli: --ssh-user, --ssh-port, --ssh-keypair-name, etc.)
		"ssh_user": {
			Type:     schema.TypeString,
			Optional: true,
			Description: "SSH User to access YugabyteDB nodes. " +
				"Required when skip_provisioning is false.",
		},
		"ssh_port": {
			Type:        schema.TypeInt,
			Optional:    true,
			Default:     22,
			Description: "SSH port. Default is 22.",
		},
		"ssh_keypair_name": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "SSH key pair name to access YugabyteDB nodes.",
		},
		"ssh_private_key_content": {
			Type:         schema.TypeString,
			Optional:     true,
			Sensitive:    true,
			RequiredWith: []string{"ssh_keypair_name"},
			Description:  "SSH private key content to access YugabyteDB nodes.",
		},

		// Provisioning settings (yba-cli flags)
		"skip_provisioning": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
			Description: "Set to true if YugabyteDB nodes have been prepared manually. " +
				"Set to false to provision during universe creation. Default is false.",
		},
		"passwordless_sudo_access": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  true,
			Description: "Whether sudo actions can be carried out without a password. " +
				"Default is true.",
		},
		"air_gap_install": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
			Description: "Whether YugabyteDB nodes are installed in an air-gapped environment. " +
				"Default is false.",
		},
		"provision_instance_script": {
			Type:     schema.TypeString,
			Optional: true,
			Description: "Custom provisioning script path for node instances. " +
				"Used during universe creation if skip_provisioning is false.",
		},

		// Node exporter settings (yba-cli flags)
		"install_node_exporter": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     true,
			Description: "Whether to install Node Exporter. Default is true.",
		},
		"node_exporter_user": {
			Type:        schema.TypeString,
			Optional:    true,
			Default:     "prometheus",
			Description: "Node Exporter user. Default is 'prometheus'.",
		},
		"node_exporter_port": {
			Type:        schema.TypeInt,
			Optional:    true,
			Default:     9300,
			Description: "Node Exporter port. Default is 9300.",
		},

		// NTP/Chrony settings
		"ntp_servers": {
			Type:        schema.TypeList,
			Optional:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
			Description: "List of NTP servers. Can be provided as separate values.",
		},
		"set_up_chrony": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
			Description: "Set up NTP chrony service. When true, chrony will be configured " +
				"with the specified ntp_servers. For on-premises providers, ntp_servers must " +
				"be provided when set_up_chrony is true. When false, assumes NTP is " +
				"pre-configured in the machine image. Default is false.",
		},
		"show_set_up_chrony": {
			Type:     schema.TypeBool,
			Computed: true,
			Description: "Flag indicating whether to show the NTP setup option in the UI. " +
				"Read-only, set by YBA based on provider creation time.",
		},

		// Home directory
		"yb_home_dir": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "YugabyteDB home directory.",
		},

		// ClockBound support
		"use_clockbound": {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
			Description: "Use ClockBound for clock synchronization. " +
				"Requires ClockBound to be set up on the nodes. Default is false.",
		},

		// Common read-only fields
		"enable_node_agent": {
			Type:        schema.TypeBool,
			Computed:    true,
			Description: "Flag indicating if node agent is enabled for this provider. Read-only.",
		},
		"access_key_code": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Access key code for this provider. Read-only, generated by YBA.",
		},

		// Deprecated fields for backward compatibility
		"access_keys": deprecatedAccessKeysSchema(),
		"details":     deprecatedDetailsSchema(),

		// Regions (yba-cli: --region, --zone)
		"regions": onpremRegionsSchema(),

		// Instance types
		"instance_types": instanceTypesSchema(),

		// Node instances
		"node_instances": nodeInstancesSchema(),
	}
}

func onpremRegionsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "Regions for the on-premises provider.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Region UUID.",
				},
				"code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Region code.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Region name.",
				},
				"latitude": {
					Type:        schema.TypeFloat,
					Optional:    true,
					Default:     0.0,
					Description: "Latitude of the region. Default is 0.0.",
				},
				"longitude": {
					Type:        schema.TypeFloat,
					Optional:    true,
					Default:     0.0,
					Description: "Longitude of the region. Default is 0.0.",
				},
				"zones": onpremZonesSchema(),
			},
		},
	}
}

func onpremZonesSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "Zones for this region.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Zone UUID.",
				},
				"code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Zone code.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Zone name.",
				},
			},
		},
	}
}

func instanceTypesSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		Description: "Instance types for the on-premises provider.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"instance_type_code": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Instance type code (e.g., 'c5.large').",
				},
				"num_cores": {
					Type:        schema.TypeFloat,
					Required:    true,
					Description: "Number of CPU cores.",
				},
				"mem_size_gb": {
					Type:        schema.TypeFloat,
					Required:    true,
					Description: "Memory size in GB.",
				},
				"volume_size_gb": {
					Type:        schema.TypeInt,
					Optional:    true,
					Default:     100,
					Description: "Volume size in GB. Default is 100.",
				},
				"volume_type": {
					Type:        schema.TypeString,
					Optional:    true,
					Default:     "SSD",
					Description: "Volume type (e.g., EBS, SSD, HDD, NVME). Default is SSD.",
				},
				"mount_paths": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Comma-separated mount paths for volumes.",
				},
			},
		},
	}
}

func nodeInstancesSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		Description: "Node instances for the on-premises provider.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ip": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "IP address of the node.",
				},
				"zone_name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Zone name for this node.",
				},
				"region_name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Region name for this node.",
				},
				"instance_type": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Instance type for this node.",
				},
				"in_use": {
					Type:        schema.TypeBool,
					Computed:    true,
					Description: "Whether the node is currently in use.",
				},
			},
		},
	}
}

// deprecatedAccessKeysSchema returns the deprecated access_keys schema for backward compatibility.
func deprecatedAccessKeysSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Computed: true,
		MaxItems: 1,
		Deprecated: "The 'access_keys' block is deprecated. " +
			"Use 'ssh_keypair_name' and 'ssh_private_key_content' instead. " +
			"This field will be removed in a future version.",
		Description: "Deprecated: Access key configuration. " +
			"Use ssh_keypair_name and ssh_private_key_content instead.",
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
					Optional: true,
					Computed: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"key_pair_name": {
								Type:     schema.TypeString,
								Optional: true,
								Computed: true,
							},
							"ssh_private_key_file_path": {Type: schema.TypeString, Optional: true},
							"ssh_private_key_content": {
								Type:      schema.TypeString,
								Optional:  true,
								Sensitive: true,
							},
							"air_gap_install":          {Type: schema.TypeBool, Computed: true},
							"install_node_exporter":    {Type: schema.TypeBool, Computed: true},
							"node_exporter_port":       {Type: schema.TypeInt, Computed: true},
							"node_exporter_user":       {Type: schema.TypeString, Computed: true},
							"passwordless_sudo_access": {Type: schema.TypeBool, Computed: true},
							"skip_provisioning":        {Type: schema.TypeBool, Computed: true},
							"ssh_port":                 {Type: schema.TypeInt, Computed: true},
							"ssh_user":                 {Type: schema.TypeString, Computed: true},
						},
					},
				},
			},
		},
	}
}

// deprecatedDetailsSchema returns the deprecated details schema for backward compatibility.
func deprecatedDetailsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Computed: true,
		MaxItems: 1,
		Deprecated: "The 'details' block is deprecated. " +
			"Use the flat fields (ssh_user, ssh_port, skip_provisioning, passwordless_sudo_access, " +
			"air_gap_install, install_node_exporter, node_exporter_user, node_exporter_port, " +
			"ntp_servers, yb_home_dir) instead. This field will be removed in a future version.",
		Description: "Deprecated: Provider details. Use flat fields instead.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"air_gap_install":       {Type: schema.TypeBool, Optional: true, Computed: true},
				"install_node_exporter": {Type: schema.TypeBool, Optional: true, Computed: true},
				"node_exporter_port":    {Type: schema.TypeInt, Optional: true, Computed: true},
				"node_exporter_user":    {Type: schema.TypeString, Optional: true, Computed: true},
				"ntp_servers": {
					Type:     schema.TypeList,
					Elem:     &schema.Schema{Type: schema.TypeString},
					Optional: true,
					Computed: true,
				},
				"passwordless_sudo_access": {
					Type:     schema.TypeBool,
					Optional: true,
					Computed: true,
				},
				"provision_instance_script": {Type: schema.TypeString, Computed: true},
				"skip_provisioning": {
					Type:     schema.TypeBool,
					Optional: true,
					Computed: true,
				},
				"ssh_port": {Type: schema.TypeInt, Optional: true, Computed: true},
				"ssh_user": {
					Type:     schema.TypeString,
					Optional: true,
					Computed: true,
				},
				"yb_home_dir": {
					Type:     schema.TypeString,
					Optional: true,
					Computed: true,
				},
			},
		},
	}
}

// buildAccessKeys builds access keys for OnPrem provider.
// It first checks the new flat fields, and falls back to the deprecated access_keys block.
func buildAccessKeys(d *schema.ResourceData) []client.AccessKey {
	keyPairName := d.Get("ssh_keypair_name").(string)
	sshContent := d.Get("ssh_private_key_content").(string)

	// Fall back to deprecated access_keys block if new fields are empty
	if keyPairName == "" {
		if accessKeys, ok := d.GetOk("access_keys"); ok {
			accessKeysList := accessKeys.([]interface{})
			if len(accessKeysList) > 0 {
				accessKey := accessKeysList[0].(map[string]interface{})
				if keyInfoList, ok := accessKey["key_info"].([]interface{}); ok &&
					len(keyInfoList) > 0 {
					keyInfo := keyInfoList[0].(map[string]interface{})
					if kpn, ok := keyInfo["key_pair_name"].(string); ok && kpn != "" {
						keyPairName = kpn
					}
					if skc, ok := keyInfo["ssh_private_key_content"].(string); ok && skc != "" {
						sshContent = skc
					}
				}
			}
		}
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

// buildRegions builds OnPrem regions from schema
func buildRegions(regions []interface{}) []client.Region {
	result := make([]client.Region, 0)

	for _, r := range regions {
		regionMap := r.(map[string]interface{})
		regionName := regionMap["name"].(string)
		latitude := regionMap["latitude"].(float64)
		longitude := regionMap["longitude"].(float64)
		zones := buildZones(regionMap["zones"].([]interface{}))

		region := client.Region{
			Code:      utils.GetStringPointer(regionName),
			Name:      utils.GetStringPointer(regionName),
			Latitude:  utils.GetFloat64Pointer(latitude),
			Longitude: utils.GetFloat64Pointer(longitude),
			Zones:     zones,
		}
		result = append(result, region)
	}

	return result
}

// buildZones builds zones for a region
func buildZones(zones []interface{}) []client.AvailabilityZone {
	result := make([]client.AvailabilityZone, 0)

	for _, z := range zones {
		zoneMap := z.(map[string]interface{})
		zoneName := zoneMap["name"].(string)

		zone := client.AvailabilityZone{
			Code: utils.GetStringPointer(zoneName),
			Name: zoneName,
		}
		result = append(result, zone)
	}

	return result
}

// flattenRegions converts API regions to schema format
func flattenRegions(regions []client.Region) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, region := range regions {
		r := map[string]interface{}{
			"uuid":      region.GetUuid(),
			"code":      region.GetCode(),
			"name":      region.GetCode(),
			"latitude":  region.GetLatitude(),
			"longitude": region.GetLongitude(),
			"zones":     flattenZones(region.GetZones()),
		}
		result = append(result, r)
	}

	return result
}

// flattenZones converts API zones to schema format
func flattenZones(zones []client.AvailabilityZone) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid": zone.GetUuid(),
			"code": zone.GetCode(),
			"name": zone.GetName(),
		}
		result = append(result, z)
	}

	return result
}

// Instance type operations

func createInstanceTypes(ctx context.Context, c *client.APIClient, cUUID, pUUID string,
	instanceTypes []interface{}) error {
	for _, it := range instanceTypes {
		itMap := it.(map[string]interface{})
		typeCode := itMap["instance_type_code"].(string)

		mountPathsStr := ""
		if mp, ok := itMap["mount_paths"].(string); ok {
			mountPathsStr = mp
		}
		mountPaths := strings.Split(mountPathsStr, ",")
		volumeType := itMap["volume_type"].(string)
		volumeDetails := make([]client.VolumeDetails, 0)
		for _, path := range mountPaths {
			if path != "" {
				volumeDetails = append(volumeDetails, client.VolumeDetails{
					MountPath:    path,
					VolumeType:   volumeType,
					VolumeSizeGB: int32(itMap["volume_size_gb"].(int)),
				})
			}
		}

		req := client.InstanceType{
			IdKey: client.InstanceTypeKey{
				ProviderUuid:     pUUID,
				InstanceTypeCode: typeCode,
			},
			NumCores:  utils.GetFloat64Pointer(itMap["num_cores"].(float64)),
			MemSizeGB: utils.GetFloat64Pointer(itMap["mem_size_gb"].(float64)),
			InstanceTypeDetails: &client.InstanceTypeDetails{
				VolumeDetailsList: volumeDetails,
			},
		}

		_, response, err := c.InstanceTypesAPI.CreateInstanceType(ctx, cUUID, pUUID).
			InstanceType(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Instance Type", "Create")
			return errMessage
		}
		tflog.Info(ctx, fmt.Sprintf("Created instance type %s", typeCode))
	}
	return nil
}

func readInstanceTypes(ctx context.Context, c *client.APIClient, cUUID, pUUID string) (
	[]client.InstanceTypeResp, error) {
	instanceTypes, response, err := c.InstanceTypesAPI.ListOfInstanceType(ctx, cUUID, pUUID).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Instance Type", "Read")
		return nil, errMessage
	}
	return instanceTypes, nil
}

func flattenInstanceTypes(instanceTypes []client.InstanceTypeResp) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, it := range instanceTypes {
		mountPaths := make([]string, 0)
		details := it.GetInstanceTypeDetails()
		for _, vol := range details.GetVolumeDetailsList() {
			mountPaths = append(mountPaths, vol.GetMountPath())
		}

		m := map[string]interface{}{
			"instance_type_code": it.GetInstanceTypeCode(),
			"num_cores":          it.GetNumCores(),
			"mem_size_gb":        it.GetMemSizeGB(),
			"mount_paths":        strings.Join(mountPaths, ","),
		}

		if len(details.GetVolumeDetailsList()) > 0 {
			m["volume_size_gb"] = details.GetVolumeDetailsList()[0].GetVolumeSizeGB()
			m["volume_type"] = details.GetVolumeDetailsList()[0].GetVolumeType()
		}

		result = append(result, m)
	}

	return result
}

func updateInstanceTypes(ctx context.Context, c *client.APIClient, cUUID, pUUID string,
	instanceTypes []interface{}) error {
	existing, err := readInstanceTypes(ctx, c, cUUID, pUUID)
	if err != nil {
		return err
	}

	for _, it := range existing {
		typeCode := it.GetInstanceTypeCode()
		tflog.Info(ctx, fmt.Sprintf("Removing instance type %s from provider %s", typeCode, pUUID))
		_, response, err := c.InstanceTypesAPI.DeleteInstanceType(ctx, cUUID, pUUID, typeCode).
			Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Instance Type", "Delete")
			return errMessage
		}
	}

	if len(instanceTypes) > 0 {
		return createInstanceTypes(ctx, c, cUUID, pUUID, instanceTypes)
	}
	return nil
}

// Node instance operations

func createNodeInstances(ctx context.Context, c *client.APIClient, cUUID, pUUID string,
	nodeInstances []interface{}) error {
	nodesByZone := make(map[string][]client.NodeInstanceData)

	for _, ni := range nodeInstances {
		niMap := ni.(map[string]interface{})
		zoneKey := fmt.Sprintf("%s:%s", niMap["region_name"].(string), niMap["zone_name"].(string))

		nodeData := client.NodeInstanceData{
			Ip:           niMap["ip"].(string),
			InstanceType: niMap["instance_type"].(string),
			Region:       niMap["region_name"].(string),
			Zone:         niMap["zone_name"].(string),
		}
		nodesByZone[zoneKey] = append(nodesByZone[zoneKey], nodeData)
	}

	for zoneKey, nodes := range nodesByZone {
		parts := strings.Split(zoneKey, ":")
		if len(parts) != 2 {
			continue
		}

		zoneUUID, err := getZoneUUID(ctx, c, cUUID, pUUID, parts[0], parts[1])
		if err != nil {
			return err
		}

		req := client.NodeInstanceFormData{
			Nodes: nodes,
		}

		_, response, err := c.NodeInstancesAPI.CreateNodeInstance(ctx, cUUID, zoneUUID).
			NodeInstance(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Node Instance", "Create")
			return errMessage
		}
		tflog.Info(ctx, fmt.Sprintf("Created %d node instances in zone %s", len(nodes), zoneKey))
	}

	return nil
}

func readNodeInstances(ctx context.Context, c *client.APIClient, cUUID, pUUID string) (
	[]client.NodeInstance, error) {
	nodes, response, err := c.NodeInstancesAPI.ListByProvider(ctx, cUUID, pUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Node Instance", "Read")
		return nil, errMessage
	}
	return nodes, nil
}

func flattenNodeInstances(nodeInstances []client.NodeInstance) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, ni := range nodeInstances {
		details := ni.GetDetails()
		m := map[string]interface{}{
			"ip":            details.GetIp(),
			"zone_name":     details.GetZone(),
			"region_name":   details.GetRegion(),
			"instance_type": details.GetInstanceType(),
			"in_use":        ni.GetInUse(),
		}
		result = append(result, m)
	}

	return result
}

func updateNodeInstances(ctx context.Context, c *client.APIClient, cUUID, pUUID string,
	nodeInstances []interface{}) error {
	existing, err := readNodeInstances(ctx, c, cUUID, pUUID)
	if err != nil {
		return err
	}

	newNodeIPs := make([]string, 0)
	for _, ni := range nodeInstances {
		niMap := ni.(map[string]interface{})
		newNodeIPs = append(newNodeIPs, niMap["ip"].(string))
	}

	inUseNodes := make([]string, 0)
	for _, n := range existing {
		if n.GetInUse() {
			details := n.GetDetails()
			ip := details.GetIp()
			if !slices.Contains(newNodeIPs, ip) {
				inUseNodes = append(inUseNodes, ip)
			}
		}
	}
	if len(inUseNodes) > 0 {
		return fmt.Errorf("cannot remove in-use nodes: %v", inUseNodes)
	}

	for _, n := range existing {
		details := n.GetDetails()
		ip := details.GetIp()
		if !n.GetInUse() && !slices.Contains(newNodeIPs, ip) {
			tflog.Info(ctx, fmt.Sprintf("Removing node instance %s from provider %s", ip, pUUID))
			_, response, err := c.NodeInstancesAPI.DeleteInstance(ctx, cUUID, pUUID, ip).Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
					"Node Instance", "Delete")
				return errMessage
			}
		}
	}

	existingAfterDelete, err := readNodeInstances(ctx, c, cUUID, pUUID)
	if err != nil {
		return err
	}
	existingIPs := make([]string, 0)
	for _, n := range existingAfterDelete {
		details := n.GetDetails()
		existingIPs = append(existingIPs, details.GetIp())
	}

	nodesToCreate := make([]interface{}, 0)
	for _, ni := range nodeInstances {
		niMap := ni.(map[string]interface{})
		if !slices.Contains(existingIPs, niMap["ip"].(string)) {
			nodesToCreate = append(nodesToCreate, ni)
		}
	}

	if len(nodesToCreate) > 0 {
		return createNodeInstances(ctx, c, cUUID, pUUID, nodesToCreate)
	}

	return nil
}

func getZoneUUID(ctx context.Context, c *client.APIClient, cUUID, pUUID,
	regionName, zoneName string) (string, error) {
	providers, response, err := c.CloudProvidersAPI.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Provider", "Get")
		return "", errMessage
	}

	for _, p := range providers {
		if p.GetUuid() == pUUID {
			for _, r := range p.GetRegions() {
				if r.GetCode() == regionName || r.GetName() == regionName {
					for _, z := range r.GetZones() {
						if z.GetCode() == zoneName || z.GetName() == zoneName {
							return z.GetUuid(), nil
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("zone %s not found in region %s", zoneName, regionName)
}

// getValueWithDeprecatedFallback returns the value from the new flat field,
// or falls back to the deprecated details block if the new field is not set.
func getValueWithDeprecatedFallback(
	d *schema.ResourceData, newField, deprecatedField string,
) interface{} {
	if v, ok := d.GetOk(newField); ok {
		return v
	}

	if details, ok := d.GetOk("details"); ok {
		detailsList := details.([]interface{})
		if len(detailsList) > 0 {
			detailsMap := detailsList[0].(map[string]interface{})
			if v, ok := detailsMap[deprecatedField]; ok {
				return v
			}
		}
	}

	return d.Get(newField)
}

func resourceOnPremProviderCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c, cUUID := providerutil.GetAPIClient(meta)

	providerName := d.Get("name").(string)

	// Get values with fallback from deprecated details block
	sshUser := getValueWithDeprecatedFallback(d, "ssh_user", "ssh_user").(string)
	sshPort := getValueWithDeprecatedFallback(d, "ssh_port", "ssh_port").(int)
	skipProvisioning := getValueWithDeprecatedFallback(
		d, "skip_provisioning", "skip_provisioning").(bool)
	passwordlessSudo := getValueWithDeprecatedFallback(
		d, "passwordless_sudo_access", "passwordless_sudo_access").(bool)
	airGapInstall := getValueWithDeprecatedFallback(
		d, "air_gap_install", "air_gap_install").(bool)
	installNodeExporter := getValueWithDeprecatedFallback(
		d, "install_node_exporter", "install_node_exporter").(bool)
	nodeExporterUser := getValueWithDeprecatedFallback(
		d, "node_exporter_user", "node_exporter_user").(string)
	nodeExporterPort := getValueWithDeprecatedFallback(
		d, "node_exporter_port", "node_exporter_port").(int)
	ybHomeDir := getValueWithDeprecatedFallback(
		d, "yb_home_dir", "yb_home_dir").(string)

	var ntpServers []string
	if v, ok := d.GetOk("ntp_servers"); ok {
		ntpServers = providerutil.GetNTPServers(v)
	} else if details, ok := d.GetOk("details"); ok {
		detailsList := details.([]interface{})
		if len(detailsList) > 0 {
			detailsMap := detailsList[0].(map[string]interface{})
			if v, ok := detailsMap["ntp_servers"]; ok {
				ntpServers = providerutil.GetNTPServers(v)
			}
		}
	}

	if !skipProvisioning && sshUser == "" {
		return diag.FromErr(fmt.Errorf("ssh_user is required when skip_provisioning is false"))
	}

	accessKeys := buildAccessKeys(d)
	regions := buildRegions(d.Get("regions").([]interface{}))

	onpremCloudInfo := &client.OnPremCloudInfo{}
	if ybHomeDir != "" {
		onpremCloudInfo.SetYbHomeDir(ybHomeDir)
	}
	onpremCloudInfo.SetUseClockbound(d.Get("use_clockbound").(bool))

	req := client.Provider{
		Code:          utils.GetStringPointer(providerutil.OnPremProviderCode),
		Name:          utils.GetStringPointer(providerName),
		AllAccessKeys: accessKeys,
		Regions:       regions,
		Details: &client.ProviderDetails{
			AirGapInstall:          utils.GetBoolPointer(airGapInstall),
			SetUpChrony:            utils.GetBoolPointer(d.Get("set_up_chrony").(bool)),
			NtpServers:             ntpServers,
			SkipProvisioning:       utils.GetBoolPointer(skipProvisioning),
			PasswordlessSudoAccess: utils.GetBoolPointer(passwordlessSudo),
			ProvisionInstanceScript: utils.GetStringPointer(
				d.Get("provision_instance_script").(string),
			),
			InstallNodeExporter: utils.GetBoolPointer(installNodeExporter),
			NodeExporterUser:    utils.GetStringPointer(nodeExporterUser),
			NodeExporterPort:    utils.GetInt32Pointer(int32(nodeExporterPort)),
			SshUser:             utils.GetStringPointer(sshUser),
			SshPort:             utils.GetInt32Pointer(int32(sshPort)),
			CloudInfo: &client.CloudInfo{
				Onprem: onpremCloudInfo,
			},
		},
	}

	r, response, err := c.CloudProvidersAPI.CreateProviders(ctx, cUUID).
		CreateProviderRequest(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"OnPrem Provider", "Create")
		return diag.FromErr(errMessage)
	}

	pUUID := *r.ResourceUUID
	d.SetId(pUUID)

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "created",
			c, cUUID, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	if v := d.Get("instance_types"); v != nil && len(v.([]interface{})) > 0 {
		tflog.Info(ctx, fmt.Sprintf("Creating instance types for provider %s", pUUID))
		err := createInstanceTypes(ctx, c, cUUID, pUUID, v.([]interface{}))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	if v := d.Get("node_instances"); v != nil && len(v.([]interface{})) > 0 {
		tflog.Info(ctx, fmt.Sprintf("Adding node instances for provider %s", pUUID))
		err := createNodeInstances(ctx, c, cUUID, pUUID, v.([]interface{}))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceOnPremProviderRead(ctx, d, meta)
}

func resourceOnPremProviderRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics
	c, cUUID := providerutil.GetAPIClient(meta)

	pUUID := d.Id()

	p, err := providerutil.GetProvider(ctx, c, cUUID, pUUID)
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("name", p.GetName()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("version", p.GetVersion()); err != nil {
		return diag.FromErr(err)
	}

	details := p.GetDetails()
	if err = d.Set("air_gap_install", details.GetAirGapInstall()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ntp_servers", details.GetNtpServers()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("set_up_chrony", details.GetSetUpChrony()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("show_set_up_chrony", details.GetShowSetUpChrony()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("skip_provisioning", details.GetSkipProvisioning()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("passwordless_sudo_access", details.GetPasswordlessSudoAccess()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("provision_instance_script", details.GetProvisionInstanceScript()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("install_node_exporter", details.GetInstallNodeExporter()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("node_exporter_user", details.GetNodeExporterUser()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("node_exporter_port", details.GetNodeExporterPort()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_user", details.GetSshUser()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_port", details.GetSshPort()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("enable_node_agent", details.GetEnableNodeAgent()); err != nil {
		return diag.FromErr(err)
	}

	cloudInfo := details.GetCloudInfo()
	onpremInfo := cloudInfo.GetOnprem()
	if err = d.Set("yb_home_dir", onpremInfo.GetYbHomeDir()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("use_clockbound", onpremInfo.GetUseClockbound()); err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("regions", flattenRegions(p.GetRegions())); err != nil {
		return diag.FromErr(err)
	}

	instanceTypes, err := readInstanceTypes(ctx, c, cUUID, pUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("instance_types", flattenInstanceTypes(instanceTypes)); err != nil {
		return diag.FromErr(err)
	}

	if v := d.Get("node_instances"); v != nil && len(v.([]interface{})) > 0 {
		nodeInstances, err := readNodeInstances(ctx, c, cUUID, pUUID)
		if err != nil {
			return diag.FromErr(err)
		}
		if err = d.Set("node_instances", flattenNodeInstances(nodeInstances)); err != nil {
			return diag.FromErr(err)
		}
	}

	accessKeys := p.GetAllAccessKeys()
	if len(accessKeys) > 0 {
		keyInfo := accessKeys[0].GetKeyInfo()
		if err = d.Set("access_key_code", keyInfo.GetKeyPairName()); err != nil {
			return diag.FromErr(err)
		}
	}

	return diags
}

func resourceOnPremProviderUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c, cUUID := providerutil.GetAPIClient(meta)
	pUUID := d.Id()

	allowed, version, err := providerutil.ProviderYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}
	if !allowed {
		return diag.FromErr(fmt.Errorf(
			"editing OnPrem providers is not supported below version %s, currently on %s",
			utils.YBAAllowEditProviderMinVersion, version))
	}

	p, err := providerutil.GetProvider(ctx, c, cUUID, pUUID)
	if err != nil {
		return diag.FromErr(err)
	}

	providerReq := *p
	providerName := d.Get("name").(string)

	if d.HasChange("name") {
		providerReq.SetName(providerName)
	}

	if d.HasChange("regions") {
		providerReq.SetRegions(buildRegions(d.Get("regions").([]interface{})))
	}

	if d.HasChange("ssh_user") || d.HasChange("ssh_port") || d.HasChange("skip_provisioning") ||
		d.HasChange("passwordless_sudo_access") || d.HasChange("air_gap_install") ||
		d.HasChange("install_node_exporter") || d.HasChange("node_exporter_user") ||
		d.HasChange("node_exporter_port") || d.HasChange("ntp_servers") ||
		d.HasChange("set_up_chrony") || d.HasChange("provision_instance_script") ||
		d.HasChange("yb_home_dir") || d.HasChange("use_clockbound") {

		details := providerReq.GetDetails()
		details.SetSshUser(d.Get("ssh_user").(string))
		details.SetSshPort(int32(d.Get("ssh_port").(int)))
		details.SetSkipProvisioning(d.Get("skip_provisioning").(bool))
		details.SetPasswordlessSudoAccess(d.Get("passwordless_sudo_access").(bool))
		details.SetAirGapInstall(d.Get("air_gap_install").(bool))
		details.SetProvisionInstanceScript(d.Get("provision_instance_script").(string))
		details.SetInstallNodeExporter(d.Get("install_node_exporter").(bool))
		details.SetNodeExporterUser(d.Get("node_exporter_user").(string))
		details.SetNodeExporterPort(int32(d.Get("node_exporter_port").(int)))
		details.SetSetUpChrony(d.Get("set_up_chrony").(bool))
		details.SetNtpServers(providerutil.GetNTPServers(d.Get("ntp_servers")))

		cloudInfo := details.GetCloudInfo()
		onpremInfo := cloudInfo.GetOnprem()
		onpremInfo.SetYbHomeDir(d.Get("yb_home_dir").(string))
		onpremInfo.SetUseClockbound(d.Get("use_clockbound").(bool))
		cloudInfo.SetOnprem(onpremInfo)
		details.SetCloudInfo(cloudInfo)
		providerReq.SetDetails(details)
	}

	r, response, err := c.CloudProvidersAPI.EditProvider(ctx, cUUID, pUUID).
		EditProviderRequest(providerReq).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"OnPrem Provider", "Update")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "updated",
			c, cUUID, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	if d.HasChange("instance_types") {
		instanceTypes := d.Get("instance_types").([]interface{})
		if err := updateInstanceTypes(ctx, c, cUUID, pUUID, instanceTypes); err != nil {
			return diag.FromErr(err)
		}
	}

	if d.HasChange("node_instances") {
		nodeInstances := d.Get("node_instances").([]interface{})
		if err := updateNodeInstances(ctx, c, cUUID, pUUID, nodeInstances); err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceOnPremProviderRead(ctx, d, meta)
}

func resourceOnPremProviderDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics
	c, cUUID := providerutil.GetAPIClient(meta)

	providerName := d.Get("name").(string)

	r, response, err := c.CloudProvidersAPI.Delete(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"OnPrem Provider", "Delete")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "deleted",
			c, cUUID, d.Timeout(schema.TimeoutDelete))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId("")
	return diags
}

// Legacy helper functions for backwards compatibility with resource_onprem_node.go
// and data_source_node_filter.go

// NodeInstanceSchema returns the schema for node instances used by the data source.
func NodeInstanceSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		Description: "Node instances for the on-premises provider.",
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
	}
}

// findProvider finds a provider by UUID from a list of providers.
func findProvider(providers []client.Provider, uuid string) (*client.Provider, error) {
	for _, p := range providers {
		if *p.Uuid == uuid {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("could not find provider %s", uuid)
}

// nodeInstancesRead reads all node instances for a given provider.
func nodeInstancesRead(ctx context.Context, c *client.APIClient, cUUID, pUUID string) (
	[]client.NodeInstance, error) {
	return readNodeInstances(ctx, c, cUUID, pUUID)
}

// nodeInstancesCreate creates node instances for a provider using the legacy format.
func nodeInstancesCreate(ctx context.Context, c *client.APIClient, cUUID, pUUID string,
	nodeInstances []interface{}) ([]map[string]interface{}, error) {

	providers, response, err := c.CloudProvidersAPI.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Node Instance", "Get Provider")
		return nil, errMessage
	}

	var provider *client.Provider
	for _, p := range providers {
		if p.GetUuid() == pUUID {
			provider = &p
			break
		}
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %s not found", pUUID)
	}

	nodesByZone := make(map[string][]client.NodeInstanceData)
	zoneUUIDs := make(map[string]string)

	for _, ni := range nodeInstances {
		niMap := ni.(map[string]interface{})
		regionName := niMap["region"].(string)
		zoneName := niMap["zone"].(string)
		zoneKey := fmt.Sprintf("%s:%s", regionName, zoneName)

		for _, r := range provider.GetRegions() {
			if r.GetCode() == regionName || r.GetName() == regionName {
				for _, z := range r.GetZones() {
					if z.GetCode() == zoneName || z.GetName() == zoneName {
						zoneUUIDs[zoneKey] = z.GetUuid()
						break
					}
				}
			}
		}

		nodeData := client.NodeInstanceData{
			Ip:           niMap["ip"].(string),
			InstanceType: niMap["instance_type"].(string),
			Region:       regionName,
			Zone:         zoneName,
		}
		if instanceName, ok := niMap["instance_name"].(string); ok && instanceName != "" {
			nodeData.InstanceName = instanceName
		}
		if sshUser, ok := niMap["ssh_user"].(string); ok && sshUser != "" {
			nodeData.SshUser = sshUser
		}

		nodesByZone[zoneKey] = append(nodesByZone[zoneKey], nodeData)
	}

	result := make([]map[string]interface{}, 0)

	for zoneKey, nodes := range nodesByZone {
		zoneUUID, ok := zoneUUIDs[zoneKey]
		if !ok {
			return nil, fmt.Errorf("zone %s not found", zoneKey)
		}

		req := client.NodeInstanceFormData{
			Nodes: nodes,
		}

		resp, response, err := c.NodeInstancesAPI.CreateNodeInstance(ctx, cUUID, zoneUUID).
			NodeInstance(req).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Node Instance", "Create")
			return nil, errMessage
		}

		if resp != nil {
			for k, v := range *resp {
				nodeResult := map[string]interface{}{
					"ip":        k,
					"node_uuid": v.GetNodeUuid(),
				}
				result = append(result, nodeResult)
			}
		}
		tflog.Info(ctx, fmt.Sprintf("Created %d node instances in zone %s", len(nodes), zoneKey))
	}

	return result, nil
}

// nodeInstanceGet retrieves a single node instance by UUID.
func nodeInstanceGet(ctx context.Context, c *client.APIClient, cUUID, nUUID string) (
	*client.NodeInstance, error) {
	node, response, err := c.NodeInstancesAPI.GetNodeInstance(ctx, cUUID, nUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Node Instance", "Get")
		return nil, errMessage
	}
	return node, nil
}

// nodeInstanceDelete deletes a node instance by IP address.
func nodeInstanceDelete(ctx context.Context, c *client.APIClient, cUUID, pUUID, ip string) error {
	_, response, err := c.NodeInstancesAPI.DeleteInstance(ctx, cUUID, pUUID, ip).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Node Instance", "Delete")
		return errMessage
	}
	return nil
}

// flattenNodeInstancesLegacy converts API node instances to the legacy schema format.
func flattenNodeInstancesLegacy(nodeInstances []client.NodeInstance,
	orderList []string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, ni := range nodeInstances {
		details := ni.GetDetails()
		m := map[string]interface{}{
			"ip":            details.GetIp(),
			"region":        details.GetRegion(),
			"zone":          details.GetZone(),
			"instance_type": details.GetInstanceType(),
			"instance_name": ni.GetInstanceName(),
			"node_name":     ni.GetNodeName(),
			"in_use":        ni.GetInUse(),
		}
		result = append(result, m)
	}

	return result
}
