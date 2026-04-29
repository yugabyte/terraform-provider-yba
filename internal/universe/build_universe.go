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

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func buildUniverse(d *schema.ResourceData) client.UniverseConfigureTaskParams {
	clusters := buildClusters(d.Get("clusters").([]interface{}))
	enableYbc := true
	rootCA := d.Get("root_ca").(string)
	clientRootCA := d.Get("client_root_ca").(string)
	// rootAndClientRootCASame defaults to true on the YBA server side.  When true, the
	// server ignores the clientRootCA field and sets it equal to rootCA instead.  We must
	// explicitly send false in any situation where the caller has chosen a clientRootCA that
	// differs from rootCA -- including when rootCA is absent and the server will auto-generate
	// one:
	//
	//   client_root_ca set, root_ca not set   -> false (server must not override with auto-rootCA)
	//   client_root_ca set, root_ca set same  -> nil  (server default "true" is correct)
	//   client_root_ca set, root_ca set diff  -> false (keep certs separate)
	//   client_root_ca not set                -> nil  (server picks/reuses rootCA for both)
	var rootAndClientRootCASame *bool
	if clientRootCA != "" && (rootCA == "" || clientRootCA != rootCA) {
		rootAndClientRootCASame = utils.GetBoolPointer(false)
	}
	// Only set RootCA/ClientRootCA when the UUID is non-empty.  Sending a
	// pointer to an empty string serialises as "rootCA": "" in JSON, which the
	// server treats differently from a missing/null field: it may bypass the
	// rootAndClientRootCASame logic and reset both certs to the auto-generated
	// rootCA.  Omitting the field entirely (nil pointer) lets the server
	// correctly auto-generate rootCA while still honoring the separate
	// clientRootCA when rootAndClientRootCASame=false.
	params := client.UniverseConfigureTaskParams{
		RootAndClientRootCASame: rootAndClientRootCASame,
		Arch:                    utils.GetStringPointer(d.Get("arch").(string)),
		Clusters:                clusters,
		CommunicationPorts: buildCommunicationPorts(
			utils.MapFromSingletonList(d.Get("communication_ports").([]interface{}))),
		EnableYbc:      utils.GetBoolPointer(enableYbc),
		UserAZSelected: utils.GetBoolPointer(hasExplicitCloudList(clusters)),
	}
	if rootCA != "" {
		params.RootCA = utils.GetStringPointer(rootCA)
	}
	if clientRootCA != "" {
		params.ClientRootCA = utils.GetStringPointer(clientRootCA)
	}
	return params
}

func buildUniverseDefinitionTaskParams(d *schema.ResourceData) client.UniverseDefinitionTaskParams {
	rootCA := d.Get("root_ca").(string)
	clientRootCA := d.Get("client_root_ca").(string)
	// See comment in buildUniverse for the full rationale behind this flag.
	var rootAndClientRootCASame *bool
	if clientRootCA != "" && (rootCA == "" || clientRootCA != rootCA) {
		rootAndClientRootCASame = utils.GetBoolPointer(false)
	}
	clusters := buildClusters(d.Get("clusters").([]interface{}))
	// See comment in buildUniverse: only set RootCA/ClientRootCA when non-empty
	// so that the server receives a missing/null field rather than "" when the
	// user has not specified a cert UUID.
	params := client.UniverseDefinitionTaskParams{
		RootAndClientRootCASame: rootAndClientRootCASame,
		Clusters:                clusters,
		CommunicationPorts: buildCommunicationPorts(
			utils.MapFromSingletonList(d.Get("communication_ports").([]interface{}))),
		UserAZSelected: utils.GetBoolPointer(hasExplicitCloudList(clusters)),
	}
	if rootCA != "" {
		params.RootCA = utils.GetStringPointer(rootCA)
	}
	if clientRootCA != "" {
		params.ClientRootCA = utils.GetStringPointer(clientRootCA)
	}
	return params
}

// hasExplicitCloudList returns true when at least one cluster in the request
// carries an explicit PlacementInfo, mirroring the UI's userAZSelected flag.
// With userAZSelected=true YBA treats the per-AZ node counts as the source of
// truth and derives userIntent.numNodes from them, instead of the reverse.
func hasExplicitCloudList(clusters []client.Cluster) bool {
	for _, c := range clusters {
		if c.PlacementInfo != nil && len(c.PlacementInfo.CloudList) > 0 {
			return true
		}
	}
	return false
}

