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

package providerutil

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ImageBundleType constants matching YBA's ImageBundleType enum
const (
	ImageBundleTypeYBAActive     = "YBA_ACTIVE"
	ImageBundleTypeYBADeprecated = "YBA_DEPRECATED"
	ImageBundleTypeCustom        = "CUSTOM"
)

// DetectUserBundleRemoval checks if user removed bundles from config.
func DetectUserBundleRemoval(oldBundles, newBundles []interface{}) bool {
	// If the user specifies an empty list of bundles in their config, but the state has bundles,
	// Terraform sets newBundles = oldBundles because of Optional+Computed.
	// So we need to determine if newBundles is EXACTLY a copy of oldBundles.
	// If it is, and there are custom bundles, it means the user probably removed the block.
	if len(oldBundles) != len(newBundles) {
		return false
	}

	// If there are no custom bundles, we can't tell if it was removed or just wasn't there.
	hasCustom := false
	for _, b := range oldBundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType != ImageBundleTypeYBAActive {
			hasCustom = true
		}
	}
	if !hasCustom {
		return false
	}

	// If we have custom bundles, let's see if new is a perfect copy of old.
	// We'll just check UUIDs and Names
	for i := range oldBundles {
		oldB := oldBundles[i].(map[string]interface{})
		newB := newBundles[i].(map[string]interface{})

		if GetString(oldB, "name") != GetString(newB, "name") ||
			GetString(oldB, "uuid") != GetString(newB, "uuid") ||
			oldB["use_as_default"] != newB["use_as_default"] {
			return false
		}
	}

	return true
}

// BuildImageBundleFromState converts state data to client.ImageBundle.
func BuildImageBundleFromState(bundleMap map[string]interface{}) client.ImageBundle {
	name, _ := bundleMap["name"].(string)
	useAsDefault, _ := bundleMap["use_as_default"].(bool)
	uuid, _ := bundleMap["uuid"].(string)

	bundle := client.ImageBundle{
		Name:         utils.GetStringPointer(name),
		UseAsDefault: utils.GetBoolPointer(useAsDefault),
	}

	if uuid != "" {
		bundle.Uuid = utils.GetStringPointer(uuid)
	}

	if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
		if det, ok := details[0].(map[string]interface{}); ok {
			arch, _ := det["arch"].(string)
			sshUser, _ := det["ssh_user"].(string)
			sshPort, _ := det["ssh_port"].(int)
			globalYbImage, _ := det["global_yb_image"].(string)

			// GetInt32Pointer returns nil for 0; apply the semantic default so
			// YBA's SSH port validator never sees a null/missing value.
			if sshPort == 0 {
				sshPort = 22
			}

			bundle.Details = &client.ImageBundleDetails{
				Arch:          utils.GetStringPointer(arch),
				SshUser:       utils.GetStringPointer(sshUser),
				SshPort:       utils.GetInt32Pointer(int32(sshPort)),
				GlobalYbImage: utils.GetStringPointer(globalYbImage),
			}

			// use_imds_v2 is present in the AWS schema; absent for GCP/Azure so no-op there.
			if v, ok := det["use_imds_v2"].(bool); ok {
				bundle.Details.UseIMDSv2 = utils.GetBoolPointer(v)
			}

			if regionOverrides, ok := det["region_overrides"].(map[string]interface{}); ok &&
				len(regionOverrides) > 0 {
				overridesMap := make(map[string]client.BundleInfo)
				for region, image := range regionOverrides {
					overridesMap[region] = client.BundleInfo{
						YbImage: utils.GetStringPointer(image.(string)),
					}
				}
				bundle.Details.Regions = &overridesMap
			}
		}
	}

	return bundle
}

