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
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
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
		"yb_controller_rpc_port": cp.YbControllerrRpcPort,
	}
	return utils.CreateSingletonList(v)
}

func flattenClusters(clusters []client.Cluster) (res []map[string]interface{}) {
	for _, cluster := range clusters {
		var cloudList []client.PlacementCloud
		if cluster.PlacementInfo != nil {
			cloudList = cluster.PlacementInfo.CloudList
		}
		c := map[string]interface{}{
			"uuid":         cluster.GetUuid(),
			"cluster_type": cluster.ClusterType,
			"user_intent":  flattenUserIntent(cluster.UserIntent),
			"cloud_list":   flattenCloudList(cloudList),
		}
		res = append(res, c)
	}
	return res
}

// restoreRedactedPasswords replaces "REDACTED" password values in freshly
// flattened clusters with the values held in the prior Terraform state.
// YBA never returns plaintext passwords on read; it returns "REDACTED"
// instead. Without this step every refresh would produce a spurious diff.
//
// Matching strategy:
//   - UUID-based: used on normal refresh where old state already has UUIDs.
//   - Index-based fallback: used on the initial Create->Read where the config
//     clusters have no UUIDs yet (they are assigned by YBA during creation).
func restoreRedactedPasswords(
	ctx context.Context,
	newClusters []map[string]interface{},
	oldClusters []interface{},
) {
	const redacted = "REDACTED"

	oldByUUID := make(map[string]map[string]interface{}, len(oldClusters))
	for _, oc := range oldClusters {
		ocMap, ok := oc.(map[string]interface{})
		if !ok {
			continue
		}
		uuid, _ := ocMap["uuid"].(string)
		if uuid != "" {
			oldByUUID[uuid] = ocMap
		}
	}

	for i, nc := range newClusters {
		uuid, _ := nc["uuid"].(string)

		var oldCluster map[string]interface{}

		// Prefer UUID-based match (stable across reorders).
		if uuid != "" {
			oldCluster = oldByUUID[uuid]
		}

		// Fall back to positional match when old clusters have no UUIDs,
		// which happens during the Create->Read call before state is written.
		if oldCluster == nil && i < len(oldClusters) {
			oldCluster, _ = oldClusters[i].(map[string]interface{})
			tflog.Debug(ctx, "restoreRedactedPasswords: using index-based match",
				map[string]interface{}{"index": i, "uuid": uuid})
		}

		if oldCluster == nil {
			tflog.Debug(ctx, "restoreRedactedPasswords: no old cluster found, skipping",
				map[string]interface{}{"index": i, "uuid": uuid})
			continue
		}

		oldUIList, ok := oldCluster["user_intent"].([]interface{})
		if !ok || len(oldUIList) == 0 {
			continue
		}
		oldUIMap, ok := oldUIList[0].(map[string]interface{})
		if !ok {
			continue
		}

		newUIList, ok := nc["user_intent"].([]interface{})
		if !ok || len(newUIList) == 0 {
			continue
		}
		newUIMap, ok := newUIList[0].(map[string]interface{})
		if !ok {
			continue
		}

		for _, field := range []string{"ysql_password", "ycql_password"} {
			p, ok := newUIMap[field].(*string)
			if ok && p != nil && *p == redacted {
				oldVal, _ := oldUIMap[field].(string)
				tflog.Debug(ctx, "restoreRedactedPasswords: restoring redacted field",
					map[string]interface{}{
						"index":         i,
						"uuid":          uuid,
						"field":         field,
						"has_old_value": oldVal != "",
					})
				newUIMap[field] = oldVal
			}
		}
	}
}

// flattenCloudList converts the API placement cloud list to schema format,
// aligning region and AZ order to match the prior state so that TypeList
// index-based comparisons stay stable across reads.
func flattenCloudList(cl []client.PlacementCloud) (res []interface{}) {
	for _, c := range cl {
		pc := map[string]interface{}{
			"provider":    c.Uuid,
			"code":        c.Code,
			"region_list": flattenRegionList(c.RegionList),
		}
		res = append(res, pc)
	}
	return res
}

func flattenRegionList(cl []client.PlacementRegion) (res []interface{}) {
	for _, r := range cl {
		// The placement API does not always populate the region Name field
		// (it can be absent for regions being edited). Fall back to Code so
		// that "terraform plan" never shows name = null in the diff.
		name := r.GetName()
		if name == "" {
			name = r.GetCode()
		}
		pr := map[string]interface{}{
			"uuid":    r.GetUuid(),
			"code":    r.GetCode(),
			"name":    name,
			"az_list": flattenAzList(r.AzList),
		}
		res = append(res, pr)
	}
	return res
}

