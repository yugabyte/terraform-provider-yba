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

package oci

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ociProviderMin is the first build on each line that contains yugabyte-db
// commit 169bfbc89d ([PLAT-20423] Add oci provider).
var ociProviderMin = utils.YBAMinimumVersion{
	Stable:  "2026.1.2.0-b29",
	Preview: "2.31.0.0-b133",
}

func checkOCIProviderVersion(ctx context.Context, version string) error {
	if version == "" {
		return nil
	}
	ok, applied, err := utils.MeetsMinimum(version, ociProviderMin)
	switch {
	case err != nil:
		tflog.Warn(ctx, "cannot parse the YBA version; skipping the minimum-version check",
			map[string]interface{}{"version": version, "check": "yba_oci_provider"})
	case !ok:
		return fmt.Errorf(
			"yba_oci_provider requires YugabyteDB Anywhere %s or later "+
				"(stable: %s, preview: %s); the target YBA reports %s",
			applied, ociProviderMin.Stable, ociProviderMin.Preview, version)
	}
	return nil
}

func validateOCIVersion(ctx context.Context, meta interface{}) error {
	c, ok := meta.(*api.APIClient)
	if !ok || c == nil {
		return nil
	}
	version, err := c.AppVersion(ctx)
	if err != nil {
		return err
	}
	return checkOCIProviderVersion(ctx, version)
}

func checkImageBundleRegionCoverage(bundlesRaw []interface{}, regionCodes []string) error {
	for _, b := range bundlesRaw {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := bundleMap["name"].(string)
		details, ok := bundleMap["details"].([]interface{})
		if !ok || len(details) == 0 {
			continue
		}
		det, ok := details[0].(map[string]interface{})
		if !ok {
			continue
		}
		overrides, _ := det["region_overrides"].(map[string]interface{})
		for _, regionCode := range regionCodes {
			image, exists := overrides[regionCode]
			imageStr, _ := image.(string)
			if !exists || imageStr == "" {
				return fmt.Errorf(
					"image bundle %q must specify a non-empty image for region %q in "+
						"region_overrides: all custom image bundles must provide a valid "+
						"image for every region configured in the provider "+
						"(empty string \"\" is not accepted)",
					name, regionCode)
			}
		}
	}
	return nil
}

func collectRegionCodes(regionsRaw []interface{}) []string {
	codes := make([]string, 0, len(regionsRaw))
	for _, r := range regionsRaw {
		regionMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := regionMap["code"].(string)
		if code != "" {
			codes = append(codes, code)
		}
	}
	return codes
}

func checkDuplicateRegionsAndZones(regionsRaw []interface{}) error {
	regionCodes := make(map[string]bool, len(regionsRaw))
	for _, r := range regionsRaw {
		regionMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		regionCode, _ := regionMap["code"].(string)
		if regionCode != "" {
			if regionCodes[regionCode] {
				return fmt.Errorf(
					"duplicate region code %q found: each region must have a unique code",
					regionCode)
			}
			regionCodes[regionCode] = true
		}
		zonesList, _ := regionMap["zones"].([]interface{})
		zoneCodes := make(map[string]bool, len(zonesList))
		for _, z := range zonesList {
			zoneMap, ok := z.(map[string]interface{})
			if !ok {
				continue
			}
			zoneCode, _ := zoneMap["code"].(string)
			if zoneCode == "" {
				continue
			}
			if zoneCodes[zoneCode] {
				return fmt.Errorf(
					"duplicate zone code %q found in region %q: "+
						"each zone within a region must have a unique code",
					zoneCode, regionCode)
			}
			zoneCodes[zoneCode] = true
		}
	}
	return nil
}

func checkDuplicateManagedArch(bundles []interface{}) error {
	seen := make(map[string]bool, len(bundles))
	for _, b := range bundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		arch, _ := bundleMap["arch"].(string)
		if arch == "" {
			continue
		}
		if seen[arch] {
			return fmt.Errorf(
				"duplicate architecture %q in yba_managed_image_bundles: "+
					"each architecture must appear at most once", arch)
		}
		seen[arch] = true
	}
	return nil
}

type regionData struct {
	vcnID                   string
	instanceConfigurationID string
	zones                   map[string]string
}

func regionsContentChanged(oldRaw, newRaw interface{}) bool {
	oldRegions := extractRegionData(oldRaw)
	newRegions := extractRegionData(newRaw)
	if len(oldRegions) != len(newRegions) {
		return true
	}
	for code, oldRegion := range oldRegions {
		newRegion, exists := newRegions[code]
		if !exists || oldRegion.vcnID != newRegion.vcnID ||
			oldRegion.instanceConfigurationID != newRegion.instanceConfigurationID ||
			len(oldRegion.zones) != len(newRegion.zones) {
			return true
		}
		for zoneCode, subnet := range oldRegion.zones {
			if newRegion.zones[zoneCode] != subnet {
				return true
			}
		}
	}
	return false
}

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
			vcnID:                   providerutil.GetString(regionMap, "vcn_id"),
			instanceConfigurationID: providerutil.GetString(regionMap, "instance_configuration_id"),
			zones:                   map[string]string{},
		}
		zones, _ := regionMap["zones"].([]interface{})
		for _, z := range zones {
			zoneMap, ok := z.(map[string]interface{})
			if !ok {
				continue
			}
			zoneCode, _ := zoneMap["code"].(string)
			if zoneCode == "" {
				continue
			}
			rd.zones[zoneCode] = providerutil.GetString(zoneMap, "subnet")
		}
		result[regionCode] = rd
	}
	return result
}