// EnsureImageBundleDefaults ensures that there is exactly one default bundle per architecture.
// If none exists for an architecture, it marks the first one.
// If multiple exist, it keeps only the first one marked as default.
func EnsureImageBundleDefaults(bundles []client.ImageBundle) []client.ImageBundle {
	if len(bundles) == 0 {
		return bundles
	}

	hasDefault := make(map[string]bool)
	firstIndex := make(map[string]int)

	// First pass: identify defaults, remember the first bundle for each arch,
	// and unmark any subsequent defaults to ensure exactly one.
	for i, b := range bundles {
		if b.Details != nil && b.Details.Arch != nil {
			arch := *b.Details.Arch
			if _, exists := firstIndex[arch]; !exists {
				firstIndex[arch] = i
			}

			if b.GetUseAsDefault() {
				if hasDefault[arch] {
					// We already have a default for this arch, unmark this one
					bundles[i].SetUseAsDefault(false)
				} else {
					hasDefault[arch] = true
				}
			}
		}
	}

	// Second pass: if an arch has no default, mark its first bundle as default
	for arch, idx := range firstIndex {
		if !hasDefault[arch] {
			bundles[idx].SetUseAsDefault(true)
		}
	}

	return bundles
}

// MergeImageBundlesForUpdate merges old state bundles with new config bundles.
// Deprecated: use PrepareImageBundlesForUpdate which sources YBA bundles from the live API.
func MergeImageBundlesForUpdate(
	oldBundlesRaw, newBundlesRaw interface{},
) []client.ImageBundle {
	// Get old bundles from state (TypeList returns []interface{} directly)
	oldBundlesList, _ := oldBundlesRaw.([]interface{})
	newBundlesList, _ := newBundlesRaw.([]interface{})

	// Build maps from state data for quick lookup
	oldByName := make(map[string]map[string]interface{})
	for _, b := range oldBundlesList {
		if bundleMap, ok := b.(map[string]interface{}); ok {
			if name, _ := bundleMap["name"].(string); name != "" {
				oldByName[name] = bundleMap
			}
		}
	}

	// Detect if user removed bundles from config
	// userBundlesRemoved := DetectUserBundleRemoval(oldBundlesList, newBundlesList)

	// YBA-managed bundles are always preserved from state (via yba_managed_image_bundles).
	// Custom image_bundles are processed separately from new config.

	// Collect YBA-managed bundle names (to skip them in new bundles loop)
	ybaManagedNames := make(map[string]bool)
	for _, b := range oldBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType == ImageBundleTypeYBAActive {
			if name, _ := bundleMap["name"].(string); name != "" {
				ybaManagedNames[name] = true
			}
		}
	}

	// Check if user is adding a bundle with use_as_default=true
	userHasDefaultArch := make(map[string]bool)
	for _, b := range newBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		if useAsDefault, _ := bundleMap["use_as_default"].(bool); useAsDefault {
			if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
				if det, ok := details[0].(map[string]interface{}); ok {
					if arch, _ := det["arch"].(string); arch != "" {
						userHasDefaultArch[arch] = true
					}
				}
			}
		}
	}

	// First, add YBA-managed bundles from state.
	// We always preserve them. The yba_managed_image_bundles schema block
	// controls inclusion/exclusion of YBA-managed bundles explicitly.

	// Track user defaults by architecture
	userDefaultByArch := make(map[string]string)
	for _, b := range newBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := bundleMap["name"].(string)
		if name == "" || ybaManagedNames[name] {
			continue
		}
		if useAsDefault, _ := bundleMap["use_as_default"].(bool); useAsDefault {
			if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
				if det, ok := details[0].(map[string]interface{}); ok {
					if arch, _ := det["arch"].(string); arch != "" {
						userDefaultByArch[arch] = name
					}
				}
			}
		}
	}

	// Build the final bundles list
	var resultBundles []client.ImageBundle

	// First, add YBA-managed bundles from state (always preserve)
	for _, b := range oldBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType == ImageBundleTypeYBAActive {
			bundle := BuildImageBundleFromState(bundleMap)

			// See if this bundle is in the new list from config
			useAsDefaultInNew := bundle.GetUseAsDefault()

			for _, nb := range newBundlesList {
				nbMap, ok := nb.(map[string]interface{})
				if !ok {
					continue
				}
				if name, _ := nbMap["name"].(string); name == bundle.GetName() {
					if useAsDefault, ok := nbMap["use_as_default"].(bool); ok {
						useAsDefaultInNew = useAsDefault
					}
					break
				}
			}

			// We ALWAYS preserve YBA-managed bundles, even if omitted from the new config list.
			// This prevents an issue where a user simply appends a CUSTOM bundle via TF config,
			// inadvertently omitting the YBA-managed bundle, which would cause an API error
			// if that YBA bundle is actively in-use by a Universe.
			// The only time we wouldn't preserve it is if we had explicit instruction to delete it,
			// but since it's a computed field managed by the backend, it's safer to always keep it.

			// Apply user's new use_as_default setting if explicitly specified in config.
			bundle.SetUseAsDefault(useAsDefaultInNew)

			// Handle default conflict: unmark YBA bundle if user has a custom default for this arch
			if bundle.Details != nil && bundle.Details.Arch != nil {
				arch := *bundle.Details.Arch
				if customDefaultName, hasCustomDefault := userDefaultByArch[arch]; hasCustomDefault {
					// Only unmark if the YBA bundle is NOT the one the user marked as default
					if bundle.GetName() != customDefaultName {
						bundle.SetUseAsDefault(false)
					}
				}
			}

			resultBundles = append(resultBundles, bundle)
		}
	}

	// We no longer rely on userBundlesRemoved to conditionally omit YBA bundles.
	// We ALWAYS preserve YBA managed bundles unless they were explicitly changed
	// in a way that requires replacing them (which isn't really a thing for YBA_ACTIVE bundles).

	// Process user bundles from new value
	for _, b := range newBundlesList {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := bundleMap["name"].(string)
		if name == "" {
			continue
		}

		// Skip YBA-managed bundles - they're already added from old state
		if ybaManagedNames[name] {
			continue
		}

		useAsDefault, _ := bundleMap["use_as_default"].(bool)

		// Get details
		var arch, sshUser, globalYbImage string
		var sshPort int
		var regionOverrides map[string]interface{}
		var useIMDSv2 bool
		var hasUseIMDSv2 bool

		if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
			if det, ok := details[0].(map[string]interface{}); ok {
				arch, _ = det["arch"].(string)
				sshUser, _ = det["ssh_user"].(string)
				sshPort, _ = det["ssh_port"].(int)
				globalYbImage, _ = det["global_yb_image"].(string)
				regionOverrides, _ = det["region_overrides"].(map[string]interface{})
				useIMDSv2, hasUseIMDSv2 = det["use_imds_v2"].(bool)
			}
		}

		if arch == "" {
			continue
		}

		// Build the bundle
		bundle := client.ImageBundle{
			Name:         utils.GetStringPointer(name),
			UseAsDefault: utils.GetBoolPointer(useAsDefault),
			Details: &client.ImageBundleDetails{
				Arch:          utils.GetStringPointer(arch),
				SshUser:       utils.GetStringPointer(sshUser),
				SshPort:       utils.GetInt32Pointer(int32(sshPort)),
				GlobalYbImage: utils.GetStringPointer(globalYbImage),
			},
		}

		// use_imds_v2 is in the AWS schema; present in the map when set, absent for GCP/Azure.
		if hasUseIMDSv2 {
			bundle.Details.UseIMDSv2 = utils.GetBoolPointer(useIMDSv2)
		}

		// Add region overrides if present
		if len(regionOverrides) > 0 {
			overridesMap := make(map[string]client.BundleInfo)
			for region, image := range regionOverrides {
				overridesMap[region] = client.BundleInfo{
					YbImage: utils.GetStringPointer(image.(string)),
				}
			}
			bundle.Details.Regions = &overridesMap
		}

		// Copy UUID from state
		if uuid, _ := bundleMap["uuid"].(string); uuid != "" {
			bundle.Uuid = utils.GetStringPointer(uuid)
		} else if oldBundle, exists := oldByName[name]; exists {
			if uuid, _ := oldBundle["uuid"].(string); uuid != "" {
				bundle.Uuid = utils.GetStringPointer(uuid)
			}
		}

		resultBundles = append(resultBundles, bundle)
	}

	return EnsureImageBundleDefaults(resultBundles)
}

