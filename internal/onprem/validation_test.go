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
	"strings"
	"testing"
)

func TestValidateOnpremDuplicateRegions(t *testing.T) {
	tests := []struct {
		name        string
		regions     []map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "no duplicates - single region",
			regions: []map[string]interface{}{
				{"code": "us-west", "zones": []interface{}{}},
			},
			expectError: false,
		},
		{
			name: "no duplicates - multiple regions",
			regions: []map[string]interface{}{
				{"code": "us-west", "zones": []interface{}{}},
				{"code": "us-east", "zones": []interface{}{}},
				{"code": "eu-central", "zones": []interface{}{}},
			},
			expectError: false,
		},
		{
			name: "duplicate region codes",
			regions: []map[string]interface{}{
				{"code": "dc1", "zones": []interface{}{}},
				{"code": "dc1", "zones": []interface{}{}},
			},
			expectError: true,
			errorMsg:    "duplicate region code \"dc1\"",
		},
		{
			name: "duplicate among multiple regions",
			regions: []map[string]interface{}{
				{"code": "us-west", "zones": []interface{}{}},
				{"code": "us-east", "zones": []interface{}{}},
				{"code": "us-west", "zones": []interface{}{}},
			},
			expectError: true,
			errorMsg:    "duplicate region code \"us-west\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkOnpremDuplicateRegions(tt.regions)
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

func TestValidateOnpremDuplicateZones(t *testing.T) {
	tests := []struct {
		name        string
		regionCode  string
		zones       []map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "no duplicates - single zone",
			regionCode:  "dc1",
			zones:       []map[string]interface{}{{"code": "dc1-rack1"}},
			expectError: false,
		},
		{
			name:       "no duplicates - multiple zones",
			regionCode: "dc1",
			zones: []map[string]interface{}{
				{"code": "dc1-rack1"},
				{"code": "dc1-rack2"},
				{"code": "dc1-rack3"},
			},
			expectError: false,
		},
		{
			name:       "duplicate zone codes",
			regionCode: "dc1",
			zones: []map[string]interface{}{
				{"code": "dc1-rack1"},
				{"code": "dc1-rack1"},
			},
			expectError: true,
			errorMsg:    "duplicate zone code \"dc1-rack1\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkOnpremDuplicateZones(tt.regionCode, tt.zones)
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

func TestOnpremRegionsContentChanged(t *testing.T) {
	makeRegions := func(entries ...map[string]interface{}) []interface{} {
		var result []interface{}
		for _, e := range entries {
			result = append(result, e)
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
			name: "identical",
			old: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{
						map[string]interface{}{"code": "dc1-rack1"},
					},
				},
			),
			new: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{
						map[string]interface{}{"code": "dc1-rack1"},
					},
				},
			),
			changed: false,
		},
		{
			name: "reordered only",
			old: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
				map[string]interface{}{
					"code": "dc2", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
			),
			new: makeRegions(
				map[string]interface{}{
					"code": "dc2", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
			),
			changed: false,
		},
		{
			name: "region added",
			old: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
			),
			new: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
				map[string]interface{}{
					"code": "dc2", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
			),
			changed: true,
		},
		{
			name: "latitude changed",
			old: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 37.7749, "longitude": -122.4194,
					"zones": []interface{}{},
				},
			),
			new: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 40.7128, "longitude": -74.0060,
					"zones": []interface{}{},
				},
			),
			changed: true,
		},
		{
			name: "zone added",
			old: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{
						map[string]interface{}{"code": "dc1-rack1"},
					},
				},
			),
			new: makeRegions(
				map[string]interface{}{
					"code": "dc1", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{
						map[string]interface{}{"code": "dc1-rack1"},
						map[string]interface{}{"code": "dc1-rack2"},
					},
				},
			),
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := onpremRegionsContentChanged(tt.old, tt.new)
			if got != tt.changed {
				t.Errorf("onpremRegionsContentChanged: expected %v, got %v", tt.changed, got)
			}
		})
	}
}

func checkOnpremDuplicateRegions(regions []map[string]interface{}) error {
	seen := make(map[string]bool)
	for _, region := range regions {
		code := region["code"].(string)
		if seen[code] {
			return &onpremDuplicateError{itemType: "region", code: code}
		}
		seen[code] = true
	}
	return nil
}

func checkOnpremDuplicateZones(regionCode string, zones []map[string]interface{}) error {
	seen := make(map[string]bool)
	for _, zone := range zones {
		code := zone["code"].(string)
		if seen[code] {
			return &onpremDuplicateError{itemType: "zone", code: code, region: regionCode}
		}
		seen[code] = true
	}
	return nil
}

type onpremDuplicateError struct {
	itemType string
	code     string
	region   string
}

func (e *onpremDuplicateError) Error() string {
	if e.itemType == "region" {
		return "duplicate region code \"" + e.code + "\" found: each region must have a unique code"
	}
	return "duplicate zone code \"" + e.code + "\" found in region \"" + e.region +
		"\": each zone within a region must have a unique code"
}