func flattenAzList(cl []client.PlacementAZ) (res []interface{}) {
	for _, az := range cl {
		paz := map[string]interface{}{
			"uuid":               az.GetUuid(),
			"code":               az.GetName(),
			"is_affinitized":     az.GetIsAffinitized(),
			"leader_preference":  az.GetLeaderPreference(),
			"num_nodes":          az.GetNumNodesInAZ(),
			"replication_factor": az.GetReplicationFactor(),
			"secondary_subnet":   az.GetSecondarySubnet(),
			"subnet":             az.GetSubnet(),
		}
		res = append(res, paz)
	}
	return res
}

// alignCloudList reorders the API cloud list to match the order of regions and
// AZs recorded in the prior state (stateCloudList). Any API entries not present
// in state are appended at the end. This mirrors the AlignRegions/AlignZones
// pattern used in the AWS/GCP/on-prem provider resources and prevents spurious
// TypeList index-shift diffs after every read.
func alignCloudList(
	apiCloudList []interface{},
	stateCloudList []interface{},
) []interface{} {
	if len(stateCloudList) == 0 {
		return apiCloudList
	}

	// Index API clouds by code for O(1) lookup.
	apiByCode := make(map[string]map[string]interface{}, len(apiCloudList))
	for _, c := range apiCloudList {
		cm := c.(map[string]interface{})
		code, _ := cm["code"].(string)
		if code != "" {
			apiByCode[code] = cm
		}
	}

	used := make(map[string]bool)
	result := make([]interface{}, 0, len(apiCloudList))

	for _, sc := range stateCloudList {
		scm := sc.(map[string]interface{})
		code, _ := scm["code"].(string)
		apiCloud, ok := apiByCode[code]
		if !ok {
			continue
		}
		used[code] = true

		// Align region_list within this cloud.
		stateRegions, _ := scm["region_list"].([]interface{})
		apiRegions, _ := apiCloud["region_list"].([]interface{})
		apiCloud["region_list"] = alignRegionList(apiRegions, stateRegions)
		result = append(result, apiCloud)
	}

	// Append any API clouds not present in state (newly added).
	for _, c := range apiCloudList {
		cm := c.(map[string]interface{})
		code, _ := cm["code"].(string)
		if !used[code] {
			result = append(result, cm)
		}
	}
	return result
}

func alignRegionList(
	apiRegions []interface{},
	stateRegions []interface{},
) []interface{} {
	if len(stateRegions) == 0 {
		return apiRegions
	}

	apiByCode := make(map[string]map[string]interface{}, len(apiRegions))
	for _, r := range apiRegions {
		rm := r.(map[string]interface{})
		code, _ := rm["code"].(string)
		if code != "" {
			apiByCode[code] = rm
		}
	}

	used := make(map[string]bool)
	result := make([]interface{}, 0, len(apiRegions))

	for _, sr := range stateRegions {
		srm := sr.(map[string]interface{})
		code, _ := srm["code"].(string)
		apiRegion, ok := apiByCode[code]
		if !ok {
			continue
		}
		used[code] = true

		// Align az_list within this region.
		stateAZs, _ := srm["az_list"].([]interface{})
		apiAZs, _ := apiRegion["az_list"].([]interface{})
		apiRegion["az_list"] = alignAZList(apiAZs, stateAZs)
		result = append(result, apiRegion)
	}

	for _, r := range apiRegions {
		rm := r.(map[string]interface{})
		code, _ := rm["code"].(string)
		if !used[code] {
			result = append(result, rm)
		}
	}
	return result
}

func alignAZList(
	apiAZs []interface{},
	stateAZs []interface{},
) []interface{} {
	if len(stateAZs) == 0 {
		return apiAZs
	}

	apiByCode := make(map[string]map[string]interface{}, len(apiAZs))
	for _, a := range apiAZs {
		am := a.(map[string]interface{})
		code, _ := am["code"].(string)
		if code != "" {
			apiByCode[code] = am
		}
	}

	used := make(map[string]bool)
	result := make([]interface{}, 0, len(apiAZs))

	for _, sa := range stateAZs {
		sam := sa.(map[string]interface{})
		code, _ := sam["code"].(string)
		apiAZ, ok := apiByCode[code]
		if !ok {
			continue
		}
		used[code] = true
		result = append(result, apiAZ)
	}

	for _, a := range apiAZs {
		am := a.(map[string]interface{})
		code, _ := am["code"].(string)
		if !used[code] {
			result = append(result, am)
		}
	}
	return result
}