// PrepareImageBundlesForUpdate builds the image bundle list for a provider update.
// YBA-managed bundles are sourced from the live API response to avoid stale-state conflicts.
// ybaConfigChanged=false: all YBA bundles forwarded unchanged; =true+non-empty: only specified
// archs with their use_as_default applied; =true+empty: no YBA bundles sent (explicit removal).
func PrepareImageBundlesForUpdate(
	currentAPIBundles []client.ImageBundle,
	oldImageBundlesRaw, newImageBundlesRaw interface{},
	ybaConfigBundlesRaw []interface{},
	ybaConfigChanged bool,
) []client.ImageBundle {
	// Step 1: collect YBA-managed bundles from the live API response, keyed by arch.
	ybaByArch := make(map[string]client.ImageBundle)
	for _, b := range currentAPIBundles {
		metadata := b.GetMetadata()
		if metadata.GetType() == ImageBundleTypeYBAActive {
			if b.Details != nil {
				ybaByArch[b.Details.GetArch()] = b
			}
		}
	}

	// Step 2: decide which YBA-managed bundles to include.
	//
	// When ybaConfigChanged=false the user did not touch yba_managed_image_bundles, so YBA
	// bundles are carried forward from the API exactly as-is (no use_as_default changes).
	// If the user's custom bundle also claims use_as_default=true for the same arch,
	// EnsureImageBundleDefaults (called below) will resolve the conflict by keeping the first
	// occurrence. To become the default, the user must explicitly set yba_managed_image_bundles
	// with use_as_default=false for that arch.
	//
	// When ybaConfigChanged=true the user is explicitly controlling YBA bundles. We apply
	// their use_as_default preferences, and additionally demote the YBA bundle if the user's
	// custom bundle for that arch has use_as_default=true.
	var result []client.ImageBundle

	// Step 4: build a UUID lookup from old image_bundles state (custom bundles only).
	oldByName := make(map[string]string)
	if oldList, ok := oldImageBundlesRaw.([]interface{}); ok {
		for _, item := range oldList {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := m["name"].(string)
			uuid, _ := m["uuid"].(string)
			if name != "" && uuid != "" {
				oldByName[name] = uuid
			}
		}
	}

	// Archs claimed as default by custom bundles (used to demote YBA bundles).
	customDefaultByArch := make(map[string]bool)
	if newList, ok := newImageBundlesRaw.([]interface{}); ok {
		for _, item := range newList {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if metaType, _ := m["metadata_type"].(string); metaType == ImageBundleTypeYBAActive {
				continue
			}
			if useAsDefault, _ := m["use_as_default"].(bool); useAsDefault {
				if details, ok := m["details"].([]interface{}); ok && len(details) > 0 {
					if det, ok := details[0].(map[string]interface{}); ok {
						if arch, _ := det["arch"].(string); arch != "" {
							customDefaultByArch[arch] = true
						}
					}
				}
			}
		}
	}

	// Custom bundles (existing UUID) come first so updateBundles clears any existing
	// default before createBundle runs; otherwise YBA rejects with "already has default".
	if newList, ok := newImageBundlesRaw.([]interface{}); ok {
		for _, item := range newList {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if metaType, _ := m["metadata_type"].(string); metaType == ImageBundleTypeYBAActive {
				continue
			}
			bundle := BuildImageBundleFromState(m)
			if bundle.Uuid == nil || *bundle.Uuid == "" {
				if name := bundle.GetName(); name != "" {
					if uuid, exists := oldByName[name]; exists {
						bundle.Uuid = utils.GetStringPointer(uuid)
					}
				}
			}
			result = append(result, bundle)
		}
	}

	// YBA bundles appended after custom bundles (see ordering note above).
	if !ybaConfigChanged {
		for _, b := range ybaByArch {
			result = append(result, b)
		}
	} else if len(ybaConfigBundlesRaw) > 0 {
		for _, item := range ybaConfigBundlesRaw {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			arch, _ := m["arch"].(string)
			useAsDefault, _ := m["use_as_default"].(bool)
			if customDefaultByArch[arch] {
				useAsDefault = false
			}
			if b, exists := ybaByArch[arch]; exists {
				b.SetUseAsDefault(useAsDefault)
				result = append(result, b)
			} else {
				// New placeholder: createBundle sees ybImage==null and fetches default AMI.
				emptyRegions := make(map[string]client.BundleInfo)
				result = append(result, client.ImageBundle{
					Name:         utils.GetStringPointer("YBA-Managed-" + arch),
					UseAsDefault: utils.GetBoolPointer(useAsDefault),
					Details: &client.ImageBundleDetails{
						Arch:    utils.GetStringPointer(arch),
						SshPort: utils.GetInt32Pointer(22),
						Regions: &emptyRegions,
					},
				})
			}
		}
	}
	// else: ybaConfigChanged=true AND empty list - user explicitly removed all YBA bundles.

	// Existing bundles (UUID) before new bundles (no UUID), non-defaults before defaults.
	// Ensures updateBundles clears the old default before createBundle sets a new one.
	sort.SliceStable(result, func(i, j int) bool {
		iHasUUID := result[i].Uuid != nil && *result[i].Uuid != ""
		jHasUUID := result[j].Uuid != nil && *result[j].Uuid != ""
		if iHasUUID != jHasUUID {
			return iHasUUID // existing bundles (UUID) before new bundles (no UUID)
		}
		// Within same group: non-defaults before defaults
		iDefault := result[i].UseAsDefault != nil && *result[i].UseAsDefault
		jDefault := result[j].UseAsDefault != nil && *result[j].UseAsDefault
		return !iDefault && jDefault
	})

	return EnsureImageBundleDefaults(result)
}

