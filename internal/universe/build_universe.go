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

// cloudListWithoutLeaderPreferenceAndRF returns a deep copy of a cloud_list
// with both leader_preference and replication_factor zeroed out in every
// az_list entry. It is used to determine whether the only differing fields
// between two cloud lists are leader_preference and/or replication_factor:
// if stripping both makes the lists reflect.DeepEqual, nothing else changed.
func cloudListWithoutLeaderPreferenceAndRF(cloudList []interface{}) []interface{} {
	out := make([]interface{}, 0, len(cloudList))
	for _, pcRaw := range cloudList {
		pc, ok := pcRaw.(map[string]interface{})
		if !ok {
			out = append(out, pcRaw)
			continue
		}
		pcCopy := make(map[string]interface{}, len(pc))
		for k, v := range pc {
			pcCopy[k] = v
		}
		rlRaw, _ := pc["region_list"].([]interface{})
		rlCopy := make([]interface{}, 0, len(rlRaw))
		for _, prRaw := range rlRaw {
			pr, ok := prRaw.(map[string]interface{})
			if !ok {
				rlCopy = append(rlCopy, prRaw)
				continue
			}
			prCopy := make(map[string]interface{}, len(pr))
			for k, v := range pr {
				prCopy[k] = v
			}
			azRaw, _ := pr["az_list"].([]interface{})
			azCopy := make([]interface{}, 0, len(azRaw))
			for _, azItemRaw := range azRaw {
				az, ok := azItemRaw.(map[string]interface{})
				if !ok {
					azCopy = append(azCopy, azItemRaw)
					continue
				}
				azMap := make(map[string]interface{}, len(az))
				for k, v := range az {
					azMap[k] = v
				}
				azMap["leader_preference"] = 0
				azMap["replication_factor"] = 0
				azCopy = append(azCopy, azMap)
			}
			prCopy["az_list"] = azCopy
			rlCopy = append(rlCopy, prCopy)
		}
		pcCopy["region_list"] = rlCopy
		out = append(out, pcCopy)
	}
	return out
}

// cloudListWithoutLeaderPreference returns a deep copy of a cloud_list
// ([]interface{}) with the leader_preference field zeroed out in every az_list
// entry. The copy is used to determine whether leader_preference is the ONLY
// differing field between two cloud lists: if stripping it makes them
// reflect.DeepEqual, nothing else changed.
func cloudListWithoutLeaderPreference(cloudList []interface{}) []interface{} {
	out := make([]interface{}, 0, len(cloudList))
	for _, pcRaw := range cloudList {
		pc, ok := pcRaw.(map[string]interface{})
		if !ok {
			out = append(out, pcRaw)
			continue
		}
		pcCopy := make(map[string]interface{}, len(pc))
		for k, v := range pc {
			pcCopy[k] = v
		}
		rlRaw, _ := pc["region_list"].([]interface{})
		rlCopy := make([]interface{}, 0, len(rlRaw))
		for _, prRaw := range rlRaw {
			pr, ok := prRaw.(map[string]interface{})
			if !ok {
				rlCopy = append(rlCopy, prRaw)
				continue
			}
			prCopy := make(map[string]interface{}, len(pr))
			for k, v := range pr {
				prCopy[k] = v
			}
			azRaw, _ := pr["az_list"].([]interface{})
			azCopy := make([]interface{}, 0, len(azRaw))
			for _, azItemRaw := range azRaw {
				az, ok := azItemRaw.(map[string]interface{})
				if !ok {
					azCopy = append(azCopy, azItemRaw)
					continue
				}
				azMap := make(map[string]interface{}, len(az))
				for k, v := range az {
					azMap[k] = v
				}
				azMap["leader_preference"] = 0
				azCopy = append(azCopy, azMap)
			}
			prCopy["az_list"] = azCopy
			rlCopy = append(rlCopy, prCopy)
		}
		pcCopy["region_list"] = rlCopy
		out = append(out, pcCopy)
	}
	return out
}