func suppressIfOCIRegionsPureReorder(k, old, new string, d *schema.ResourceData) bool {
	if old == new {
		return true
	}
	o, n := d.GetChange("regions")
	return !regionsContentChanged(o, n)
}

func suppressInactiveRegionOverride(k, old, new string, d *schema.ResourceData) bool {
	if old != "" {
		return false
	}
	parts := strings.Split(k, ".")
	if len(parts) == 0 {
		return false
	}
	regionCode := parts[len(parts)-1]
	regionsRaw, _ := d.Get("regions").([]interface{})
	for _, r := range regionsRaw {
		regionMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		if code, _ := regionMap["code"].(string); code == regionCode {
			return false
		}
	}
	return true
}

// authField is one API-key argument as CustomizeDiff sees it.
// known is false while the value is still unknown (computed in the same apply).
type authField struct {
	known bool
	value string
}

var ociAPIKeyFieldNames = []string{
	"tenancy_id", "user_id", "fingerprint",
	"private_key_content", "private_key_file_path",
}

// ociAuthPlanError rejects a known-invalid auth combination.
// An unknown value is not treated as empty. Apply calls
// validateOCIAuthInput from setOCIAuthForCreate and from the API-key
// branch of setOCIAuthForUpdate. An explicit use_instance_principal = false
// is API-key auth.
func ociAuthPlanError(principalKnown, principal bool, fields map[string]authField) error {
	if !principalKnown {
		return nil
	}
	if principal {
		for _, name := range ociAPIKeyFieldNames {
			f := fields[name]
			if !f.known {
				continue
			}
			if f.value != "" {
				return fmt.Errorf(
					"%s must be omitted when use_instance_principal is true", name)
			}
		}
		return nil
	}
	for _, name := range []string{"tenancy_id", "user_id", "fingerprint"} {
		f, ok := fields[name]
		if !ok || !f.known {
			continue
		}
		if f.value == "" {
			return fmt.Errorf(
				"%s is required when use_instance_principal is false", name)
		}
	}
	content, contentOK := fields["private_key_content"]
	path, pathOK := fields["private_key_file_path"]
	if contentOK && content.known && pathOK && path.known {
		if (content.value == "") == (path.value == "") {
			return fmt.Errorf(
				"private_key_content and private_key_file_path: set exactly one")
		}
	}
	return nil
}

func ociAuthPlanErrorFromDiff(d *schema.ResourceDiff) error {
	fields := make(map[string]authField, len(ociAPIKeyFieldNames))
	for _, name := range ociAPIKeyFieldNames {
		value := ""
		if d.NewValueKnown(name) {
			value, _ = d.Get(name).(string)
		}
		fields[name] = authField{known: d.NewValueKnown(name), value: value}
	}
	principal := false
	if d.NewValueKnown("use_instance_principal") {
		principal, _ = d.Get("use_instance_principal").(bool)
	}
	return ociAuthPlanError(d.NewValueKnown("use_instance_principal"), principal, fields)
}

func validatePrivateKeyFile(d *schema.ResourceDiff) error {
	if !d.NewValueKnown("private_key_file_path") {
		return nil
	}
	if !d.NewValueKnown("use_instance_principal") {
		return nil
	}
	if d.Get("use_instance_principal").(bool) {
		return nil
	}
	path := d.Get("private_key_file_path").(string)
	if path == "" {
		return nil
	}
	expanded, err := expandHome(path)
	if err != nil {
		return err
	}
	if err := utils.FileExist(expanded); err != nil {
		return fmt.Errorf("private_key_file_path: %w", err)
	}
	return nil
}

func validateOCIProvider(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	if err := validateOCIVersion(ctx, meta); err != nil {
		return err
	}
	if err := ociAuthPlanErrorFromDiff(d); err != nil {
		return err
	}
	if err := validatePrivateKeyFile(d); err != nil {
		return err
	}

	regionsRaw, _ := d.Get("regions").([]interface{})
	activeRegions := make(map[string]bool, len(regionsRaw))
	for _, code := range collectRegionCodes(regionsRaw) {
		activeRegions[code] = true
	}
	if err := providerutil.MarkVersionComputedIfChanged(ctx, d,
		[]string{
			"use_instance_principal", "tenancy_id", "user_id", "fingerprint",
			"private_key_content", "private_key_file_path", "compartment_id",
			"default_region", "hosted_zone_id", "ssh_keypair_name",
			"ssh_private_key_content",
		},
		regionsContentChanged,
		activeRegions,
	); err != nil {
		return err
	}
	if err := providerutil.ValidateAtLeastOneImageBundle(d); err != nil {
		return err
	}
	if err := providerutil.ValidateImageBundles(d); err != nil {
		return err
	}
	if err := providerutil.ValidateNewBundlesNotDefault(d); err != nil {
		return err
	}
	if err := checkDuplicateRegionsAndZones(regionsRaw); err != nil {
		return err
	}
	if ybaBundles, ok := d.Get("yba_managed_image_bundles").([]interface{}); ok {
		if err := checkDuplicateManagedArch(ybaBundles); err != nil {
			return err
		}
	}
	bundlesRaw, _ := d.Get("image_bundles").([]interface{})
	if len(bundlesRaw) == 0 {
		return nil
	}
	return checkImageBundleRegionCoverage(bundlesRaw, collectRegionCodes(regionsRaw))
}