// GetString safely extracts string from map
func GetString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// GetBundleDetails extracts details map from a bundle
func GetBundleDetails(bundle map[string]interface{}) map[string]interface{} {
	if details, ok := bundle["details"].([]interface{}); ok && len(details) > 0 {
		if det, ok := details[0].(map[string]interface{}); ok {
			return det
		}
	}
	return make(map[string]interface{})
}

// BundleContentChanged compares editable fields between two bundles
func BundleContentChanged(old, new map[string]interface{}) bool {
	// Compare use_as_default
	oldDefault, _ := old["use_as_default"].(bool)
	newDefault, _ := new["use_as_default"].(bool)
	if oldDefault != newDefault {
		return true
	}

	// Compare details
	oldDetails := GetBundleDetails(old)
	newDetails := GetBundleDetails(new)

	if oldDetails["arch"] != newDetails["arch"] ||
		oldDetails["ssh_user"] != newDetails["ssh_user"] ||
		oldDetails["ssh_port"] != newDetails["ssh_port"] ||
		oldDetails["global_yb_image"] != newDetails["global_yb_image"] ||
		oldDetails["use_imds_v2"] != newDetails["use_imds_v2"] {
		return true
	}

	// Compare region_overrides
	oldOverrides, _ := oldDetails["region_overrides"].(map[string]interface{})
	newOverrides, _ := newDetails["region_overrides"].(map[string]interface{})
	if len(oldOverrides) != len(newOverrides) {
		return true
	}
	for k, v := range oldOverrides {
		if newOverrides[k] != v {
			return true
		}
	}

	return false
}

