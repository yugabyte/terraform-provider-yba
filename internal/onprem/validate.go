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
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// validateOnPremProvider is the CustomizeDiff function for On-Premises providers.
func validateOnPremProvider(
	ctx context.Context,
	d *schema.ResourceDiff,
	meta interface{},
) error {
	if d.Id() != "" {
		hasRealChange := false

		simpleFields := []string{
			"name", "ssh_user", "ssh_port", "skip_provisioning", "passwordless_sudo_access",
			"air_gap_install", "install_node_exporter", "node_exporter_user", "node_exporter_port",
			"ntp_servers", "set_up_chrony", "provision_instance_script", "yb_home_dir",
			"use_clockbound", "ssh_keypair_name", "ssh_private_key_content",
		}
		for _, field := range simpleFields {
			if d.HasChange(field) {
				hasRealChange = true
				break
			}
		}

		if d.HasChange("regions") {
			oldRaw, newRaw := d.GetChange("regions")
			if onpremRegionsContentChanged(oldRaw, newRaw) {
				hasRealChange = true
			}
		}

		if hasRealChange {
			if err := d.SetNewComputed("version"); err != nil {
				return err
			}
		}
	}

	regionsRaw, _ := d.Get("regions").([]interface{})

	regionCodes := make(map[string]bool)
	for _, r := range regionsRaw {
		regionMap := r.(map[string]interface{})
		regionCode, _ := regionMap["code"].(string)
		if regionCode == "" {
			regionCode, _ = regionMap["name"].(string)
		}
		if regionCode == "" {
			continue
		}

		if regionCodes[regionCode] {
			return fmt.Errorf(
				"duplicate region code %q found: each region must have a unique code",
				regionCode,
			)
		}
		regionCodes[regionCode] = true

		zonesList, _ := regionMap["zones"].([]interface{})
		zoneCodes := make(map[string]bool)
		for _, z := range zonesList {
			zoneMap := z.(map[string]interface{})
			zoneCode, _ := zoneMap["code"].(string)
			if zoneCode == "" {
				zoneCode, _ = zoneMap["name"].(string)
			}
			if zoneCode == "" {
				continue
			}
			if zoneCodes[zoneCode] {
				return fmt.Errorf(
					"duplicate zone code %q found in region %q: "+
						"each zone within a region must have a unique code",
					zoneCode, regionCode,
				)
			}
			zoneCodes[zoneCode] = true
		}
	}

	return nil
}

// onpremRegionsContentChanged reports whether two onprem region lists differ in content,
// ignoring order.
func onpremRegionsContentChanged(oldRaw, newRaw interface{}) bool {
	oldRegions := extractOnpremRegionData(oldRaw)
	newRegions := extractOnpremRegionData(newRaw)

	if len(oldRegions) != len(newRegions) {
		return true
	}

	for code, old := range oldRegions {
		nw, exists := newRegions[code]
		if !exists {
			return true
		}
		if old.latitude != nw.latitude || old.longitude != nw.longitude {
			return true
		}
		if len(old.zones) != len(nw.zones) {
			return true
		}
		for z := range old.zones {
			if _, exists := nw.zones[z]; !exists {
				return true
			}
		}
	}
	return false
}

type onpremRegionEntry struct {
	latitude  float64
	longitude float64
	zones     map[string]struct{}
}

// extractOnpremRegionData extracts user-specified fields from onprem regions for comparison.
func extractOnpremRegionData(regionsRaw interface{}) map[string]onpremRegionEntry {
	result := make(map[string]onpremRegionEntry)

	regions, _ := regionsRaw.([]interface{})
	for _, r := range regions {
		regionMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := regionMap["code"].(string)
		if code == "" {
			code, _ = regionMap["name"].(string)
		}
		if code == "" {
			continue
		}

		lat, _ := regionMap["latitude"].(float64)
		lon, _ := regionMap["longitude"].(float64)

		zones := make(map[string]struct{})
		if zonesRaw, ok := regionMap["zones"].([]interface{}); ok {
			for _, z := range zonesRaw {
				zoneMap, ok := z.(map[string]interface{})
				if !ok {
					continue
				}
				zoneCode, _ := zoneMap["code"].(string)
				if zoneCode == "" {
					zoneCode, _ = zoneMap["name"].(string)
				}
				if zoneCode != "" {
					zones[zoneCode] = struct{}{}
				}
			}
		}

		result[code] = onpremRegionEntry{
			latitude:  lat,
			longitude: lon,
			zones:     zones,
		}
	}

	return result
}

// suppressIfOnpremRegionsPureReorder suppresses positional diffs when regions are only reordered.
func suppressIfOnpremRegionsPureReorder(k, old, new string, d *schema.ResourceData) bool {
	if old == new {
		return true
	}
	o, n := d.GetChange("regions")
	return !onpremRegionsContentChanged(o, n)
}