// resolveAZUUIDs fixes the UUID on every PlacementRegion and PlacementAZ in
// newPI by matching on Code (for regions) and Name (for AZs) against the live
// cloud list fetched from the YBA API (oldCloudList).
//
// The problem it solves: region_list and az_list are TypeLists in the schema,
// so Terraform matches elements by index position. When the practitioner
// removes a region or AZ from the middle of the list, subsequent elements shift
// down one index and inherit the wrong state UUID (the removed element's UUID).
// This leads to incorrect ToBeRemoved node marking and wrong placement sent to
// the API.
//
// When a new AZ or region is introduced that was never part of the prior
// placement (e.g. moving a node from AZ1 to AZ2), oldCloudList has no entry
// for it and the UUID in Terraform state is stale (it still holds the old AZ's
// UUID). fallbackByRegionCode and fallbackByAZCode, built from the provider's
// full region/zone list, are used in that case.
func resolveAZUUIDs(
	newPI *client.PlacementInfo,
	oldCloudList []client.PlacementCloud,
	fallbackByRegionCode map[string]string,
	fallbackByAZCode map[string]string,
) {
	if newPI == nil {
		return
	}
	// Build lookup maps from the live API state (correct, stable UUIDs).
	uuidByRegionCode := make(map[string]string)
	uuidByAZName := make(map[string]string)
	for _, cloud := range oldCloudList {
		for _, region := range cloud.GetRegionList() {
			if region.GetCode() != "" && region.GetUuid() != "" {
				uuidByRegionCode[region.GetCode()] = region.GetUuid()
			}
			for _, az := range region.GetAzList() {
				if az.GetName() != "" && az.GetUuid() != "" {
					uuidByAZName[az.GetName()] = az.GetUuid()
				}
			}
		}
	}
	// Overwrite each region's and AZ's UUID in the new placement with the
	// resolved value from the live API state, falling back to the full
	// provider zone list for AZs/regions not present in the current placement.
	for ci := range newPI.CloudList {
		for ri := range newPI.CloudList[ci].RegionList {
			region := &newPI.CloudList[ci].RegionList[ri]
			if uuid, ok := uuidByRegionCode[region.GetCode()]; ok {
				region.Uuid = utils.GetStringPointer(uuid)
			} else if fallbackByRegionCode != nil {
				if uuid, ok := fallbackByRegionCode[region.GetCode()]; ok {
					region.Uuid = utils.GetStringPointer(uuid)
				}
			}
			for ai := range region.AzList {
				az := &region.AzList[ai]
				if uuid, ok := uuidByAZName[az.GetName()]; ok {
					az.Uuid = utils.GetStringPointer(uuid)
				} else if fallbackByAZCode != nil {
					if uuid, ok := fallbackByAZCode[az.GetName()]; ok {
						az.Uuid = utils.GetStringPointer(uuid)
					}
				}
			}
		}
	}
}

// buildProviderZoneFallback constructs code->UUID lookup maps for all regions
// and zones returned by the provider's region list API. These are used as a
// fallback in resolveAZUUIDs for AZs that do not appear in the current live
// placement (e.g. when moving a node to a new AZ within the same region).
// Multiple calls merge into the same maps, so callers can accumulate results
// from several providers (multi-cloud universes).
func buildProviderZoneFallback(
	regions []client.Region,
	byRegionCode map[string]string,
	byAZCode map[string]string,
) {
	for _, r := range regions {
		if r.GetCode() != "" && r.GetUuid() != "" {
			byRegionCode[r.GetCode()] = r.GetUuid()
		}
		for _, az := range r.GetZones() {
			// AvailabilityZone.Name is the zone code (e.g. "us-west-2a").
			if az.GetName() != "" && az.GetUuid() != "" {
				byAZCode[az.GetName()] = az.GetUuid()
			}
		}
	}
}