// CollectYBAManagedNames finds all YBA-managed bundle names from state
func CollectYBAManagedNames(bundlesRaw interface{}) map[string]bool {
	ybaManagedNames := make(map[string]bool)

	bundles, _ := bundlesRaw.([]interface{})

	for _, b := range bundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType == ImageBundleTypeYBAActive {
			if name, _ := bundleMap["name"].(string); name != "" {
				ybaManagedNames[name] = true
			}
		}
	}

	return ybaManagedNames
}

// ExtractUserBundlesMap extracts user bundles as a map (name -> bundle)
func ExtractUserBundlesMap(
	bundlesRaw interface{},
	ybaManagedNames map[string]bool,
) map[string]map[string]interface{} {
	userBundles := make(map[string]map[string]interface{})

	bundles, _ := bundlesRaw.([]interface{})

	for _, b := range bundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := bundleMap["name"].(string)
		if name == "" {
			continue
		}

		// Skip YBA-managed bundles
		if ybaManagedNames[name] {
			continue
		}

		userBundles[name] = bundleMap
	}

	return userBundles
}

// HasImageBundleRealChange checks if user actually changed
// image_bundles or yba_managed_image_bundles.
func HasImageBundleRealChange(d *schema.ResourceDiff) bool {
	// Check custom bundles
	if d.HasChange("image_bundles") {
		oldRaw, newRaw := d.GetChange("image_bundles")

		oldBundles, _ := oldRaw.([]interface{})
		newBundles, _ := newRaw.([]interface{})

		if len(oldBundles) != len(newBundles) {
			return true
		}

		for i := range oldBundles {
			oldBundle := oldBundles[i].(map[string]interface{})
			newBundle := newBundles[i].(map[string]interface{})

			if GetString(oldBundle, "name") != GetString(newBundle, "name") {
				return true
			}

			oldDefault, _ := oldBundle["use_as_default"].(bool)
			newDefault, _ := newBundle["use_as_default"].(bool)
			if oldDefault != newDefault {
				return true
			}

			if BundleContentChanged(oldBundle, newBundle) {
				return true
			}
		}
	}

	// Check YBA managed bundles by arch (identity key); only use_as_default is user-editable.
	if d.HasChange("yba_managed_image_bundles") {
		oldRaw, newRaw := d.GetChange("yba_managed_image_bundles")

		oldBundles, _ := oldRaw.([]interface{})
		newBundles, _ := newRaw.([]interface{})

		// Index old bundles by arch.
		oldByArch := make(map[string]bool)
		for _, b := range oldBundles {
			m, ok := b.(map[string]interface{})
			if !ok {
				continue
			}
			arch := GetString(m, "arch")
			if arch == "" {
				continue
			}
			useAsDefault, _ := m["use_as_default"].(bool)
			oldByArch[arch] = useAsDefault
		}

		newArchsSeen := make(map[string]bool)
		for _, b := range newBundles {
			m, ok := b.(map[string]interface{})
			if !ok {
				continue
			}
			arch := GetString(m, "arch")
			if arch == "" {
				continue
			}
			newArchsSeen[arch] = true
			newDefault, _ := m["use_as_default"].(bool)
			oldDefault, existed := oldByArch[arch]
			if !existed || oldDefault != newDefault {
				return true
			}
		}

		// Arch removed from config.
		for arch := range oldByArch {
			if !newArchsSeen[arch] {
				return true
			}
		}
	}

	return false
}

