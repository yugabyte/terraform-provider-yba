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

package gcp

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// buildGCPCloudInfo builds GCP cloud info from schema
// Mirrors yba-cli: gcpCloudInfo construction in create_provider.go
func buildGCPCloudInfo(d *schema.ResourceData) (*client.GCPCloudInfo, error) {
	gcpCloudInfo := &client.GCPCloudInfo{}

	useHostCredentials := d.Get("use_host_credentials").(bool)
	gcpCloudInfo.SetUseHostCredentials(useHostCredentials)

	// Set firewall tags if provided
	if v, ok := d.GetOk("yb_firewall_tags"); ok {
		gcpCloudInfo.SetYbFirewallTags(v.(string))
	}

	// Set project IDs
	if v, ok := d.GetOk("project_id"); ok {
		gcpCloudInfo.SetGceProject(v.(string))
	}
	if v, ok := d.GetOk("shared_vpc_project_id"); ok {
		gcpCloudInfo.SetSharedVPCProject(v.(string))
	}

	// Handle VPC settings (mirrors yba-cli logic)
	// See: yugabyte-db/managed/yba-cli/cmd/provider/gcp/create_provider.go
	// API UseHostVPC is true for both "use YBA host VPC" and "use existing custom VPC"
	// It's only false when creating a new VPC
	// The difference is whether DestVpcId (network) is set
	useHostVPC := d.Get("use_host_vpc").(bool)
	createVPC := d.Get("create_vpc").(bool)
	network := d.Get("network").(string)

	if createVPC && useHostVPC {
		return nil, fmt.Errorf("create_vpc and use_host_vpc cannot both be true")
	}

	if createVPC {
		// Creating new VPC - UseHostVPC must be false
		gcpCloudInfo.SetUseHostVPC(false)
		if network == "" {
			return nil, fmt.Errorf("network is required when create_vpc is true")
		}
		gcpCloudInfo.SetDestVpcId(network)
	} else {
		// Using existing VPC (either YBA host's or custom) - UseHostVPC is always true
		gcpCloudInfo.SetUseHostVPC(true)
		if !useHostVPC {
			// Using custom existing VPC - network is required
			if network == "" {
				return nil, fmt.Errorf("network is required when use_host_vpc is false")
			}
			gcpCloudInfo.SetDestVpcId(network)
		}
		// If useHostVPC is true, don't set DestVpcId - use YBA host's VPC
	}

	// Get credentials if not using host credentials
	if !useHostCredentials {
		credentials := d.Get("credentials").(string)
		if credentials == "" {
			return nil, fmt.Errorf("GCP credentials required: set credentials " +
				"or use use_host_credentials=true")
		}
		gcpCloudInfo.SetGceApplicationCredentials(credentials)
	}

	return gcpCloudInfo, nil
}

// buildGCPAccessKeys builds access keys for GCP provider.
// Returns nil when both ssh_keypair_name and ssh_private_key_content are empty,
// which causes allAccessKeys to be omitted from the request and lets YBA generate
// a managed keypair - matching UI behavior for the YBA-managed mode.
func buildGCPAccessKeys(d *schema.ResourceData) []client.AccessKey {
	keyPairName := d.Get("ssh_keypair_name").(string)
	sshContent := d.Get("ssh_private_key_content").(string)

	if keyPairName == "" && sshContent == "" {
		return nil
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

// buildGCPRegions builds GCP regions from schema
// Mirrors yba-cli pattern: create zone with shared_subnet, YBA auto-discovers zone names
func buildGCPRegions(regions []interface{}) []client.Region {
	result := make([]client.Region, 0)

	for _, r := range regions {
		regionMap := r.(map[string]interface{})
		regionCode := regionMap["code"].(string)

		zones := buildGCPZones(regionCode, regionMap)

		region := client.Region{
			Code:  utils.GetStringPointer(regionCode),
			Name:  utils.GetStringPointer(regionCode),
			Zones: zones,
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Gcp: &client.GCPRegionCloudInfo{},
				},
			},
		}

		// Set GCP-specific region fields
		if v, ok := regionMap["instance_template"]; ok && v.(string) != "" {
			region.Details.CloudInfo.Gcp.SetInstanceTemplate(v.(string))
		}

		// Include UUID for existing regions (needed for updates)
		if uuid, ok := regionMap["uuid"].(string); ok && uuid != "" {
			region.Uuid = utils.GetStringPointer(uuid)
		}

		result = append(result, region)
	}

	return result
}

