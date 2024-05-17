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
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func flattenCommunicationPorts(cp *client.CommunicationPorts) []interface{} {
	v := map[string]interface{}{
		"master_http_port":       cp.MasterHttpPort,
		"master_rpc_port":        cp.MasterRpcPort,
		"node_exporter_port":     cp.NodeExporterPort,
		"redis_server_http_port": cp.RedisServerHttpPort,
		"redis_server_rpc_port":  cp.RedisServerRpcPort,
		"tserver_http_port":      cp.TserverHttpPort,
		"tserver_rpc_port":       cp.TserverRpcPort,
		"yql_server_http_port":   cp.YqlServerHttpPort,
		"yql_server_rpc_port":    cp.YqlServerRpcPort,
		"ysql_server_http_port":  cp.YsqlServerHttpPort,
		"ysql_server_rpc_port":   cp.YsqlServerRpcPort,
	}
	return utils.CreateSingletonList(v)
}

func flattenClusters(clusters []client.Cluster) (res []map[string]interface{}) {
	for _, cluster := range clusters {
		c := map[string]interface{}{
			"uuid":         cluster.Uuid,
			"cluster_type": cluster.ClusterType,
			"user_intent":  flattenUserIntent(cluster.UserIntent),
			"cloud_list":   flattenCloudList(cluster.PlacementInfo.CloudList),
		}
		res = append(res, c)
	}
	return res
}

func flattenCloudList(cl []client.PlacementCloud) (res []interface{}) {
	for _, c := range cl {
		pc := map[string]interface{}{
			"uuid":        c.Uuid,
			"code":        c.Code,
			"region_list": flattenRegionList(*c.RegionList),
		}
		res = append(res, pc)
	}
	return res
}

func flattenRegionList(cl []client.PlacementRegion) (res []interface{}) {
	for _, r := range cl {
		pr := map[string]interface{}{
			"uuid":    r.Uuid,
			"code":    r.Code,
			"az_list": flattenAzList(*r.AzList),
		}
		res = append(res, pr)
	}
	return res
}

func flattenAzList(cl []client.PlacementAZ) (res []interface{}) {
	for _, az := range cl {
		paz := map[string]interface{}{
			"uuid":               az.Uuid,
			"is_affinitized":     az.IsAffinitized,
			"name":               az.Name,
			"num_nodes":          az.NumNodesInAZ,
			"replication_factor": az.ReplicationFactor,
			"secondary_subnet":   az.SecondarySubnet,
			"subnet":             az.Subnet,
		}
		res = append(res, paz)
	}
	return res
}

func flattenUserIntent(ui client.UserIntent) []interface{} {
	v := map[string]interface{}{
		"assign_static_ip":              ui.AssignStaticPublicIP,
		"aws_arn_string":                ui.AwsArnString,
		"enable_exposing_service":       ui.EnableExposingService,
		"enable_ipv6":                   ui.EnableIPV6,
		"enable_ycql":                   ui.EnableYCQL,
		"enable_ycql_auth":              ui.EnableYCQLAuth,
		"enable_ysql_auth":              ui.EnableYSQLAuth,
		"instance_tags":                 ui.GetInstanceTags(),
		"preferred_region":              ui.PreferredRegion,
		"use_host_name":                 ui.UseHostname,
		"use_systemd":                   ui.UseSystemd,
		"ysql_password":                 ui.YsqlPassword,
		"ycql_password":                 ui.YcqlPassword,
		"universe_name":                 ui.UniverseName,
		"provider_type":                 ui.ProviderType,
		"provider":                      ui.Provider,
		"region_list":                   ui.RegionList,
		"num_nodes":                     ui.NumNodes,
		"replication_factor":            ui.ReplicationFactor,
		"instance_type":                 ui.InstanceType,
		"device_info":                   flattenDeviceInfo(ui.DeviceInfo),
		"assign_public_ip":              ui.AssignPublicIP,
		"use_time_sync":                 ui.UseTimeSync,
		"enable_ysql":                   ui.EnableYSQL,
		"enable_yedis":                  ui.EnableYEDIS,
		"enable_node_to_node_encrypt":   ui.EnableNodeToNodeEncrypt,
		"enable_client_to_node_encrypt": ui.EnableClientToNodeEncrypt,
		"enable_volume_encryption":      ui.EnableVolumeEncryption,
		"yb_software_version":           ui.YbSoftwareVersion,
		"access_key_code":               ui.AccessKeyCode,
		"tserver_gflags":                ui.GetTserverGFlags(),
		"master_gflags":                 ui.GetMasterGFlags(),
	}
	return utils.CreateSingletonList(v)
}