// CopyBundleMap creates a deep copy of a bundle map.
func CopyBundleMap(original map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{})
	for k, v := range original {
		if k == "details" {
			if details, ok := v.([]interface{}); ok && len(details) > 0 {
				if detMap, ok := details[0].(map[string]interface{}); ok {
					detCopy := make(map[string]interface{})
					for dk, dv := range detMap {
						detCopy[dk] = dv
					}
					cp[k] = []interface{}{detCopy}
					continue
				}
			}
		}
		cp[k] = v
	}
	return cp
}

// PreserveYBAManagedBundlesInPlan ensures YBA-managed bundles are preserved in the plan.
// It injects any YBA-managed bundles from state that the user omitted from their config,
// preventing misleading plan diffs that show YBA bundles being removed.
func PreserveYBAManagedBundlesInPlan(ctx context.Context, d *schema.ResourceDiff) error {
	// If there's no change to image_bundles, we don't need to manipulate the plan.
	if !d.HasChange("image_bundles") {
		return nil
	}

	oldRaw, newRaw := d.GetChange("image_bundles")

	// TypeList returns []interface{} directly
	oldBundles, _ := oldRaw.([]interface{})
	newBundles, _ := newRaw.([]interface{})

	// Find YBA-managed bundles in old state
	var ybaManagedBundles []interface{}
	for _, b := range oldBundles {
		bundleMap, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		metadataType, _ := bundleMap["metadata_type"].(string)
		if metadataType == ImageBundleTypeYBAActive {
			ybaManagedBundles = append(ybaManagedBundles, bundleMap)
		}
	}

	if len(ybaManagedBundles) == 0 {
		return nil
	}

	// Check if YBA-managed bundles are already in the new value (by name)
	newBundleNames := make(map[string]bool)
	for _, b := range newBundles {
		if bundleMap, ok := b.(map[string]interface{}); ok {
			if name, _ := bundleMap["name"].(string); name != "" {
				newBundleNames[name] = true
			}
		}
	}

	// Since we ALWAYS preserve YBA-managed bundles during MergeImageBundlesForUpdate,
	// we must also force them into the new plan so that Terraform doesn't falsely
	// show them as being "removed".
	needsMerge := false
	for _, ybaBundle := range ybaManagedBundles {
		bundleMap := ybaBundle.(map[string]interface{})
		name, _ := bundleMap["name"].(string)
		if !newBundleNames[name] {
			needsMerge = true
			break
		}
	}

	if !needsMerge {
		return nil
	}

	// Build a map of architectures to the name of the bundle the user has marked as default
	userDefaultByArch := make(map[string]string)
	for _, b := range newBundles {
		if bundleMap, ok := b.(map[string]interface{}); ok {
			if useAsDefault, _ := bundleMap["use_as_default"].(bool); useAsDefault {
				if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
					if det, ok := details[0].(map[string]interface{}); ok {
						if arch, _ := det["arch"].(string); arch != "" {
							if name, _ := bundleMap["name"].(string); name != "" {
								userDefaultByArch[arch] = name
							}
						}
					}
				}
			}
		}
	}

	// Build merged bundle list (using slice for TypeList)
	var mergedList []interface{}

	// Add all new bundles first (user-provided bundles)
	for _, b := range newBundles {
		mergedList = append(mergedList, b)
	}

	// Add YBA-managed bundles that aren't already in new value
	for _, ybaBundle := range ybaManagedBundles {
		bundleMap := ybaBundle.(map[string]interface{})
		name, _ := bundleMap["name"].(string)

		if newBundleNames[name] {
			continue
		}

		// Always create a copy to avoid modifying original state
		bundleCopy := CopyBundleMap(bundleMap)

		// Check if user has a default for this arch in their custom bundles
		// and unmark the YBA bundle if necessary
		if details, ok := bundleMap["details"].([]interface{}); ok && len(details) > 0 {
			if det, ok := details[0].(map[string]interface{}); ok {
				if arch, _ := det["arch"].(string); arch != "" {
					if customDefaultName, hasCustomDefault := userDefaultByArch[arch]; hasCustomDefault {
						// Only unmark if the YBA bundle is NOT the one the user marked as default
						if name != customDefaultName {
							bundleCopy["use_as_default"] = false
						}
					}
				}
			}
		}

		mergedList = append(mergedList, bundleCopy)
	}

	if err := d.SetNew("image_bundles", mergedList); err != nil {
		return err
	}

	return nil
}

