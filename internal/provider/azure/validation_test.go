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

package azure

import (
	"strings"
	"testing"
)

// TestValidateAzureDuplicateRegions tests detection of duplicate region codes.
func TestValidateAzureDuplicateRegions(t *testing.T) {
	tests := []struct {
		name        string
		regions     []map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "no duplicates - single region",
			regions: []map[string]interface{}{
				{"code": "eastus", "zones": []interface{}{}},
			},
			expectError: false,
		},
		{
			name: "no duplicates - multiple regions",
			regions: []map[string]interface{}{
				{"code": "eastus", "zones": []interface{}{}},
				{"code": "westus2", "zones": []interface{}{}},
				{"code": "northeurope", "zones": []interface{}{}},
			},
			expectError: false,
		},
		{
			name: "duplicate region codes",
			regions: []map[string]interface{}{
				{"code": "eastus", "zones": []interface{}{}},
				{"code": "eastus", "zones": []interface{}{}},
			},
			expectError: true,
			errorMsg:    "duplicate region code \"eastus\"",
		},
		{
			name: "duplicate among multiple regions",
			regions: []map[string]interface{}{
				{"code": "eastus", "zones": []interface{}{}},
				{"code": "westus2", "zones": []interface{}{}},
				{"code": "eastus", "zones": []interface{}{}},
			},
			expectError: true,
			errorMsg:    "duplicate region code \"eastus\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAzureDuplicateRegions(tt.regions)
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

// TestValidateAzureDuplicateZones tests detection of duplicate zone codes within a region.
func TestValidateAzureDuplicateZones(t *testing.T) {
	tests := []struct {
		name        string
		regionCode  string
		zones       []map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "no duplicates - single zone",
			regionCode:  "eastus",
			zones:       []map[string]interface{}{{"code": "eastus-1"}},
			expectError: false,
		},
		{
			name:       "no duplicates - multiple zones",
			regionCode: "eastus",
			zones: []map[string]interface{}{
				{"code": "eastus-1"},
				{"code": "eastus-2"},
				{"code": "eastus-3"},
			},
			expectError: false,
		},
		{
			name:       "duplicate zone codes",
			regionCode: "eastus",
			zones: []map[string]interface{}{
				{"code": "eastus-1"},
				{"code": "eastus-1"},
			},
			expectError: true,
			errorMsg:    "duplicate zone code \"eastus-1\"",
		},
		{
			name:       "duplicate among multiple zones",
			regionCode: "westus2",
			zones: []map[string]interface{}{
				{"code": "westus2-1"},
				{"code": "westus2-2"},
				{"code": "westus2-1"},
			},
			expectError: true,
			errorMsg:    "duplicate zone code \"westus2-1\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAzureDuplicateZones(tt.regionCode, tt.zones)
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

// TestRegionsContentChanged tests that content changes are correctly detected.
func TestAzureRegionsContentChanged(t *testing.T) {
	makeRegions := func(codes ...string) []interface{} {
		var result []interface{}
		for _, code := range codes {
			result = append(result, map[string]interface{}{
				"code":              code,
				"vnet":              "vnet-" + code,
				"security_group_id": "",
				"zones":             []interface{}{},
			})
		}
		return result
	}

	tests := []struct {
		name    string
		old     []interface{}
		new     []interface{}
		changed bool
	}{
		{
			name:    "identical",
			old:     makeRegions("eastus", "westus2"),
			new:     makeRegions("eastus", "westus2"),
			changed: false,
		},
		{
			name:    "reordered only",
			old:     makeRegions("eastus", "westus2"),
			new:     makeRegions("westus2", "eastus"),
			changed: false,
		},
		{
			name:    "region added",
			old:     makeRegions("eastus"),
			new:     makeRegions("eastus", "westus2"),
			changed: true,
		},
		{
			name:    "region removed",
			old:     makeRegions("eastus", "westus2"),
			new:     makeRegions("eastus"),
			changed: true,
		},
		{
			name: "vnet changed",
			old: []interface{}{
				map[string]interface{}{
					"code": "eastus", "vnet": "old-vnet",
					"security_group_id": "", "zones": []interface{}{},
				},
			},
			new: []interface{}{
				map[string]interface{}{
					"code": "eastus", "vnet": "new-vnet",
					"security_group_id": "", "zones": []interface{}{},
				},
			},
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := regionsContentChanged(tt.old, tt.new)
			if got != tt.changed {
				t.Errorf("regionsContentChanged: expected %v, got %v", tt.changed, got)
			}
		})
	}
}

// checkAzureDuplicateRegions mirrors the region duplicate check in validateAzureProvider.
func checkAzureDuplicateRegions(regions []map[string]interface{}) error {
	seen := make(map[string]bool)
	for _, region := range regions {
		code := region["code"].(string)
		if seen[code] {
			return &azureDuplicateError{itemType: "region", code: code}
		}
		seen[code] = true
	}
	return nil
}

// checkAzureDuplicateZones mirrors the zone duplicate check in validateAzureProvider.
func checkAzureDuplicateZones(regionCode string, zones []map[string]interface{}) error {
	seen := make(map[string]bool)
	for _, zone := range zones {
		code := zone["code"].(string)
		if seen[code] {
			return &azureDuplicateError{itemType: "zone", code: code, region: regionCode}
		}
		seen[code] = true
	}
	return nil
}

type azureDuplicateError struct {
	itemType string
	code     string
	region   string
}

func (e *azureDuplicateError) Error() string {
	if e.itemType == "region" {
		return "duplicate region code \"" + e.code + "\" found: each region must have a unique code"
	}
	return "duplicate zone code \"" + e.code + "\" found in region \"" + e.region +
		"\": each zone within a region must have a unique code"
}