// validateCloudListDuplicates returns an error if any region code appears more
// than once within a cloud entry's region_list, or if any AZ code appears more
// than once within a region's az_list. Duplicates trigger a cryptic BE error:
// "Duplicate key <uuid> (attempted merging values N and M)".
func validateCloudListDuplicates(clusters []interface{}) error {
	for ci, clRaw := range clusters {
		cl, ok := clRaw.(map[string]interface{})
		if !ok {
			continue
		}
		cloudList, _ := cl["cloud_list"].([]interface{})
		for pi, pcRaw := range cloudList {
			pc, ok := pcRaw.(map[string]interface{})
			if !ok {
				continue
			}
			seenRegion := make(map[string]bool)
			regionList, _ := pc["region_list"].([]interface{})
			for _, prRaw := range regionList {
				pr, ok := prRaw.(map[string]interface{})
				if !ok {
					continue
				}
				regionCode, _ := pr["code"].(string)
				if regionCode == "" {
					continue
				}
				if seenRegion[regionCode] {
					return fmt.Errorf(
						"clusters[%d].cloud_list[%d].region_list: "+
							"duplicate region code %q; "+
							"each region may appear at most once per cloud entry",
						ci, pi, regionCode)
				}
				seenRegion[regionCode] = true

				seenAZ := make(map[string]bool)
				azList, _ := pr["az_list"].([]interface{})
				for _, azRaw := range azList {
					az, ok := azRaw.(map[string]interface{})
					if !ok {
						continue
					}
					azCode, _ := az["code"].(string)
					if azCode == "" {
						continue
					}
					if seenAZ[azCode] {
						return fmt.Errorf(
							"clusters[%d].cloud_list[%d].region_list"+
								"[region %q].az_list: "+
								"duplicate zone code %q; "+
								"each zone may appear at most once per region",
							ci, pi, regionCode, azCode)
					}
					seenAZ[azCode] = true
				}
			}
		}
	}
	return nil
}

// validateCloudListRegionsInUserIntent returns an error when a region listed in
// cloud_list.region_list cannot be found in the corresponding cluster's
// user_intent.region_list. The cloud_list uses region codes (e.g. "us-east-1")
// while user_intent.region_list holds UUIDs; the function fetches the provider
// to map codes to UUIDs before comparing. Provider UUIDs are deduplicated.
//
// Catching this mismatch at plan time replaces the opaque BE error
// "Unable to place replicas, no zones available."
func validateCloudListRegionsInUserIntent(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	clusters []interface{},
) error {
	for ci, clRaw := range clusters {
		cl, ok := clRaw.(map[string]interface{})
		if !ok {
			continue
		}
		cloudList, _ := cl["cloud_list"].([]interface{})
		if len(cloudList) == 0 {
			continue
		}

		uiList, _ := cl["user_intent"].([]interface{})
		if len(uiList) == 0 {
			continue
		}
		ui, ok := uiList[0].(map[string]interface{})
		if !ok {
			continue
		}
		uiRegionRaw, _ := ui["region_list"].([]interface{})
		uiRegionSet := make(map[string]bool, len(uiRegionRaw))
		for _, v := range uiRegionRaw {
			if s, ok := v.(string); ok && s != "" {
				uiRegionSet[s] = true
			}
		}
		// If every UUID in user_intent.region_list is still unknown at plan time
		// (e.g. referencing a concurrently-created provider's computed region UUIDs),
		// the set is empty and we cannot perform the cross-check. Skip rather than
		// produce a false-positive error.
		if len(uiRegionSet) == 0 {
			continue
		}

		seenProvider := make(map[string]map[string]string)
		for pi, pcRaw := range cloudList {
			pc, ok := pcRaw.(map[string]interface{})
			if !ok {
				continue
			}
			provUUID, _ := pc["provider"].(string)
			if provUUID == "" {
				continue
			}
			if _, fetched := seenProvider[provUUID]; !fetched {
				regions, _, err := c.RegionManagementAPI.GetRegion(
					ctx, cUUID, provUUID).Execute()
				if err != nil {
					seenProvider[provUUID] = nil
					continue
				}
				codeToUUID := make(map[string]string, len(regions))
				for _, r := range regions {
					if r.GetCode() != "" && r.GetUuid() != "" {
						codeToUUID[r.GetCode()] = r.GetUuid()
					}
				}
				seenProvider[provUUID] = codeToUUID
			}
			codeToUUID := seenProvider[provUUID]
			if codeToUUID == nil {
				continue
			}

			regionList, _ := pc["region_list"].([]interface{})
			for _, prRaw := range regionList {
				pr, ok := prRaw.(map[string]interface{})
				if !ok {
					continue
				}
				regionCode, _ := pr["code"].(string)
				if regionCode == "" {
					continue
				}
				regionUUID, known := codeToUUID[regionCode]
				if !known {
					// unknown code is caught by validateCloudListAZCodes; skip here
					continue
				}
				if !uiRegionSet[regionUUID] {
					return fmt.Errorf(
						"clusters[%d].cloud_list[%d].region_list: "+
							"region %q (UUID %s) is not listed in "+
							"clusters[%d].user_intent.region_list; "+
							"add %s to user_intent.region_list",
						ci, pi, regionCode, regionUUID, ci, regionUUID)
				}
			}
		}
	}
	return nil
}

