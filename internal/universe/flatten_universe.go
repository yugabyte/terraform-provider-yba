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
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
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
//
// Returns Warning diagnostics for any redacted field that could not be
// restored from prior state - typically the import-bootstrap case, where the
// "REDACTED" sentinel ends up in state and the operator needs to use
// lifecycle.ignore_changes or hand-patch state. See the Import section of
// the universe resource docs.
func restoreRedactedPasswords(
	ctx context.Context,
	newClusters []map[string]interface{},
	oldClusters []interface{},
) diag.Diagnostics {
	const redacted = "REDACTED"
	var diags diag.Diagnostics

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

		var oldUIMap map[string]interface{}
		if oldCluster != nil {
			if oldUIList, ok := oldCluster["user_intent"].([]interface{}); ok &&
				len(oldUIList) > 0 {
				oldUIMap, _ = oldUIList[0].(map[string]interface{})
			}
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
			if !ok || p == nil || *p != redacted {
				continue
			}
			var oldVal string
			if oldUIMap != nil {
				oldVal, _ = oldUIMap[field].(string)
			}
			if oldVal != "" {
				tflog.Debug(ctx, "restoreRedactedPasswords: restoring redacted field",
					map[string]interface{}{
						"index": i, "uuid": uuid, "field": field,
					})
				newUIMap[field] = oldVal
				continue
			}
			// No prior value to restore - the literal "REDACTED" sentinel
			// will land in state. This is the import-bootstrap case.
			tflog.Warn(ctx, "restoreRedactedPasswords: no prior value for redacted field",
				map[string]interface{}{
					"index": i, "uuid": uuid, "field": field,
				})
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary: fmt.Sprintf(
					"%s could not be populated from YBA on read; state contains the sentinel \"REDACTED\"",
					field,
				),
				Detail: fmt.Sprintf(
					"YBA does not return plaintext passwords. After import (or any refresh "+
						"with no prior state for this field), state holds \"REDACTED\" for "+
						"clusters[%d].user_intent[0].%s. The next plan will show a diff "+
						"from \"REDACTED\" to your configured value, and the update will "+
						"be rejected because passwords cannot be changed after universe "+
						"creation. Add a lifecycle.ignore_changes block for this field or "+
						"patch state with the real value. See the Import section of the "+
						"yba_universe docs.",
					i, field),
			})
		}
	}
	return diags
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
		"tserver_gflags":                tserverFromIntent(ui),
		"master_gflags":                 masterFromIntent(ui),
		"specific_gflags":               flattenSpecificGFlags(ui.SpecificGFlags),
		"dedicated_masters":             flattenDedicatedMasters(ui),
	}
	return utils.CreateSingletonList(v)
}