func flattenUserIntent(ui client.UserIntent) []interface{} {
	v := map[string]interface{}{
		"assign_static_ip":              ui.AssignStaticPublicIP,
		"aws_arn_string":                ui.AwsArnString,
		"enable_ipv6":                   ui.EnableIPV6,
		"enable_ycql":                   ui.EnableYCQL,
		"enable_ycql_auth":              ui.EnableYCQLAuth,
		"enable_ysql_auth":              ui.EnableYSQLAuth,
		"image_bundle_uuid":             ui.GetImageBundleUUID(),
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
		"yb_software_version":           ui.YbSoftwareVersion,
		"access_key_code":               ui.AccessKeyCode,
		"tserver_gflags":                ui.GetTserverGFlags(),
		"master_gflags":                 ui.GetMasterGFlags(),
		"dedicated_masters":             flattenDedicatedMasters(ui),
	}
	return utils.CreateSingletonList(v)
}

// flattenDedicatedMasters converts dedicated-master fields from the API UserIntent
// into the dedicated_masters schema block.
//
// Returns an empty list when DedicatedNodes is false (block absent in config).
// When DedicatedNodes is true the block is always present. instance_type and
// device_info are suppressed (set to their zero values) when they match the
// TServer values, which is the case when the user wrote "dedicated_masters {}"
// and the API applied the TServer fallback. This keeps the state consistent
// with an empty block so no spurious diff appears on subsequent plans.
func flattenDedicatedMasters(ui client.UserIntent) []interface{} {
	if !ui.GetDedicatedNodes() {
		return []interface{}{}
	}
	// Suppress instance_type when it equals the TServer instance type (fallback).
	masterIT := ui.GetMasterInstanceType()
	if masterIT == ui.GetInstanceType() {
		masterIT = ""
	}
	// Suppress device_info when it equals the TServer device info (fallback).
	var masterDI []interface{}
	if ui.MasterDeviceInfo != nil && !deviceInfoEqual(ui.MasterDeviceInfo, ui.DeviceInfo) {
		masterDI = flattenDeviceInfo(ui.MasterDeviceInfo)
	} else {
		masterDI = []interface{}{}
	}
	dm := map[string]interface{}{
		"instance_type": masterIT,
		"device_info":   masterDI,
	}
	return []interface{}{dm}
}

// deviceInfoEqual returns true when both DeviceInfo pointers represent the
// same disk configuration. Used to detect whether masterDeviceInfo is the
// TServer fallback (identical) or an explicit user override.
func deviceInfoEqual(a, b *client.DeviceInfo) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.GetNumVolumes() == b.GetNumVolumes() &&
		a.GetVolumeSize() == b.GetVolumeSize() &&
		a.GetStorageType() == b.GetStorageType() &&
		a.GetDiskIops() == b.GetDiskIops() &&
		a.GetThroughput() == b.GetThroughput() &&
		a.GetMountPoints() == b.GetMountPoints()
}

func flattenDeviceInfo(di *client.DeviceInfo) []interface{} {
	v := map[string]interface{}{
		"disk_iops":    di.DiskIops,
		"mount_points": di.MountPoints,
		"throughput":   di.Throughput,
		"num_volumes":  di.NumVolumes,
		"volume_size":  di.VolumeSize,
		"storage_type": di.StorageType,
	}
	return utils.CreateSingletonList(v)
}