// buildGCPZones builds zones for a region using shared_subnet
// Mirrors yba-cli: only sets subnet, YBA auto-discovers zone names
func buildGCPZones(regionCode string, regionMap map[string]interface{}) []client.AvailabilityZone {
	sharedSubnet := ""
	if v, ok := regionMap["shared_subnet"]; ok && v != nil {
		sharedSubnet = v.(string)
	}

	zone := client.AvailabilityZone{
		Code:   utils.GetStringPointer(regionCode),
		Name:   regionCode,
		Subnet: utils.GetStringPointer(sharedSubnet),
	}

	return []client.AvailabilityZone{zone}
}

// flattenGCPRegions converts API regions to schema format
// Returns all zones from API - YBA auto-discovers zones
func flattenGCPRegions(regions []client.Region) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, region := range regions {
		zones := region.GetZones()

		// Get shared_subnet from first zone (all zones in region have same subnet)
		sharedSubnet := ""
		if len(zones) > 0 {
			sharedSubnet = zones[0].GetSubnet()
		}

		r := map[string]interface{}{
			"uuid":          region.GetUuid(),
			"code":          region.GetCode(),
			"name":          region.GetName(),
			"shared_subnet": sharedSubnet,
			"zones":         flattenGCPZones(zones),
		}

		// Extract GCP-specific region info
		details := region.GetDetails()
		cloudInfo := details.GetCloudInfo()
		gcpInfo := cloudInfo.GetGcp()
		r["instance_template"] = gcpInfo.GetInstanceTemplate()

		result = append(result, r)
	}

	return result
}

// flattenGCPZones converts API zones to schema format
func flattenGCPZones(zones []client.AvailabilityZone) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, zone := range zones {
		z := map[string]interface{}{
			"uuid":   zone.GetUuid(),
			"code":   zone.GetCode(),
			"name":   zone.GetName(),
			"subnet": zone.GetSubnet(),
		}
		result = append(result, z)
	}

	return result
}

// ImageBundleType constants - re-exported from providerutil for local use
const (
	ImageBundleTypeYBAActive = providerutil.ImageBundleTypeYBAActive
)