func flattenDeviceInfo(di *client.DeviceInfo) []interface{} {
	v := map[string]interface{}{
		"disk_iops":     di.DiskIops,
		"mount_points":  di.MountPoints,
		"storage_class": di.StorageClass,
		"throughput":    di.Throughput,
		"num_volumes":   di.NumVolumes,
		"volume_size":   di.VolumeSize,
		"storage_type":  di.StorageType,
	}
	return utils.CreateSingletonList(v)
}

func flattenNodeDetailsSet(nsd []client.NodeDetailsResp) (res []interface{}) {
	for _, n := range nsd {
		i := map[string]interface{}{
			"az_uuid":                     n.AzUuid,
			"cloud_info":                  flattenCloudInfo(n.CloudInfo),
			"crons_active":                n.CronsActive,
			"dedicated_to":                n.DedicatedTo,
			"disks_are_mounted_by_uuid":   n.DisksAreMountedByUUID,
			"is_master":                   n.IsMaster,
			"is_redis_server":             n.IsRedisServer,
			"is_tserver":                  n.IsTserver,
			"is_yql_server":               n.IsYqlServer,
			"is_ysql_server":              n.IsYsqlServer,
			"kubernetes_overrides":        n.KubernetesOverrides,
			"last_volume_update_time":     n.LastVolumeUpdateTime,
			"machine_image":               n.MachineImage,
			"master_http_port":            n.MasterHttpPort,
			"master_rpc_port":             n.MasterRpcPort,
			"master_state":                n.MasterState,
			"node_exporter_port":          n.NodeExporterPort,
			"node_idx":                    n.NodeIdx,
			"node_name":                   n.NodeName,
			"node_uuid":                   n.NodeUuid,
			"otel_collector_metrics_port": n.OtelCollectorMetricsPort,
			"placement_uuid":              n.PlacementUuid,
			"redis_server_http_port":      n.RedisServerHttpPort,
			"redis_server_rpc_port":       n.RedisServerRpcPort,
			"ssh_port_override":           n.SshPortOverride,
			"ssh_user_override":           n.SshUserOverride,
			"state":                       n.State,
			"tserver_http_port":           n.TserverHttpPort,
			"tserver_rpc_port":            n.TserverRpcPort,
			"yb_controller_http_port":     n.YbControllerHttpPort,
			"yb_controller_rpc_port":      n.YbControllerRpcPort,
			"yb_prebuilt_ami":             n.YbPrebuiltAmi,
			"yql_server_http_port":        n.YqlServerHttpPort,
			"yql_server_rpc_port":         n.YqlServerRpcPort,
			"ysql_server_http_port":       n.YsqlServerHttpPort,
			"ysql_server_rpc_port":        n.YsqlServerRpcPort,
		}
		res = append(res, i)
	}
	return res
}

func flattenCloudInfo(ci *client.CloudSpecificInfo) []interface{} {
	v := map[string]interface{}{

		"assign_public_ip":     ci.AssignPublicIP,
		"az":                   ci.Az,
		"cloud":                ci.Cloud,
		"instance_type":        ci.InstanceType,
		"kubernetes_namespace": ci.KubernetesNamespace,
		"kubernetes_pod_name":  ci.KubernetesPodName,
		"lun_indexes":          ci.LunIndexes,
		"mount_roots":          ci.MountRoots,
		"private_dns":          ci.PrivateDns,
		"private_ip":           ci.PrivateIp,
		"public_dns":           ci.PublicDns,
		"public_ip":            ci.PublicIp,
		"region":               ci.Region,
		"root_volume":          ci.RootVolume,
		"secondary_private_ip": ci.SecondaryPrivateIp,
		"secondary_subnet_id":  ci.SecondarySubnetId,
		"subnet_id":            ci.SubnetId,
		"use_time_sync":        ci.UseTimeSync,
	}
	return utils.CreateSingletonList(v)
}