// flattenSpecificGFlags converts the API SpecificGFlags model into the
// Terraform specific_gflags block. Returns an empty list when the API did not
// return any specific_gflags data.
func flattenSpecificGFlags(sg *client.SpecificGFlags) []interface{} {
	if sg == nil {
		return []interface{}{}
	}
	out := map[string]interface{}{
		"inherit_from_primary": sg.GetInheritFromPrimary(),
		"gflag_groups":         sg.GetGflagGroups(),
		"per_process":          []interface{}{},
		"per_az":               []interface{}{},
	}
	if sg.PerProcessFlags != nil {
		ppf := map[string]interface{}{}
		if m, ok := sg.PerProcessFlags.Value["MASTER"]; ok && len(m) > 0 {
			ppf["master_gflags"] = m
		}
		if t, ok := sg.PerProcessFlags.Value["TSERVER"]; ok && len(t) > 0 {
			ppf["tserver_gflags"] = t
		}
		if len(ppf) > 0 {
			out["per_process"] = []interface{}{ppf}
		}
	}
	if sg.PerAZ != nil && len(*sg.PerAZ) > 0 {
		az := make([]interface{}, 0, len(*sg.PerAZ))
		for uuid, ppf := range *sg.PerAZ {
			entry := map[string]interface{}{"az_uuid": uuid}
			if m, ok := ppf.Value["MASTER"]; ok && len(m) > 0 {
				entry["master_gflags"] = m
			}
			if t, ok := ppf.Value["TSERVER"]; ok && len(t) > 0 {
				entry["tserver_gflags"] = t
			}
			az = append(az, entry)
		}
		out["per_az"] = az
	}
	return []interface{}{out}
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

// restoreDedicatedMasterFields corrects a suppression edge case: when the user
// explicitly wrote dedicated_masters.instance_type or dedicated_masters.device_info
// whose values happen to equal the TServer equivalents, flattenDedicatedMasters
// suppresses them (because it cannot distinguish "user omitted it" from "user
// wrote identical values"). That produces a permanent plan diff: config has the
// field, state does not.
//
// Restoration rule (applied independently to instance_type and device_info):
//   - On terraform import (oldClusters empty), always restore the API master
//     value so the imported state faithfully reflects server state.
//   - On a regular post-apply read, restore when EITHER the prior Terraform
//     state held a non-empty value for that field OR the user's current HCL
//     (rawConfig) explicitly authors it. The rawConfig signal closes the
//     transition gap where the user just switched from `dedicated_masters {}`
//     to an explicit value that happens to equal the TServer field -- prior
//     state is still empty, so without rawConfig we would never restore.
//
// Cluster matching mirrors restoreRedactedPasswords: UUID-first, then index.
func restoreDedicatedMasterFields(
	newClusters []map[string]interface{},
	oldClusters []interface{},
	apiClusters []client.Cluster,
	rawConfig cty.Value,
) {
	importing := len(oldClusters) == 0

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

		// Determine whether each field should be restored. On import, both
		// are restored unconditionally. On a regular read, each is gated on
		// prior state or raw config explicitly tracking the field.
		restoreIT, restoreDI := importing, importing
		if !importing {
			var oldCluster map[string]interface{}
			if uuid != "" {
				oldCluster = oldByUUID[uuid]
			}
			if oldCluster == nil && i < len(oldClusters) {
				oldCluster, _ = oldClusters[i].(map[string]interface{})
			}
			priorIT, priorDI := priorStateHasDedicatedMasterFields(oldCluster)
			cfgIT, cfgDI := rawConfigHasDedicatedMasterFields(rawConfig, i)
			restoreIT = priorIT || cfgIT
			restoreDI = priorDI || cfgDI
			if !restoreIT && !restoreDI {
				continue
			}
		}

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

		var apiCluster client.Cluster
		var found bool
		if uuid != "" {
			apiCluster, found = apiByUUID[uuid]
		}
		if !found && i < len(apiClusters) {
			apiCluster = apiClusters[i]
			found = true
		}
		if !found {
			continue
		}

		if restoreIT {
			if it, _ := newDM["instance_type"].(string); it == "" {
				if mit := apiCluster.UserIntent.GetMasterInstanceType(); mit != "" {
					newDM["instance_type"] = mit
				}
			}
		}
		if restoreDI {
			if diList, _ := newDM["device_info"].([]interface{}); len(diList) == 0 &&
				apiCluster.UserIntent.MasterDeviceInfo != nil {
				newDM["device_info"] = flattenDeviceInfo(apiCluster.UserIntent.MasterDeviceInfo)
			}
		}
	}
}

// priorStateHasDedicatedMasterFields reports whether the prior Terraform state
// tracked an explicit dedicated_masters.instance_type (non-empty) and/or
// dedicated_masters.device_info (non-empty list).
func priorStateHasDedicatedMasterFields(
	oldCluster map[string]interface{},
) (hasIT, hasDI bool) {
	if oldCluster == nil {
		return false, false
	}
	oldUIList, _ := oldCluster["user_intent"].([]interface{})
	if len(oldUIList) == 0 {
		return false, false
	}
	oldUI, ok := oldUIList[0].(map[string]interface{})
	if !ok {
		return false, false
	}
	oldDMList, _ := oldUI["dedicated_masters"].([]interface{})
	if len(oldDMList) == 0 {
		return false, false
	}
	oldDM, ok := oldDMList[0].(map[string]interface{})
	if !ok {
		return false, false
	}
	if it, _ := oldDM["instance_type"].(string); it != "" {
		hasIT = true
	}
	if oldDIList, _ := oldDM["device_info"].([]interface{}); len(oldDIList) > 0 {
		hasDI = true
	}
	return
}

