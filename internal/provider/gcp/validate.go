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
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
)

// regionData holds user-provided region fields for comparison
type regionData struct {
	sharedSubnet     string
	instanceTemplate string
}

// validateNoDuplicateRegions is the CustomizeDiff function for GCP providers.
// GCP zones are auto-discovered by YBA, so no zone duplicate validation is needed.
func validateNoDuplicateRegions(
	ctx context.Context,
	d *schema.ResourceDiff,
	meta interface{},
) error {
	if err := providerutil.MarkVersionComputedIfChanged(ctx, d,
		[]string{
			"credentials", "use_host_credentials", "project_id",
			"shared_vpc_project_id", "network", "use_host_vpc", "create_vpc",
			"yb_firewall_tags",
		},
		regionsContentChanged,
		nil, // GCP image bundles have no region_overrides; no region filtering needed
	); err != nil {
		return err
	}

	if err := providerutil.ValidateAtLeastOneImageBundle(d); err != nil {
		return err
	}

	// ValidateImageBundles runs first so a "multiple defaults for arch X" config produces
	// the clearer "at most one bundle per arch can be the default" message rather than
	// the more confusing "new bundle cannot have use_as_default=true" message from
	// ValidateNewBundlesNotDefault, which would otherwise fire first.
	if err := providerutil.ValidateImageBundles(d); err != nil {
		return err
	}

	if err := providerutil.ValidateNewBundlesNotDefault(d); err != nil {
		return err
	}

	regionsRaw, _ := d.Get("regions").([]interface{})

	regionCodes := make(map[string]bool)

	for _, r := range regionsRaw {
		regionMap := r.(map[string]interface{})
		regionCode := regionMap["code"].(string)

		if regionCodes[regionCode] {
			return fmt.Errorf(
				"duplicate region code %q found: each region must have a unique code",
				regionCode,
			)
		}
		regionCodes[regionCode] = true
	}

	return nil
}

// regionsContentChanged reports whether two region lists differ in content (ignoring order).
func regionsContentChanged(oldRaw, newRaw interface{}) bool {
	oldRegions := extractRegionData(oldRaw)
	newRegions := extractRegionData(newRaw)

	if len(oldRegions) != len(newRegions) {
		return true
	}

	for regionCode, oldRegion := range oldRegions {
		newRegion, exists := newRegions[regionCode]
		if !exists {
			return true
		}
		if oldRegion.sharedSubnet != newRegion.sharedSubnet ||
			oldRegion.instanceTemplate != newRegion.instanceTemplate {
			return true
		}
	}

	return false
}

// extractRegionData extracts user-provided fields from regions for comparison.
func extractRegionData(regionsRaw interface{}) map[string]regionData {
	result := make(map[string]regionData)

	regions, _ := regionsRaw.([]interface{})

	for _, r := range regions {
		regionMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		regionCode, _ := regionMap["code"].(string)
		if regionCode == "" {
			continue
		}

		rd := regionData{
			sharedSubnet:     providerutil.GetString(regionMap, "shared_subnet"),
			instanceTemplate: providerutil.GetString(regionMap, "instance_template"),
		}

		result[regionCode] = rd
	}

	return result
}

// suppressIfGCPRegionsPureReorder suppresses positional diffs when regions are only reordered.
func suppressIfGCPRegionsPureReorder(k, old, new string, d *schema.ResourceData) bool {
	if old == new {
		return true
	}
	o, n := d.GetChange("regions")
	return !regionsContentChanged(o, n)
}