// fetchProviderZoneFallback fetches the full region/zone list for every
// distinct provider UUID found in oldCloudList (the live placement) and
// merges the results into a single pair of code->UUID lookup maps.
// Using oldCloudList (live state) rather than the desired config ensures we
// query the correct provider even if the user's config has an incorrect or
// changed provider UUID. All clouds in a multi-cloud universe are covered.
func fetchProviderZoneFallback(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	oldCloudList []client.PlacementCloud,
) (byRegionCode map[string]string, byAZCode map[string]string) {
	byRegionCode = make(map[string]string)
	byAZCode = make(map[string]string)
	seen := make(map[string]bool)
	for _, cloud := range oldCloudList {
		providerUUID := cloud.GetUuid()
		if providerUUID == "" || seen[providerUUID] {
			continue
		}
		seen[providerUUID] = true
		provRegions, _, err := c.RegionManagementAPI.GetRegion(ctx, cUUID, providerUUID).Execute()
		if err != nil {
			tflog.Warn(ctx, fmt.Sprintf(
				"could not fetch provider regions for UUID %s: %v; "+
					"AZ UUID resolution will use current placement only",
				providerUUID, err))
			continue
		}
		buildProviderZoneFallback(provRegions, byRegionCode, byAZCode)
	}
	return byRegionCode, byAZCode
}