// rawConfigHasDedicatedMasterFields inspects the user's HCL config for cluster i
// and reports whether dedicated_masters.instance_type and/or .device_info are
// explicitly authored. Used by restoreDedicatedMasterFields to close the
// transition gap where prior state was empty (user wrote `dedicated_masters {}`)
// but the new config sets an explicit value that happens to equal the TServer
// field -- the flattener suppresses it, so without a config signal we would
// produce a permanent plan diff.
//
// TypeList traversal in cty mirrors cloudListInRawConfig: an absent nested
// block can surface as either null or a zero-length list, so we treat both as
// "not authored."
func rawConfigHasDedicatedMasterFields(rawConfig cty.Value, i int) (hasIT, hasDI bool) {
	if rawConfig == cty.NilVal || !rawConfig.IsKnown() || rawConfig.IsNull() {
		return false, false
	}
	clusters := rawConfig.GetAttr("clusters")
	if !clusters.IsKnown() || clusters.IsNull() {
		return false, false
	}
	clusterSlice := clusters.AsValueSlice()
	if i >= len(clusterSlice) {
		return false, false
	}
	clusterVal := clusterSlice[i]
	if !clusterVal.IsKnown() || clusterVal.IsNull() {
		return false, false
	}
	uiVal := clusterVal.GetAttr("user_intent")
	if uiVal == cty.NilVal || !uiVal.IsKnown() || uiVal.IsNull() {
		return false, false
	}
	uiType := uiVal.Type()
	if (!uiType.IsListType() && !uiType.IsTupleType()) || uiVal.LengthInt() == 0 {
		return false, false
	}
	ui := uiVal.AsValueSlice()[0]
	if !ui.IsKnown() || ui.IsNull() {
		return false, false
	}
	dmVal := ui.GetAttr("dedicated_masters")
	if dmVal == cty.NilVal || !dmVal.IsKnown() || dmVal.IsNull() {
		return false, false
	}
	dmType := dmVal.Type()
	if (!dmType.IsListType() && !dmType.IsTupleType()) || dmVal.LengthInt() == 0 {
		return false, false
	}
	dm := dmVal.AsValueSlice()[0]
	if !dm.IsKnown() || dm.IsNull() {
		return false, false
	}
	if itVal := dm.GetAttr("instance_type"); itVal != cty.NilVal &&
		itVal.IsKnown() && !itVal.IsNull() && itVal.AsString() != "" {
		hasIT = true
	}
	if diVal := dm.GetAttr("device_info"); diVal != cty.NilVal &&
		diVal.IsKnown() && !diVal.IsNull() {
		t := diVal.Type()
		if (t.IsListType() || t.IsTupleType()) && diVal.LengthInt() > 0 {
			hasDI = true
		}
	}
	return
}

// pruneSpecificGFlagsByConfig clears specific_gflags on clusters where the
// customer's HCL did not author it. Skips when rawConfig is unavailable
// (e.g. terraform import or pure refresh) so server state is preserved.
func pruneSpecificGFlagsByConfig(
	newClusters []map[string]interface{},
	rawConfig cty.Value,
) {
	if rawConfig == cty.NilVal || !rawConfig.IsKnown() || rawConfig.IsNull() {
		return
	}
	for i, nc := range newClusters {
		if rawConfigHasSpecificGFlags(rawConfig, i) {
			continue
		}
		uiList, _ := nc["user_intent"].([]interface{})
		if len(uiList) == 0 {
			continue
		}
		ui, ok := uiList[0].(map[string]interface{})
		if !ok {
			continue
		}
		ui["specific_gflags"] = []interface{}{}
	}
}