// validateCloudListAZCodes returns an error if any region code or AZ code in
// cloudList does not exist in the provider's zone list. Catches two classes of
// mistakes that otherwise produce the cryptic BE error
// "Unable to place replicas, no zones available.":
//  1. A region code listed under cloud_list.region_list that does not exist for
//     the specified provider (e.g. a typo or a region not imported into YBA).
//  2. An AZ code listed under az_list that does not belong to the declared
//     parent region (e.g. us-east-1a placed inside a us-west-2 block).
//
// Uses RegionManagementAPI.GetRegion (same source as fetchProviderZoneFallback)
// for authoritative zone data. Provider UUIDs are deduplicated so each
// provider is fetched at most once.
func validateCloudListAZCodes(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	cloudList []interface{},
) error {
	seenProvider := make(map[string][]client.Region)
	for _, pcRaw := range cloudList {
		pc, ok := pcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		provUUID, _ := pc["provider"].(string)
		if provUUID == "" {
			continue
		}
		if _, fetched := seenProvider[provUUID]; !fetched {
			regions, _, err := c.RegionManagementAPI.GetRegion(ctx, cUUID, provUUID).Execute()
			if err != nil {
				seenProvider[provUUID] = nil
				continue
			}
			seenProvider[provUUID] = regions
		}
		provRegions := seenProvider[provUUID]
		if provRegions == nil {
			continue
		}
		validByRegion := make(map[string]map[string]bool)
		for _, r := range provRegions {
			zones := make(map[string]bool)
			for _, az := range r.GetZones() {
				zones[az.GetName()] = true
				if az.GetCode() != "" {
					zones[az.GetCode()] = true
				}
			}
			validByRegion[r.GetCode()] = zones
		}
		for _, rRaw := range pc["region_list"].([]interface{}) {
			r, ok := rRaw.(map[string]interface{})
			if !ok {
				continue
			}
			regionCode, _ := r["code"].(string)
			if regionCode == "" {
				continue
			}
			validZones, regionKnown := validByRegion[regionCode]
			if !regionKnown {
				var validRegions []string
				for rc := range validByRegion {
					validRegions = append(validRegions, rc)
				}
				return fmt.Errorf(
					"region %q does not exist for provider %s; "+
						"valid regions: %s",
					regionCode, provUUID,
					strings.Join(validRegions, ", "))
			}
			for _, azRaw := range r["az_list"].([]interface{}) {
				az, ok := azRaw.(map[string]interface{})
				if !ok {
					continue
				}
				azCode, _ := az["code"].(string)
				if azCode == "" {
					continue
				}
				if !validZones[azCode] {
					var valid []string
					for z := range validZones {
						valid = append(valid, z)
					}
					return fmt.Errorf(
						"zone %q does not exist in region %q for "+
							"provider %s; valid zones in that region: %s",
						azCode, regionCode, provUUID,
						strings.Join(valid, ", "))
				}
			}
		}
	}
	return nil
}