// ValidateImageBundles checks image_bundles for duplicate names and duplicate
// use_as_default=true within the same architecture.
func ValidateImageBundles(d *schema.ResourceDiff) error {
	bundles, _ := d.Get("image_bundles").([]interface{})

	seenNames := make(map[string]bool)
	defaultByArch := make(map[string]bool)

	for _, b := range bundles {
		m, ok := b.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := m["name"].(string)
		if name != "" {
			if seenNames[name] {
				return fmt.Errorf(
					"duplicate name %q in image_bundles: each bundle must have a unique name",
					name,
				)
			}
			seenNames[name] = true
		}

		if useAsDefault, _ := m["use_as_default"].(bool); useAsDefault {
			details, ok := m["details"].([]interface{})
			if !ok || len(details) == 0 {
				continue
			}
			det, ok := details[0].(map[string]interface{})
			if !ok {
				continue
			}
			arch, _ := det["arch"].(string)
			if arch == "" {
				continue
			}
			if defaultByArch[arch] {
				return fmt.Errorf(
					"multiple image_bundles have use_as_default=true for arch %q: "+
						"at most one bundle per architecture can be the default",
					arch,
				)
			}
			defaultByArch[arch] = true
		}
	}

	return nil
}

// ValidateAtLeastOneImageBundle returns an error if neither image_bundles nor
// yba_managed_image_bundles contains at least one entry, mirroring the YBA UI
// which always requires a default image before a provider can be created.
func ValidateAtLeastOneImageBundle(d *schema.ResourceDiff) error {
	imageBundles, _ := d.Get("image_bundles").([]interface{})
	ybaManagedBundles, _ := d.Get("yba_managed_image_bundles").([]interface{})
	if len(imageBundles) == 0 && len(ybaManagedBundles) == 0 {
		return fmt.Errorf(
			"at least one image_bundles or yba_managed_image_bundles block must be specified: " +
				"use yba_managed_image_bundles to use YBA default images, or image_bundles for custom images",
		)
	}
	return nil
}