func rawConfigHasSpecificGFlags(rawConfig cty.Value, i int) bool {
	clusters := rawConfig.GetAttr("clusters")
	if !clusters.IsKnown() || clusters.IsNull() {
		return false
	}
	clusterSlice := clusters.AsValueSlice()
	if i >= len(clusterSlice) {
		return false
	}
	clusterVal := clusterSlice[i]
	if !clusterVal.IsKnown() || clusterVal.IsNull() {
		return false
	}
	uiVal := clusterVal.GetAttr("user_intent")
	if uiVal == cty.NilVal || !uiVal.IsKnown() || uiVal.IsNull() {
		return false
	}
	uiType := uiVal.Type()
	if (!uiType.IsListType() && !uiType.IsTupleType()) || uiVal.LengthInt() == 0 {
		return false
	}
	ui := uiVal.AsValueSlice()[0]
	if !ui.IsKnown() || ui.IsNull() {
		return false
	}
	sgVal := ui.GetAttr("specific_gflags")
	if sgVal == cty.NilVal || !sgVal.IsKnown() || sgVal.IsNull() {
		return false
	}
	t := sgVal.Type()
	if !t.IsListType() && !t.IsTupleType() {
		return false
	}
	return sgVal.LengthInt() > 0
}

func ctyHasNonEmptyBlock(val cty.Value, attr string) bool {
	v := val.GetAttr(attr)
	if v == cty.NilVal || !v.IsKnown() || v.IsNull() {
		return false
	}
	t := v.Type()
	if !t.IsListType() && !t.IsTupleType() {
		return false
	}
	return v.LengthInt() > 0
}

// gflagGroupsFromClusterHCL pulls user_intent[0].specific_gflags[0].gflag_groups
// from a cluster cty.Value (typically a slice element of d.GetRawConfig().GetAttr
// ("clusters")). Returns an empty slice when the path is null, missing, or empty.
func gflagGroupsFromClusterHCL(clusterVal cty.Value) []string {
	uiVal := clusterVal.GetAttr("user_intent")
	if uiVal == cty.NilVal || !uiVal.IsKnown() || uiVal.IsNull() {
		return nil
	}
	uiType := uiVal.Type()
	if (!uiType.IsListType() && !uiType.IsTupleType()) || uiVal.LengthInt() == 0 {
		return nil
	}
	ui := uiVal.AsValueSlice()[0]
	if !ui.IsKnown() || ui.IsNull() {
		return nil
	}
	sgVal := ui.GetAttr("specific_gflags")
	if sgVal == cty.NilVal || !sgVal.IsKnown() || sgVal.IsNull() {
		return nil
	}
	sgType := sgVal.Type()
	if (!sgType.IsListType() && !sgType.IsTupleType()) || sgVal.LengthInt() == 0 {
		return nil
	}
	sg := sgVal.AsValueSlice()[0]
	if !sg.IsKnown() || sg.IsNull() {
		return nil
	}
	gg := sg.GetAttr("gflag_groups")
	if gg == cty.NilVal || !gg.IsKnown() || gg.IsNull() {
		return nil
	}
	ggType := gg.Type()
	if !ggType.IsListType() && !ggType.IsTupleType() {
		return nil
	}
	out := make([]string, 0, gg.LengthInt())
	for _, v := range gg.AsValueSlice() {
		if !v.IsKnown() || v.IsNull() {
			continue
		}
		// Normalize to upper-case to match the StateFunc on the gflag_groups
		// element so HCL case differences do not falsely trigger the
		// mismatch check.
		out = append(out, strings.ToUpper(v.AsString()))
	}
	return out
}

func ctyHasNonEmptyMap(val cty.Value, attr string) bool {
	v := val.GetAttr(attr)
	if v == cty.NilVal || !v.IsKnown() || v.IsNull() {
		return false
	}
	t := v.Type()
	if !t.IsMapType() && !t.IsObjectType() {
		return false
	}
	return v.LengthInt() > 0
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
