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

package universe

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

func buildUniverse(d *schema.ResourceData) client.UniverseConfigureTaskParams {
	clusters := buildClusters(d.Get("clusters").([]interface{}))
	enableYbc := true
	return client.UniverseConfigureTaskParams{
		ClientRootCA: utils.GetStringPointer(d.Get("client_root_ca").(string)),
		Clusters:     clusters,
		CommunicationPorts: buildCommunicationPorts(
			utils.MapFromSingletonList(d.Get("communication_ports").([]interface{}))),
		EnableYbc: utils.GetBoolPointer(enableYbc),
	}
}

func buildUniverseDefinitionTaskParams(d *schema.ResourceData) client.UniverseDefinitionTaskParams {
	return client.UniverseDefinitionTaskParams{
		ClientRootCA: utils.GetStringPointer(d.Get("client_root_ca").(string)),
		Clusters:     buildClusters(d.Get("clusters").([]interface{})),
		CommunicationPorts: buildCommunicationPorts(
			utils.MapFromSingletonList(d.Get("communication_ports").([]interface{}))),
	}
}

func buildCommunicationPorts(cp map[string]interface{}) *client.CommunicationPorts {
	if len(cp) == 0 {
		return &client.CommunicationPorts{}
	}
	return &client.CommunicationPorts{
		MasterHttpPort:      utils.GetInt32Pointer(int32(cp["master_http_port"].(int))),
		MasterRpcPort:       utils.GetInt32Pointer(int32(cp["master_rpc_port"].(int))),
		NodeExporterPort:    utils.GetInt32Pointer(int32(cp["node_exporter_port"].(int))),
		RedisServerHttpPort: utils.GetInt32Pointer(int32(cp["redis_server_http_port"].(int))),
		RedisServerRpcPort:  utils.GetInt32Pointer(int32(cp["redis_server_rpc_port"].(int))),
		TserverHttpPort:     utils.GetInt32Pointer(int32(cp["tserver_http_port"].(int))),
		TserverRpcPort:      utils.GetInt32Pointer(int32(cp["tserver_rpc_port"].(int))),
		YqlServerHttpPort:   utils.GetInt32Pointer(int32(cp["yql_server_http_port"].(int))),
		YqlServerRpcPort:    utils.GetInt32Pointer(int32(cp["yql_server_rpc_port"].(int))),
		YsqlServerHttpPort:  utils.GetInt32Pointer(int32(cp["ysql_server_http_port"].(int))),
		YsqlServerRpcPort:   utils.GetInt32Pointer(int32(cp["ysql_server_rpc_port"].(int))),
	}
}

func buildClusters(clusters []interface{}) (res []client.Cluster) {
	for _, v := range clusters {
		cluster := v.(map[string]interface{})
		c := client.Cluster{
			ClusterType: cluster["cluster_type"].(string),
			UserIntent:  buildUserIntent(utils.MapFromSingletonList(cluster["user_intent"].([]interface{}))),
		}
		if len(cluster["cloud_list"].([]interface{})) > 0 {
			c.PlacementInfo = &client.PlacementInfo{
				CloudList: buildCloudList(cluster["cloud_list"].([]interface{})),
			}
		}

		res = append(res, c)
	}
	return res
}

func buildCloudList(cl []interface{}) (res []client.PlacementCloud) {
	for _, v := range cl {
		c := v.(map[string]interface{})
		pc := client.PlacementCloud{
			Code:       utils.GetStringPointer(c["code"].(string)),
			RegionList: buildRegionList(c["region_list"].([]interface{})),
		}
		res = append(res, pc)
	}
	return res
}

func buildRegionList(cl []interface{}) *[]client.PlacementRegion {
	var res []client.PlacementRegion
	for _, v := range cl {
		r := v.(map[string]interface{})
		pr := client.PlacementRegion{
			Code:   utils.GetStringPointer(r["code"].(string)),
			AzList: buildAzList(r["az_list"].([]interface{})),
		}
		res = append(res, pr)
	}
	return &res
}

func buildAzList(cl []interface{}) *[]client.PlacementAZ {
	var res []client.PlacementAZ
	for _, v := range cl {
		az := v.(map[string]interface{})
		paz := client.PlacementAZ{
			IsAffinitized:     utils.GetBoolPointer(az["is_affinitized"].(bool)),
			Name:              utils.GetStringPointer(az["name"].(string)),
			NumNodesInAZ:      utils.GetInt32Pointer(int32(az["num_nodes"].(int))),
			ReplicationFactor: utils.GetInt32Pointer(int32(az["replication_factor"].(int))),
			SecondarySubnet:   utils.GetStringPointer(az["secondary_subnet"].(string)),
			Subnet:            utils.GetStringPointer(az["subnet"].(string)),
		}
		res = append(res, paz)
	}
	return &res
}