// commonVersionFields are the schema fields shared by every cloud provider that,
// when changed, cause YBA to increment the provider version.
var commonVersionFields = []string{
	"name", "air_gap_install", "ntp_servers", "set_up_chrony",
	"ssh_keypair_name", "ssh_private_key_content",
}

// MarkVersionComputedIfChanged marks the version attribute as (known after apply)
// when any user-editable field has changed. cloudFields contains the provider-specific
// fields beyond the common set. regionsContentChanged is the provider's own function
// that determines whether region changes are meaningful (ignoring pure reorders).
func MarkVersionComputedIfChanged(
	ctx context.Context,
	d *schema.ResourceDiff,
	cloudFields []string,
	regionsContentChanged func(interface{}, interface{}) bool,
) error {
	if d.Id() == "" {
		return nil
	}

	hasRealChange := false

	for _, field := range append(commonVersionFields, cloudFields...) {
		if d.HasChange(field) {
			hasRealChange = true
			break
		}
	}

	if !hasRealChange && d.HasChange("regions") {
		oldRaw, newRaw := d.GetChange("regions")
		if regionsContentChanged(oldRaw, newRaw) {
			hasRealChange = true
		}
	}

	if !hasRealChange && HasImageBundleRealChange(d) {
		hasRealChange = true
	}

	if hasRealChange {
		if err := d.SetNewComputed("version"); err != nil {
			return err
		}
	}

	return PreserveYBAManagedBundlesInPlan(ctx, d)
}

// ValidateNewBundlesNotDefault rejects use_as_default=true on a new bundle (no UUID)
// during UPDATE. createBundle rejects this when another bundle is already the default;
// add the bundle first (use_as_default=false) then promote it in a subsequent apply.
func ValidateNewBundlesNotDefault(d *schema.ResourceDiff) error {
	if d.Id() == "" {
		return nil // creates are fine - no existing default yet
	}

	bundles, _ := d.Get("image_bundles").([]interface{})
	for _, b := range bundles {
		m, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		if uuid, _ := m["uuid"].(string); uuid != "" {
			continue // existing bundle - can freely change its default flag
		}
		if useAsDefault, _ := m["use_as_default"].(bool); useAsDefault {
			name, _ := m["name"].(string)
			return fmt.Errorf(
				"image bundle %q is new (no uuid yet) and cannot have use_as_default=true "+
					"in the same apply: add it first with use_as_default=false, then set it "+
					"as default in a subsequent apply once it has been created",
				name,
			)
		}
	}

	ybaBundles, _ := d.Get("yba_managed_image_bundles").([]interface{})
	for _, b := range ybaBundles {
		m, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		if uuid, _ := m["uuid"].(string); uuid != "" {
			continue
		}
		if useAsDefault, _ := m["use_as_default"].(bool); useAsDefault {
			arch, _ := m["arch"].(string)
			return fmt.Errorf(
				"yba_managed_image_bundles entry for arch %q is new (no uuid yet) and cannot "+
					"have use_as_default=true in the same apply: add it first with "+
					"use_as_default=false, then set it as default in a subsequent apply",
				arch,
			)
		}
	}

	return nil
}