func buildCommunicationPorts(cp map[string]interface{}) *client.CommunicationPorts {
	if len(cp) == 0 {
		return &client.CommunicationPorts{}
	}
	ports := &client.CommunicationPorts{
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
	if v, ok := cp["yb_controller_rpc_port"].(int); ok && v != 0 {
		ports.YbControllerrRpcPort = utils.GetInt32Pointer(int32(v))
	}
	return ports
}

func buildClusters(clusters []interface{}) (res []client.Cluster) {
	for _, v := range clusters {
		cluster := v.(map[string]interface{})
		c := client.Cluster{
			ClusterType: cluster["cluster_type"].(string),
			UserIntent: buildUserIntent(
				utils.MapFromSingletonList(cluster["user_intent"].([]interface{})),
			),
		}
		if len(cluster["cloud_list"].([]interface{})) > 0 {
			c.PlacementInfo = &client.PlacementInfo{
				CloudList: buildCloudList(cluster["cloud_list"].(interface{})),
			}
		}

		res = append(res, c)
	}
	return res
}

func buildCloudList(clI interface{}) (res []client.PlacementCloud) {
	if clI == nil {
		return nil
	}
	cl := clI.([]interface{})
	for _, v := range cl {
		c := v.(map[string]interface{})
		pc := client.PlacementCloud{
			Uuid:       utils.GetStringPointer(c["provider"].(string)),
			Code:       utils.GetStringPointer(c["code"].(string)),
			RegionList: buildRegionList(c["region_list"].([]interface{})),
		}
		res = append(res, pc)
	}
	return res
}

func buildRegionList(cl []interface{}) []client.PlacementRegion {
	var res []client.PlacementRegion
	for _, v := range cl {
		r := v.(map[string]interface{})
		pr := client.PlacementRegion{
			Uuid:   utils.GetStringPointer(r["uuid"].(string)),
			Code:   utils.GetStringPointer(r["code"].(string)),
			Name:   utils.GetStringPointer(r["name"].(string)),
			AzList: buildAzList(r["az_list"].(interface{})),
		}
		res = append(res, pr)
	}
	return res
}

func buildAzList(clI interface{}) []client.PlacementAZ {
	var res []client.PlacementAZ
	if clI == nil {
		return res
	}
	cl := clI.([]interface{})
	for _, v := range cl {
		az := v.(map[string]interface{})
		paz := client.PlacementAZ{
			Uuid:              utils.GetStringPointer(az["uuid"].(string)),
			IsAffinitized:     utils.GetBoolPointer(az["is_affinitized"].(bool)),
			LeaderPreference:  utils.GetInt32Pointer(int32(az["leader_preference"].(int))),
			Name:              utils.GetStringPointer(az["code"].(string)),
			NumNodesInAZ:      utils.GetInt32Pointer(int32(az["num_nodes"].(int))),
			ReplicationFactor: utils.GetInt32Pointer(int32(az["replication_factor"].(int))),
			SecondarySubnet:   utils.GetStringPointer(az["secondary_subnet"].(string)),
			Subnet:            utils.GetStringPointer(az["subnet"].(string)),
		}
		res = append(res, paz)
	}
	return res
}

func buildUserIntent(ui map[string]interface{}) client.UserIntent {
	return client.UserIntent{
		AssignStaticPublicIP: utils.GetBoolPointer(ui["assign_static_ip"].(bool)),
		AwsArnString:         utils.GetStringPointer(ui["aws_arn_string"].(string)),
		EnableIPV6:           utils.GetBoolPointer(ui["enable_ipv6"].(bool)),
		EnableYCQL:           utils.GetBoolPointer(ui["enable_ycql"].(bool)),
		EnableYCQLAuth:       utils.GetBoolPointer(ui["enable_ycql_auth"].(bool)),
		EnableYSQLAuth:       utils.GetBoolPointer(ui["enable_ysql_auth"].(bool)),
		ImageBundleUUID:      utils.GetStringPointer(ui["image_bundle_uuid"].(string)),
		InstanceTags:         utils.StringMap(ui["instance_tags"].(map[string]interface{})),
		PreferredRegion:      utils.GetStringPointer(ui["preferred_region"].(string)),
		UseHostname:          utils.GetBoolPointer(ui["use_host_name"].(bool)),
		UseSystemd:           utils.GetBoolPointer(ui["use_systemd"].(bool)),
		YsqlPassword:         utils.GetStringPointer(ui["ysql_password"].(string)),
		YcqlPassword:         utils.GetStringPointer(ui["ycql_password"].(string)),
		UniverseName:         utils.GetStringPointer(ui["universe_name"].(string)),
		ProviderType:         utils.GetStringPointer(ui["provider_type"].(string)),
		Provider:             utils.GetStringPointer(ui["provider"].(string)),
		RegionList:           *utils.StringSlice(ui["region_list"].([]interface{})),
		NumNodes:             utils.GetInt32Pointer(int32(ui["num_nodes"].(int))),
		ReplicationFactor:    utils.GetInt32Pointer(int32(ui["replication_factor"].(int))),
		InstanceType:         utils.GetStringPointer(ui["instance_type"].(string)),
		DeviceInfo: buildDeviceInfo(
			utils.MapFromSingletonList(ui["device_info"].([]interface{}))),
		AssignPublicIP:            utils.GetBoolPointer(ui["assign_public_ip"].(bool)),
		UseTimeSync:               utils.GetBoolPointer(ui["use_time_sync"].(bool)),
		EnableYSQL:                utils.GetBoolPointer(ui["enable_ysql"].(bool)),
		EnableYEDIS:               utils.GetBoolPointer(ui["enable_yedis"].(bool)),
		EnableNodeToNodeEncrypt:   utils.GetBoolPointer(ui["enable_node_to_node_encrypt"].(bool)),
		EnableClientToNodeEncrypt: utils.GetBoolPointer(ui["enable_client_to_node_encrypt"].(bool)),
		YbSoftwareVersion:         utils.GetStringPointer(ui["yb_software_version"].(string)),
		AccessKeyCode:             utils.GetStringPointer(ui["access_key_code"].(string)),
		TserverGFlags:             utils.StringMap(ui["tserver_gflags"].(map[string]interface{})),
		MasterGFlags:              utils.StringMap(ui["master_gflags"].(map[string]interface{})),
	}
}

func buildDeviceInfo(di map[string]interface{}) *client.DeviceInfo {
	return &client.DeviceInfo{
		DiskIops:    utils.GetInt32Pointer(int32(di["disk_iops"].(int))),
		MountPoints: utils.GetStringPointer(di["mount_points"].(string)),
		Throughput:  utils.GetInt32Pointer(int32(di["throughput"].(int))),
		NumVolumes:  utils.GetInt32Pointer(int32(di["num_volumes"].(int))),
		VolumeSize:  utils.GetInt32Pointer(int32(di["volume_size"].(int))),
		StorageType: utils.GetStringPointer(di["storage_type"].(string)),
	}
}

// collectAZUUIDs returns a set of AZ UUIDs present in the given cloud list.
func collectAZUUIDs(cloudList []client.PlacementCloud) map[string]bool {
	uuids := make(map[string]bool)
	for _, cloud := range cloudList {
		for _, region := range cloud.GetRegionList() {
			for _, az := range region.GetAzList() {
				uuids[az.GetUuid()] = true
			}
		}
	}
	return uuids
}

func buildNodeDetailsRespArrayToNodeDetailsArray(
	nodes []client.NodeDetailsResp,
) []client.NodeDetails {
	var nodesDetails []client.NodeDetails
	for _, v := range nodes {
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
	return nodesDetails
}
