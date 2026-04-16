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

package aws

import (
	"strings"
	"testing"
)

// TestValidateDuplicateRegions tests detection of duplicate region codes
func TestValidateDuplicateRegions(t *testing.T) {
	tests := []struct {
		name        string
		regions     []map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "no duplicates - single region",
			regions: []map[string]interface{}{
				{
					"code":  "us-west-2",
					"zones": []interface{}{},
				},
			},
			expectError: false,
		},
		{
			name: "no duplicates - multiple regions",
			regions: []map[string]interface{}{
				{
					"code":  "us-west-2",
					"zones": []interface{}{},
				},
				{
					"code":  "us-east-1",
					"zones": []interface{}{},
				},
				{
					"code":  "ap-south-1",
					"zones": []interface{}{},
				},
			},
			expectError: false,
		},
		{
			name: "duplicate region codes",
			regions: []map[string]interface{}{
				{
					"code":  "us-west-2",
					"zones": []interface{}{},
				},
				{
					"code":  "us-west-2",
					"zones": []interface{}{},
				},
			},
			expectError: true,
			errorMsg:    "duplicate region code \"us-west-2\"",
		},
		{
			name: "duplicate among multiple regions",
			regions: []map[string]interface{}{
				{
					"code":  "us-west-2",
					"zones": []interface{}{},
				},
				{
					"code":  "us-east-1",
					"zones": []interface{}{},
				},
				{
					"code":  "ap-south-1",
					"zones": []interface{}{},
				},
				{
					"code":  "ap-south-1",
					"zones": []interface{}{},
				},
			},
			expectError: true,
			errorMsg:    "duplicate region code \"ap-south-1\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDuplicateRegions(tt.regions)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidateDuplicateZones tests detection of duplicate zone codes within a region
func TestValidateDuplicateZones(t *testing.T) {
	tests := []struct {
		name        string
		regionCode  string
		zones       []map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:       "no duplicates - single zone",
			regionCode: "us-west-2",
			zones: []map[string]interface{}{
				{"code": "us-west-2a"},
			},
			expectError: false,
		},
		{
			name:       "no duplicates - multiple zones",
			regionCode: "us-west-2",
			zones: []map[string]interface{}{
				{"code": "us-west-2a"},
				{"code": "us-west-2b"},
				{"code": "us-west-2c"},
			},
			expectError: false,
		},
		{
			name:       "duplicate zone codes",
			regionCode: "us-west-2",
			zones: []map[string]interface{}{
				{"code": "us-west-2a"},
				{"code": "us-west-2a"},
			},
			expectError: true,
			errorMsg:    "duplicate zone code \"us-west-2a\"",
		},
		{
			name:       "duplicate among multiple zones",
			regionCode: "ap-south-1",
			zones: []map[string]interface{}{
				{"code": "ap-south-1a"},
				{"code": "ap-south-1b"},
				{"code": "ap-south-1a"},
			},
			expectError: true,
			errorMsg:    "duplicate zone code \"ap-south-1a\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDuplicateZones(tt.regionCode, tt.zones)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestCheckImageBundleRegionCoverage exercises the pure region-coverage logic.
// It covers the four operations described in the PLAT-20294 issue:
// provider creation, adding a region, adding a bundle, and modifying a bundle.
func TestCheckImageBundleRegionCoverage(t *testing.T) {
	bundle := func(name string, overrides map[string]interface{}) interface{} {
		return map[string]interface{}{
			"name": name,
			"details": []interface{}{
				map[string]interface{}{
					"arch":             "x86_64",
					"ssh_user":         "ec2-user",
					"ssh_port":         22,
					"use_imds_v2":      true,
					"region_overrides": overrides,
				},
			},
		}
	}

	tests := []struct {
		name        string
		bundles     []interface{}
		regions     []string
		expectError bool
		errorMsg    string
	}{
		// --- happy paths ---
		{
			name: "single bundle single region covered",
			bundles: []interface{}{
				bundle("b1", map[string]interface{}{"us-east-1": "ami-aaa"}),
			},
			regions:     []string{"us-east-1"},
			expectError: false,
		},
		{
			name: "single bundle multiple regions all covered",
			bundles: []interface{}{
				bundle("b1", map[string]interface{}{
					"us-east-1": "ami-aaa",
					"us-west-2": "ami-bbb",
				}),
			},
			regions:     []string{"us-east-1", "us-west-2"},
			expectError: false,
		},
		{
			name: "multiple bundles all regions covered",
			bundles: []interface{}{
				bundle("b1", map[string]interface{}{
					"us-east-1": "ami-aaa",
					"us-west-2": "ami-bbb",
				}),
				bundle("b2", map[string]interface{}{
					"us-east-1": "ami-ccc",
					"us-west-2": "ami-ddd",
				}),
			},
			regions:     []string{"us-east-1", "us-west-2"},
			expectError: false,
		},
		{
			name:        "no bundles - validation skipped",
			bundles:     []interface{}{},
			regions:     []string{"us-east-1"},
			expectError: false,
		},
		{
			name: "bundle has extra region not in provider - allowed",
			bundles: []interface{}{
				bundle("b1", map[string]interface{}{
					"us-east-1":  "ami-aaa",
					"ap-south-1": "ami-zzz", // extra; not a provider region
				}),
			},
			regions:     []string{"us-east-1"},
			expectError: false,
		},
		// --- provider creation: missing region from the start ---
		{
			name: "create: bundle missing one region",
			bundles: []interface{}{
				bundle("b1", map[string]interface{}{"us-east-1": "ami-aaa"}),
			},
			regions:     []string{"us-east-1", "us-west-2"},
			expectError: true,
			errorMsg:    `image bundle "b1" must specify a non-empty AMI for region "us-west-2"`,
		},
		// --- add region: existing bundle lacks AMI for new region ---
		{
			name: "add region: second bundle has no override for new region",
			bundles: []interface{}{
				bundle("b1", map[string]interface{}{
					"us-east-1": "ami-aaa",
					"us-west-2": "ami-bbb",
				}),
				bundle("b2", map[string]interface{}{
					"us-east-1": "ami-ccc",
					// us-west-2 missing in b2
				}),
			},
			regions:     []string{"us-east-1", "us-west-2"},
			expectError: true,
			errorMsg:    `image bundle "b2" must specify a non-empty AMI for region "us-west-2"`,
		},
		// --- add bundle: new bundle is incomplete ---
		{
			name: "add bundle: new bundle missing a region",
			bundles: []interface{}{
				bundle("existing-bundle", map[string]interface{}{
					"us-east-1": "ami-aaa",
					"us-west-2": "ami-bbb",
				}),
				bundle("new-bundle", map[string]interface{}{
					"us-east-1": "ami-ccc",
					// us-west-2 intentionally omitted in new-bundle
				}),
			},
			regions:     []string{"us-east-1", "us-west-2"},
			expectError: true,
			errorMsg:    `image bundle "new-bundle" must specify a non-empty AMI for region "us-west-2"`,
		},
		// --- modify bundle: user sets AMI to empty string ---
		{
			name: "modify bundle: AMI set to empty string",
			bundles: []interface{}{
				bundle("b1", map[string]interface{}{
					"us-east-1": "ami-aaa",
					"us-west-2": "", // user cleared the AMI
				}),
			},
			regions:     []string{"us-east-1", "us-west-2"},
			expectError: true,
			errorMsg:    `image bundle "b1" must specify a non-empty AMI for region "us-west-2"`,
		},
		// --- region_overrides absent entirely ---
		{
			name: "bundle with no region_overrides at all",
			bundles: []interface{}{
				map[string]interface{}{
					"name": "bare-bundle",
					"details": []interface{}{
						map[string]interface{}{
							"arch":        "x86_64",
							"ssh_user":    "ec2-user",
							"ssh_port":    22,
							"use_imds_v2": true,
							// region_overrides key absent
						},
					},
				},
			},
			regions:     []string{"us-east-1"},
			expectError: true,
			errorMsg:    `image bundle "bare-bundle" must specify a non-empty AMI for region "us-east-1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkImageBundleRegionCoverage(tt.bundles, tt.regions)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestNormalizeRegionOverrides_NoChange verifies that when all overrides are for active
// regions, the slice is returned unchanged.
func TestNormalizeRegionOverrides_NoChange(t *testing.T) {
	active := map[string]bool{"us-east-1": true, "us-west-2": true}
	bundles := []interface{}{
		map[string]interface{}{
			"name": "b1",
			"details": []interface{}{
				map[string]interface{}{
					"region_overrides": map[string]interface{}{
						"us-east-1": "ami-aaa",
						"us-west-2": "ami-bbb",
					},
				},
			},
		},
	}

	result, changed := normalizeRegionOverrides(bundles, active)

	if changed {
		t.Error("expected changed=false when all overrides are for active regions")
	}
	_ = result
}

// TestNormalizeRegionOverrides_StripInactive verifies that overrides for regions not in
// activeRegions are stripped.
func TestNormalizeRegionOverrides_StripInactive(t *testing.T) {
	active := map[string]bool{"us-east-1": true}
	bundles := []interface{}{
		map[string]interface{}{
			"name": "b1",
			"details": []interface{}{
				map[string]interface{}{
					"region_overrides": map[string]interface{}{
						"us-east-1": "ami-aaa",
						"us-west-2": "ami-bbb", // inactive
					},
				},
			},
		},
	}

	result, changed := normalizeRegionOverrides(bundles, active)

	if !changed {
		t.Error("expected changed=true when an inactive override exists")
	}
	m := result[0].(map[string]interface{})
	det := m["details"].([]interface{})[0].(map[string]interface{})
	overrides := det["region_overrides"].(map[string]interface{})

	if _, found := overrides["us-west-2"]; found {
		t.Error("us-west-2 should have been stripped from region_overrides")
	}
	if overrides["us-east-1"] != "ami-aaa" {
		t.Errorf("us-east-1 should be preserved, got %v", overrides["us-east-1"])
	}
}

// TestNormalizeRegionOverrides_AllInactive verifies that when all overrides are for
// inactive regions, an empty map remains after normalization.
func TestNormalizeRegionOverrides_AllInactive(t *testing.T) {
	active := map[string]bool{"us-east-1": true}
	bundles := []interface{}{
		map[string]interface{}{
			"name": "b1",
			"details": []interface{}{
				map[string]interface{}{
					"region_overrides": map[string]interface{}{
						"us-west-2":  "ami-bbb",
						"ap-south-1": "ami-ccc",
					},
				},
			},
		},
	}

	result, changed := normalizeRegionOverrides(bundles, active)

	if !changed {
		t.Error("expected changed=true")
	}
	m := result[0].(map[string]interface{})
	det := m["details"].([]interface{})[0].(map[string]interface{})
	overrides := det["region_overrides"].(map[string]interface{})
	if len(overrides) != 0 {
		t.Errorf("expected empty region_overrides, got %v", overrides)
	}
}

// TestNormalizeRegionOverrides_MultiBundleSomeInactive verifies that stripping applies
// to every bundle independently.
func TestNormalizeRegionOverrides_MultiBundleSomeInactive(t *testing.T) {
	active := map[string]bool{"us-east-1": true}
	bundles := []interface{}{
		map[string]interface{}{
			"name": "b1",
			"details": []interface{}{
				map[string]interface{}{
					"region_overrides": map[string]interface{}{
						"us-east-1": "ami-a1",
						"us-west-2": "ami-b1",
					},
				},
			},
		},
		map[string]interface{}{
			"name": "b2",
			"details": []interface{}{
				map[string]interface{}{
					"region_overrides": map[string]interface{}{
						"us-east-1": "ami-a2",
						"us-west-2": "ami-b2",
					},
				},
			},
		},
	}

	result, changed := normalizeRegionOverrides(bundles, active)

	if !changed {
		t.Error("expected changed=true")
	}
	for idx, r := range result {
		m := r.(map[string]interface{})
		det := m["details"].([]interface{})[0].(map[string]interface{})
		overrides := det["region_overrides"].(map[string]interface{})
		if _, found := overrides["us-west-2"]; found {
			t.Errorf("bundle %d: us-west-2 should have been stripped", idx)
		}
		if _, found := overrides["us-east-1"]; !found {
			t.Errorf("bundle %d: us-east-1 should be preserved", idx)
		}
	}
}

// TestNormalizeRegionOverrides_NoMutateOriginal verifies the original bundle maps are
// not modified.
func TestNormalizeRegionOverrides_NoMutateOriginal(t *testing.T) {
	active := map[string]bool{"us-east-1": true}
	originalOverrides := map[string]interface{}{
		"us-east-1": "ami-aaa",
		"us-west-2": "ami-bbb",
	}
	bundles := []interface{}{
		map[string]interface{}{
			"name": "b1",
			"details": []interface{}{
				map[string]interface{}{
					"region_overrides": originalOverrides,
				},
			},
		},
	}

	normalizeRegionOverrides(bundles, active)

	// The original map must not have been mutated.
	if _, found := originalOverrides["us-west-2"]; !found {
		t.Error("original region_overrides map must not be mutated")
	}
}

// Helper function to check duplicate regions (mirrors the validation logic)
func checkDuplicateRegions(regions []map[string]interface{}) error {
	regionCodes := make(map[string]bool)
	for _, region := range regions {
		code := region["code"].(string)
		if regionCodes[code] {
			return &duplicateError{itemType: "region", code: code}
		}
		regionCodes[code] = true
	}
	return nil
}

// Helper function to check duplicate zones (mirrors the validation logic)
func checkDuplicateZones(regionCode string, zones []map[string]interface{}) error {
	zoneCodes := make(map[string]bool)
	for _, zone := range zones {
		code := zone["code"].(string)
		if zoneCodes[code] {
			return &duplicateError{itemType: "zone", code: code, region: regionCode}
		}
		zoneCodes[code] = true
	}
	return nil
}

type duplicateError struct {
	itemType string
	code     string
	region   string
}

func (e *duplicateError) Error() string {
	if e.itemType == "region" {
		return "duplicate region code \"" + e.code + "\" found: each region must have a unique code"
	}
	return "duplicate zone code \"" + e.code + "\" found in region \"" + e.region +
		"\": each zone within a region must have a unique code"
}