// azFallbackAttrs holds provider-fetched UUID and subnet for an AZ, used to
// backfill both fields when a new AZ is introduced in a placement edit.
type azFallbackAttrs struct {
	uuid            string
	subnet          string
	secondarySubnet string
}

// resolveAZUUIDs fixes the UUID on every PlacementRegion and PlacementAZ in
// newPI by matching on Code/Name against oldCloudList (live API state).
// TypeList index-shifting can assign a removed element's UUID to a kept one;
// this corrects it by code/name lookup. For AZs not present in the live
// placement (new AZ, new region, or new provider), the fallback maps built
// from the provider's zone list are used to supply the correct UUID and subnet.
func resolveAZUUIDs(
	newPI *client.PlacementInfo,
	oldCloudList []client.PlacementCloud,
	fallbackByRegionCode map[string]string,
	fallbackByAZCode map[string]string,
	fallbackByAZAttrs map[string]azFallbackAttrs,
) {
	if newPI == nil {
		return
	}
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
						// New AZ: override UUID and subnet from provider data.
						// State values at this TypeList index belong to the old AZ.
						az.Uuid = utils.GetStringPointer(uuid)
						if fallbackByAZAttrs != nil {
							if attrs, ok := fallbackByAZAttrs[az.GetName()]; ok {
								if attrs.subnet != "" {
									az.Subnet = utils.GetStringPointer(attrs.subnet)
								}
								if attrs.secondarySubnet != "" {
									az.SecondarySubnet = utils.GetStringPointer(attrs.secondarySubnet)
								}
							}
						}
					}
				}
			}
		}
	}
}

// buildProviderZoneFallback populates region/AZ code->UUID maps and subnet
// attrs from a provider's region list. Multiple calls merge into the same maps.
func buildProviderZoneFallback(
	regions []client.Region,
	byRegionCode map[string]string,
	byAZCode map[string]string,
	byAZAttrs map[string]azFallbackAttrs,
) {
	for _, r := range regions {
		if r.GetCode() != "" && r.GetUuid() != "" {
			byRegionCode[r.GetCode()] = r.GetUuid()
		}
		for _, az := range r.GetZones() {
			// AvailabilityZone.Name is the zone code (e.g. "us-west-2a").
			if az.GetName() != "" && az.GetUuid() != "" {
				byAZCode[az.GetName()] = az.GetUuid()
				if byAZAttrs != nil {
					byAZAttrs[az.GetName()] = azFallbackAttrs{
						uuid:            az.GetUuid(),
						subnet:          az.GetSubnet(),
						secondarySubnet: az.GetSecondarySubnet(),
					}
				}
			}
		}
	}
}