// mergeRegionUUIDs merges UUIDs from old state into new config regions.
// Works with TypeList ([]interface{}).
// For existing GCP regions, actual zones from state are reused (with updated subnet)
// to avoid sending placeholder zones with null codes, which the backend rejects as
// "Duplicate AZ code null". Only newly added regions use a placeholder zone so that
// YBA can auto-discover zones for them.
func mergeRegionUUIDs(
	oldRegions []interface{},
	newRegions []interface{},
) []client.Region {
	// Build a map of old regions by code to get UUIDs
	oldByCode := make(map[string]map[string]interface{})
	for _, r := range oldRegions {
		regionMap := r.(map[string]interface{})
		code := regionMap["code"].(string)
		oldByCode[code] = regionMap
	}

	// Track which old regions are still in config
	newRegionCodes := make(map[string]bool)

	result := make([]client.Region, 0, len(newRegions))

	for _, nr := range newRegions {
		newMap := nr.(map[string]interface{})
		regionCode := newMap["code"].(string)
		newRegionCodes[regionCode] = true

		var zones []client.AvailabilityZone
		if oldRegion, exists := oldByCode[regionCode]; exists {
			// Existing region: reuse actual zones from state so the backend receives
			// proper zone codes/UUIDs rather than a null-code placeholder.
			zones = buildGCPZonesFromState(oldRegion, newMap)
		} else {
			// New region: send placeholder zone so YBA auto-discovers zones.
			zones = buildGCPZones(regionCode, newMap)
		}

		// Build the region
		region := client.Region{
			Code:  utils.GetStringPointer(regionCode),
			Name:  utils.GetStringPointer(regionCode),
			Zones: zones,
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Gcp: &client.GCPRegionCloudInfo{},
				},
			},
		}

		// Set GCP-specific region fields
		if v, ok := newMap["instance_template"]; ok && v.(string) != "" {
			region.Details.CloudInfo.Gcp.SetInstanceTemplate(v.(string))
		}

		// If this region exists in old state, copy UUID
		if oldRegion, exists := oldByCode[regionCode]; exists {
			if uuid, ok := oldRegion["uuid"].(string); ok && uuid != "" {
				region.Uuid = utils.GetStringPointer(uuid)
			}
		}

		result = append(result, region)
	}

	// Handle removed regions - mark them as inactive (like yba-cli does).
	// We must include the existing zones from state so the GCP backend validator can
	// access zone subnet data (region.getZones().get(0)) without a NullPointerException.
	for code, oldRegion := range oldByCode {
		if !newRegionCodes[code] {
			uuid, _ := oldRegion["uuid"].(string)
			gcpInfo := &client.GCPRegionCloudInfo{}
			if v, ok := oldRegion["instance_template"].(string); ok && v != "" {
				gcpInfo.SetInstanceTemplate(v)
			}
			region := client.Region{
				Code:   utils.GetStringPointer(code),
				Name:   utils.GetStringPointer(code),
				Active: utils.GetBoolPointer(false),
				Zones:  buildGCPZonesFromState(oldRegion, oldRegion),
				Details: &client.RegionDetails{
					CloudInfo: &client.RegionCloudInfo{
						Gcp: gcpInfo,
					},
				},
			}
			if uuid != "" {
				region.Uuid = utils.GetStringPointer(uuid)
			}
			result = append(result, region)
		}
	}

	return result
}

// buildGCPZonesFromState constructs zones for an existing region using the actual zone
// data stored in Terraform state (real codes, names, and UUIDs from GCP auto-discovery),
// while applying the subnet from the new config. Falls back to a placeholder zone when
// state has no zones (e.g. immediately after import before a refresh).
func buildGCPZonesFromState(
	oldRegion map[string]interface{},
	newRegion map[string]interface{},
) []client.AvailabilityZone {
	newSubnet := ""
	if v, ok := newRegion["shared_subnet"]; ok && v != nil {
		newSubnet = v.(string)
	}

	regionCode, _ := newRegion["code"].(string)

	oldZonesRaw, ok := oldRegion["zones"]
	if !ok || oldZonesRaw == nil {
		return buildGCPZones(regionCode, newRegion)
	}
	oldZones, ok := oldZonesRaw.([]interface{})
	if !ok || len(oldZones) == 0 {
		return buildGCPZones(regionCode, newRegion)
	}

	zones := make([]client.AvailabilityZone, 0, len(oldZones))
	for _, z := range oldZones {
		zoneMap, ok := z.(map[string]interface{})
		if !ok {
			continue
		}
		zone := client.AvailabilityZone{
			Subnet: utils.GetStringPointer(newSubnet),
		}
		if uuid, ok := zoneMap["uuid"].(string); ok && uuid != "" {
			zone.Uuid = utils.GetStringPointer(uuid)
		}
		if code, ok := zoneMap["code"].(string); ok && code != "" {
			zone.Code = utils.GetStringPointer(code)
		}
		if name, ok := zoneMap["name"].(string); ok && name != "" {
			zone.Name = name
		}
		zones = append(zones, zone)
	}

	if len(zones) == 0 {
		return buildGCPZones(regionCode, newRegion)
	}
	return zones
}