// restoreDedicatedMasterDeviceInfo corrects a suppression edge case: when the
// user explicitly wrote a dedicated_masters.device_info block whose values happen
// to equal the TServer device_info, flattenDedicatedMasters suppresses it to []
// (because it cannot distinguish "user omitted it" from "user wrote identical
// values"). That produces a permanent plan diff: config has device_info, state
// has [].
//
// This function restores the actual API-returned master device_info values
// whenever the prior Terraform state already held a non-empty device_info list,
// indicating the user deliberately tracked those values. Clusters that had
// device_info: [] in prior state are left untouched -- their suppression is
// intentional (user omitted the block and relies on TServer inheritance).
//
// Cluster matching mirrors restoreRedactedPasswords: UUID-first, then index.
func restoreDedicatedMasterDeviceInfo(
	newClusters []map[string]interface{},
	oldClusters []interface{},
	apiClusters []client.Cluster,
) {
	oldByUUID := make(map[string]map[string]interface{}, len(oldClusters))
	for _, oc := range oldClusters {
		ocm, ok := oc.(map[string]interface{})
		if !ok {
			continue
		}
		uuid, _ := ocm["uuid"].(string)
		if uuid != "" {
			oldByUUID[uuid] = ocm
		}
	}
	apiByUUID := make(map[string]client.Cluster, len(apiClusters))
	for _, ac := range apiClusters {
		if ac.GetUuid() != "" {
			apiByUUID[ac.GetUuid()] = ac
		}
	}

	for i, nc := range newClusters {
		uuid, _ := nc["uuid"].(string)

		var oldCluster map[string]interface{}
		if uuid != "" {
			oldCluster = oldByUUID[uuid]
		}
		if oldCluster == nil && i < len(oldClusters) {
			oldCluster, _ = oldClusters[i].(map[string]interface{})
		}
		if oldCluster == nil {
			continue
		}

		// Locate the prior dedicated_masters.device_info in old state.
		oldUIList, ok := oldCluster["user_intent"].([]interface{})
		if !ok || len(oldUIList) == 0 {
			continue
		}
		oldUI, ok := oldUIList[0].(map[string]interface{})
		if !ok {
			continue
		}
		oldDMList, _ := oldUI["dedicated_masters"].([]interface{})
		if len(oldDMList) == 0 {
			continue
		}
		oldDM, ok := oldDMList[0].(map[string]interface{})
		if !ok {
			continue
		}
		oldDIList, _ := oldDM["device_info"].([]interface{})
		if len(oldDIList) == 0 {
			// Prior state had no explicit device_info -- suppression is correct.
			continue
		}

		// Prior state had explicit device_info. If the flattener suppressed it
		// to [] in newClusters, restore actual API master device_info values.
		newUIList, ok := nc["user_intent"].([]interface{})
		if !ok || len(newUIList) == 0 {
			continue
		}
		newUI, ok := newUIList[0].(map[string]interface{})
		if !ok {
			continue
		}
		newDMList, _ := newUI["dedicated_masters"].([]interface{})
		if len(newDMList) == 0 {
			continue
		}
		newDM, ok := newDMList[0].(map[string]interface{})
		if !ok {
			continue
		}
		if diList, _ := newDM["device_info"].([]interface{}); len(diList) > 0 {
			// Flattener already wrote values -- nothing to restore.
			continue
		}

		// Find the matching API cluster to get the live MasterDeviceInfo.
		var apiCluster client.Cluster
		var found bool
		if uuid != "" {
			apiCluster, found = apiByUUID[uuid]
		}
		if !found && i < len(apiClusters) {
			apiCluster = apiClusters[i]
			found = true
		}
		if !found || apiCluster.UserIntent.MasterDeviceInfo == nil {
			continue
		}
		newDM["device_info"] = flattenDeviceInfo(apiCluster.UserIntent.MasterDeviceInfo)
	}
}

// alignClustersCloudList reorders the cloud_list, region_list, and az_list
// within each flattened cluster to match the order held in the prior Terraform
// state. This prevents spurious TypeList index-shift diffs after every read.
// Cluster matching mirrors restoreRedactedPasswords: UUID-first, then index.
func alignClustersCloudList(
	newClusters []map[string]interface{},
	oldClusters []interface{},
) {
	oldByUUID := make(map[string]map[string]interface{}, len(oldClusters))
	for _, oc := range oldClusters {
		ocm, ok := oc.(map[string]interface{})
		if !ok {
			continue
		}
		uuid, _ := ocm["uuid"].(string)
		if uuid != "" {
			oldByUUID[uuid] = ocm
		}
	}

	for i, nc := range newClusters {
		uuid, _ := nc["uuid"].(string)
		var oldCluster map[string]interface{}
		if uuid != "" {
			oldCluster = oldByUUID[uuid]
		}
		if oldCluster == nil && i < len(oldClusters) {
			oldCluster, _ = oldClusters[i].(map[string]interface{})
		}
		if oldCluster == nil {
			continue
		}
		newCloudList, _ := nc["cloud_list"].([]interface{})
		oldCloudList, _ := oldCluster["cloud_list"].([]interface{})
		if len(newCloudList) > 0 && len(oldCloudList) > 0 {
			newClusters[i]["cloud_list"] = alignCloudList(newCloudList, oldCloudList)
		}
	}
}

func flattenNodeDetailsSet(nsd []client.NodeDetailsResp) (res []interface{}) {
	for _, n := range nsd {
		var lastVolTime string
		if n.LastVolumeUpdateTime != nil {
			// .Format(time.RFC3339) creates a standard ISO-8601 string
			lastVolTime = n.LastVolumeUpdateTime.Format(time.RFC3339)
		}
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
			"last_volume_update_time":     lastVolTime,
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