// fetchProviderZoneFallback fetches region/zone data for every distinct
// provider UUID across both oldCloudList (live placement) and newCloudList
// (desired config). Including newCloudList handles a changed cloud_list.provider
// whose zones would not appear in oldCloudList at all. Provider UUIDs are
// deduplicated so each provider is fetched at most once.
func fetchProviderZoneFallback(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	oldCloudList []client.PlacementCloud,
	newCloudList []client.PlacementCloud,
) (byRegionCode map[string]string, byAZCode map[string]string,
	byAZAttrs map[string]azFallbackAttrs) {
	byRegionCode = make(map[string]string)
	byAZCode = make(map[string]string)
	byAZAttrs = make(map[string]azFallbackAttrs)
	seen := make(map[string]bool)
	for _, cloud := range append(oldCloudList, newCloudList...) {
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
		buildProviderZoneFallback(provRegions, byRegionCode, byAZCode, byAZAttrs)
	}
	return byRegionCode, byAZCode, byAZAttrs
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
	intent := client.UserIntent{
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
	// dedicated_masters block presence drives DedicatedNodes.
	// An empty block means: dedicated mode, fall back to TServer instance/device.
	// Terraform SDK v2 may pass []interface{}{nil} for an empty block that has
	// only optional fields, so guard against a nil first element.
	dmList, _ := ui["dedicated_masters"].([]interface{})
	if len(dmList) > 0 {
		intent.DedicatedNodes = utils.GetBoolPointer(true)
		var dm map[string]interface{}
		if dmList[0] != nil {
			dm = dmList[0].(map[string]interface{})
		} else {
			dm = make(map[string]interface{})
		}
		// instance_type: explicit override or fall back to TServer instance type.
		if v, ok := dm["instance_type"].(string); ok && v != "" {
			intent.MasterInstanceType = utils.GetStringPointer(v)
		} else {
			intent.MasterInstanceType = intent.InstanceType
		}
		// device_info: start from a copy of the TServer device info so every
		// field is pre-populated with a sensible default, then overwrite only
		// the fields the user explicitly provided (non-zero / non-empty values).
		if diList, ok := dm["device_info"].([]interface{}); ok && len(diList) > 0 {
			// Shallow-copy the TServer DeviceInfo as the base. Pointer fields
			// are replaced below, never mutated, so the TServer intent stays clean.
			mdi := &client.DeviceInfo{}
			if tdi := intent.DeviceInfo; tdi != nil {
				*mdi = *tdi
			}
			explicit := buildDeviceInfo(utils.MapFromSingletonList(diList))
			if explicit.GetNumVolumes() != 0 {
				mdi.NumVolumes = explicit.NumVolumes
			}
			if explicit.GetVolumeSize() != 0 {
				mdi.VolumeSize = explicit.VolumeSize
			}
			if explicit.GetStorageType() != "" {
				mdi.StorageType = explicit.StorageType
			}
			if explicit.GetDiskIops() != 0 {
				mdi.DiskIops = explicit.DiskIops
			}
			if explicit.GetThroughput() != 0 {
				mdi.Throughput = explicit.Throughput
			}
			if explicit.GetMountPoints() != "" {
				mdi.MountPoints = explicit.MountPoints
			}
			intent.MasterDeviceInfo = mdi
		} else {
			intent.MasterDeviceInfo = intent.DeviceInfo
		}
	} else {
		intent.DedicatedNodes = utils.GetBoolPointer(false)
	}
	return intent
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

// redistributeNodesInAZs distributes totalNodes evenly across all AZs in pi,
// updating NumNodesInAZ on each AZ. The first (totalNodes % numAZs) AZs each
// receive one extra node; the rest receive the base count.
//
// This is used when cloud_list is absent from the Terraform config but
// num_nodes changes: YBA's EditUniverse handler calls isSamePlacement() to
// detect whether a change exists. If PlacementInfo still has the old per-AZ
// counts it reports "same placement" and returns "No changes that could be
// applied by EditUniverse". Updating the counts here makes isSamePlacement()
// return false so the UPDATE option is recognised. UserAZSelected stays false,
// so YBA auto-computes the final per-node placement during task execution.
func redistributeNodesInAZs(pi *client.PlacementInfo, totalNodes int) {
	var azPtrs []*client.PlacementAZ
	for ci := range pi.CloudList {
		for ri := range pi.CloudList[ci].RegionList {
			for ai := range pi.CloudList[ci].RegionList[ri].AzList {
				azPtrs = append(azPtrs, &pi.CloudList[ci].RegionList[ri].AzList[ai])
			}
		}
	}
	if len(azPtrs) == 0 {
		return
	}
	// Clamp base to 1 so every AZ receives NumNodesInAZ >= 1. With
	// UserAZSelected=false YBA recomputes the final placement, so the exact
	// counts here only need to (a) differ from the prior per-AZ counts so
	// isSamePlacement() returns false and (b) satisfy any present-or-future
	// YBA-side numNodesInAZ >= 1 invariant. Without the clamp, totalNodes <
	// numAZs would yield zero-node AZs.
	base := totalNodes / len(azPtrs)
	extra := totalNodes % len(azPtrs)
	if base == 0 {
		base = 1
		extra = 0
	}
	for j, az := range azPtrs {
		count := int32(base)
		if j < extra {
			count++
		}
		az.NumNodesInAZ = &count
	}
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
			DedicatedTo:           v.DedicatedTo,
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