func buildUserIntent(ui map[string]interface{}) client.UserIntent {
	return client.UserIntent{
		AssignStaticPublicIP:  utils.GetBoolPointer(ui["assign_static_ip"].(bool)),
		AwsArnString:          utils.GetStringPointer(ui["aws_arn_string"].(string)),
		EnableExposingService: utils.GetStringPointer(ui["enable_exposing_service"].(string)),
		EnableIPV6:            utils.GetBoolPointer(ui["enable_ipv6"].(bool)),
		EnableYCQL:            utils.GetBoolPointer(ui["enable_ycql"].(bool)),
		EnableYCQLAuth:        utils.GetBoolPointer(ui["enable_ycql_auth"].(bool)),
		EnableYSQLAuth:        utils.GetBoolPointer(ui["enable_ysql_auth"].(bool)),
		InstanceTags:          utils.StringMap(ui["instance_tags"].(map[string]interface{})),
		PreferredRegion:       utils.GetStringPointer(ui["preferred_region"].(string)),
		UseHostname:           utils.GetBoolPointer(ui["use_host_name"].(bool)),
		UseSystemd:            utils.GetBoolPointer(ui["use_systemd"].(bool)),
		YsqlPassword:          utils.GetStringPointer(ui["ysql_password"].(string)),
		YcqlPassword:          utils.GetStringPointer(ui["ycql_password"].(string)),
		UniverseName:          utils.GetStringPointer(ui["universe_name"].(string)),
		ProviderType:          utils.GetStringPointer(ui["provider_type"].(string)),
		Provider:              utils.GetStringPointer(ui["provider"].(string)),
		RegionList:            utils.StringSlice(ui["region_list"].([]interface{})),
		NumNodes:              utils.GetInt32Pointer(int32(ui["num_nodes"].(int))),
		ReplicationFactor:     utils.GetInt32Pointer(int32(ui["replication_factor"].(int))),
		InstanceType:          utils.GetStringPointer(ui["instance_type"].(string)),
		DeviceInfo: buildDeviceInfo(
			utils.MapFromSingletonList(ui["device_info"].([]interface{}))),
		AssignPublicIP:            utils.GetBoolPointer(ui["assign_public_ip"].(bool)),
		UseTimeSync:               utils.GetBoolPointer(ui["use_time_sync"].(bool)),
		EnableYSQL:                utils.GetBoolPointer(ui["enable_ysql"].(bool)),
		EnableYEDIS:               utils.GetBoolPointer(ui["enable_yedis"].(bool)),
		EnableNodeToNodeEncrypt:   utils.GetBoolPointer(ui["enable_node_to_node_encrypt"].(bool)),
		EnableClientToNodeEncrypt: utils.GetBoolPointer(ui["enable_client_to_node_encrypt"].(bool)),
		EnableVolumeEncryption:    utils.GetBoolPointer(ui["enable_volume_encryption"].(bool)),
		YbSoftwareVersion:         utils.GetStringPointer(ui["yb_software_version"].(string)),
		AccessKeyCode:             utils.GetStringPointer(ui["access_key_code"].(string)),
		TserverGFlags:             utils.StringMap(ui["tserver_gflags"].(map[string]interface{})),
		MasterGFlags:              utils.StringMap(ui["master_gflags"].(map[string]interface{})),
	}
}

func buildDeviceInfo(di map[string]interface{}) *client.DeviceInfo {
	return &client.DeviceInfo{
		DiskIops:     utils.GetInt32Pointer(int32(di["disk_iops"].(int))),
		MountPoints:  utils.GetStringPointer(di["mount_points"].(string)),
		StorageClass: utils.GetStringPointer(di["storage_class"].(string)),
		Throughput:   utils.GetInt32Pointer(int32(di["throughput"].(int))),
		NumVolumes:   utils.GetInt32Pointer(int32(di["num_volumes"].(int))),
		VolumeSize:   utils.GetInt32Pointer(int32(di["volume_size"].(int))),
		StorageType:  utils.GetStringPointer(di["storage_type"].(string)),
	}
}

func buildNodeDetailsRespArrayToNodeDetailsArray(nodes *[]client.NodeDetailsResp) (
	*[]client.NodeDetails) {
	var nodesDetails []client.NodeDetails
	for _, v := range *nodes {
		nodeDetail := client.NodeDetails{
			AzUuid:                v.AzUuid,
			CloudInfo:             v.CloudInfo,
			CronsActive:           v.CronsActive,
			DisksAreMountedByUUID: v.DisksAreMountedByUUID,
			IsMaster:              v.IsMaster,
			IsRedisServer:         v.IsRedisServer,
			IsTserver:             v.IsTserver,
			IsYqlServer:           v.IsYqlServer,
			IsYsqlServer:          v.IsYsqlServer,
			MachineImage:          v.MachineImage,
			MasterHttpPort:        v.MasterHttpPort,
			MasterRpcPort:         v.MasterRpcPort,
			MasterState:           v.MasterState,
			NodeExporterPort:      v.NodeExporterPort,
			NodeIdx:               v.NodeIdx,
			NodeName:              v.NodeName,
			NodeUuid:              v.NodeUuid,
			PlacementUuid:         v.PlacementUuid,
			RedisServerHttpPort:   v.RedisServerHttpPort,
			RedisServerRpcPort:    v.RedisServerRpcPort,
			State:                 v.State,
			TserverHttpPort:       v.TserverHttpPort,
			TserverRpcPort:        v.TserverRpcPort,
			YbPrebuiltAmi:         v.YbPrebuiltAmi,
			YqlServerHttpPort:     v.YqlServerHttpPort,
			YqlServerRpcPort:      v.YqlServerRpcPort,
			YsqlServerHttpPort:    v.YsqlServerHttpPort,
			YsqlServerRpcPort:     v.YsqlServerRpcPort,
		}
		nodesDetails = append(nodesDetails, nodeDetail)
	}
	return &nodesDetails
}
