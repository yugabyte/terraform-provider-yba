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
	"testing"
)

// makeBundle is a helper for building a minimal image_bundle map for tests.
func makeBundle(name, arch string, useAsDefault bool) map[string]interface{} {
	return map[string]interface{}{
		"name":           name,
		"use_as_default": useAsDefault,
		"details": []interface{}{
			map[string]interface{}{
				"arch":     arch,
				"ssh_user": "ec2-user",
				"ssh_port": 22,
			},
		},
	}
}

func getUseAsDefault(b interface{}) bool {
	m, ok := b.(map[string]interface{})
	if !ok {
		return false
	}
	v, _ := m["use_as_default"].(bool)
	return v
}

// TestNormalizeImageBundleDefaults_NoChange verifies that when at least one bundle
// per arch already has use_as_default=true, the slice is returned unchanged.
func TestNormalizeImageBundleDefaults_NoChange(t *testing.T) {
	bundles := []interface{}{
		makeBundle("b1", "x86_64", true),
		makeBundle("b2", "x86_64", false),
	}

	result, changed := normalizeImageBundleDefaults(bundles)

	if changed {
		t.Error("expected changed=false when a default already exists, got true")
	}
	if len(result) != len(bundles) {
		t.Errorf("expected %d bundles, got %d", len(bundles), len(result))
	}
}

// TestNormalizeImageBundleDefaults_PromoteFirst verifies that the first bundle for
// an arch is promoted to use_as_default=true when none has it set.
func TestNormalizeImageBundleDefaults_PromoteFirst(t *testing.T) {
	bundles := []interface{}{
		makeBundle("first", "x86_64", false),
		makeBundle("second", "x86_64", false),
	}

	result, changed := normalizeImageBundleDefaults(bundles)

	if !changed {
		t.Error("expected changed=true when no default exists, got false")
	}
	if !getUseAsDefault(result[0]) {
		t.Error("expected first bundle to be promoted to use_as_default=true")
	}
	if getUseAsDefault(result[1]) {
		t.Error("expected second bundle to remain use_as_default=false")
	}
}

// TestNormalizeImageBundleDefaults_MultiArch verifies that each arch independently
// gets its first bundle promoted when no default exists for that arch.
func TestNormalizeImageBundleDefaults_MultiArch(t *testing.T) {
	bundles := []interface{}{
		makeBundle("x86-1", "x86_64", false),
		makeBundle("x86-2", "x86_64", false),
		makeBundle("arm-1", "aarch64", false),
		makeBundle("arm-2", "aarch64", false),
	}

	result, changed := normalizeImageBundleDefaults(bundles)

	if !changed {
		t.Error("expected changed=true")
	}
	if !getUseAsDefault(result[0]) {
		t.Error("x86-1 should be promoted to default")
	}
	if getUseAsDefault(result[1]) {
		t.Error("x86-2 should remain non-default")
	}
	if !getUseAsDefault(result[2]) {
		t.Error("arm-1 should be promoted to default")
	}
	if getUseAsDefault(result[3]) {
		t.Error("arm-2 should remain non-default")
	}
}

// TestNormalizeImageBundleDefaults_PartialDefaults verifies that only arches without a
// default are normalized; arches that already have a default are untouched.
func TestNormalizeImageBundleDefaults_PartialDefaults(t *testing.T) {
	bundles := []interface{}{
		makeBundle("x86-a", "x86_64", true),   // already default
		makeBundle("x86-b", "x86_64", false),  // should stay false
		makeBundle("arm-a", "aarch64", false), // no default for this arch -> promoted
	}

	result, changed := normalizeImageBundleDefaults(bundles)

	if !changed {
		t.Error("expected changed=true because aarch64 has no default")
	}
	if !getUseAsDefault(result[0]) {
		t.Error("x86-a should remain use_as_default=true")
	}
	if getUseAsDefault(result[1]) {
		t.Error("x86-b should remain use_as_default=false")
	}
	if !getUseAsDefault(result[2]) {
		t.Error("arm-a should be promoted to default")
	}
}

// TestNormalizeImageBundleDefaults_Empty verifies that an empty slice is a no-op.
func TestNormalizeImageBundleDefaults_Empty(t *testing.T) {
	result, changed := normalizeImageBundleDefaults([]interface{}{})

	if changed {
		t.Error("expected changed=false for empty input")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d elements", len(result))
	}
}

// TestNormalizeImageBundleDefaults_NoMutateOriginal verifies that the original slice
// is not mutated; a new copy is returned.
func TestNormalizeImageBundleDefaults_NoMutateOriginal(t *testing.T) {
	orig := makeBundle("b1", "x86_64", false)
	bundles := []interface{}{orig}

	result, changed := normalizeImageBundleDefaults(bundles)

	if !changed {
		t.Error("expected changed=true")
	}
	// Original map must remain unchanged.
	if getUseAsDefault(orig) {
		t.Error("original bundle map must not be mutated")
	}
	// Result has the promoted value.
	if !getUseAsDefault(result[0]) {
		t.Error("result bundle should have use_as_default=true")
	}
}
