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
	"testing"

	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestBuildRegions(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected int
	}{
		{
			name:     "empty regions",
			input:    []interface{}{},
			expected: 0,
		},
		{
			name: "single region with zones",
			input: []interface{}{
				map[string]interface{}{
					"code":      "us-west",
					"latitude":  37.7749,
					"longitude": -122.4194,
					"zones": []interface{}{
						map[string]interface{}{"code": "us-west-az1"},
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple regions",
			input: []interface{}{
				map[string]interface{}{
					"code": "us-west", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
				map[string]interface{}{
					"code": "us-east", "latitude": 0.0, "longitude": 0.0,
					"zones": []interface{}{},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildRegions(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d regions, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildRegionsValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"code":      "dc1",
			"latitude":  37.7749,
			"longitude": -122.4194,
			"zones": []interface{}{
				map[string]interface{}{"code": "dc1-rack1"},
				map[string]interface{}{"code": "dc1-rack2"},
			},
		},
	}

	result := buildRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region.GetCode() != "dc1" {
		t.Errorf("expected code 'dc1', got '%s'", region.GetCode())
	}
	if region.GetName() != "dc1" {
		t.Errorf("expected name 'dc1' (mirrors code), got '%s'", region.GetName())
	}
	if region.GetLatitude() != 37.7749 {
		t.Errorf("expected latitude 37.7749, got %f", region.GetLatitude())
	}
	if region.GetLongitude() != -122.4194 {
		t.Errorf("expected longitude -122.4194, got %f", region.GetLongitude())
	}
	if len(region.Zones) != 2 {
		t.Errorf("expected 2 zones, got %d", len(region.Zones))
	}
}

func TestBuildZones(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected int
	}{
		{
			name:     "empty zones",
			input:    []interface{}{},
			expected: 0,
		},
		{
			name: "single zone",
			input: []interface{}{
				map[string]interface{}{"code": "us-west-az1"},
			},
			expected: 1,
		},
		{
			name: "multiple zones",
			input: []interface{}{
				map[string]interface{}{"code": "dc1-rack1"},
				map[string]interface{}{"code": "dc1-rack2"},
				map[string]interface{}{"code": "dc1-rack3"},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildZones(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d zones, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildZonesValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{"code": "dc1-rack1"},
	}

	result := buildZones(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result))
	}

	zone := result[0]
	if zone.GetCode() != "dc1-rack1" {
		t.Errorf("expected code 'dc1-rack1', got '%s'", zone.GetCode())
	}
	if zone.Name != "dc1-rack1" {
		t.Errorf("expected name 'dc1-rack1' (mirrors code), got '%s'", zone.Name)
	}
}

func TestFlattenRegions(t *testing.T) {
	input := []client.Region{
		{
			Uuid:      utils.GetStringPointer("region-uuid-1"),
			Code:      utils.GetStringPointer("us-west"),
			Name:      utils.GetStringPointer("us-west"),
			Latitude:  utils.GetFloat64Pointer(37.7749),
			Longitude: utils.GetFloat64Pointer(-122.4194),
			Zones: []client.AvailabilityZone{
				{
					Uuid: utils.GetStringPointer("zone-uuid-1"),
					Code: utils.GetStringPointer("us-west-az1"),
					Name: "us-west-az1",
				},
				{
					Uuid: utils.GetStringPointer("zone-uuid-2"),
					Code: utils.GetStringPointer("us-west-az2"),
					Name: "us-west-az2",
				},
			},
		},
	}

	result := flattenRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region["uuid"] != "region-uuid-1" {
		t.Errorf("expected uuid 'region-uuid-1', got '%v'", region["uuid"])
	}
	if region["code"] != "us-west" {
		t.Errorf("expected code 'us-west', got '%v'", region["code"])
	}
	if region["name"] != "us-west" {
		t.Errorf("expected name 'us-west' (mirrors code), got '%v'", region["name"])
	}
	if region["latitude"] != 37.7749 {
		t.Errorf("expected latitude 37.7749, got '%v'", region["latitude"])
	}

	zones := region["zones"].([]map[string]interface{})
	if len(zones) != 2 {
		t.Errorf("expected 2 zones, got %d", len(zones))
	}
}

func TestFlattenZones(t *testing.T) {
	input := []client.AvailabilityZone{
		{
			Uuid: utils.GetStringPointer("zone-uuid-1"),
			Code: utils.GetStringPointer("dc1-rack1"),
			Name: "dc1-rack1",
		},
		{
			Uuid: utils.GetStringPointer("zone-uuid-2"),
			Code: utils.GetStringPointer("dc1-rack2"),
			Name: "dc1-rack2",
		},
	}

	result := flattenZones(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(result))
	}

	zone1 := result[0]
	if zone1["uuid"] != "zone-uuid-1" {
		t.Errorf("expected uuid 'zone-uuid-1', got '%v'", zone1["uuid"])
	}
	if zone1["code"] != "dc1-rack1" {
		t.Errorf("expected code 'dc1-rack1', got '%v'", zone1["code"])
	}
	if zone1["name"] != "dc1-rack1" {
		t.Errorf("expected name 'dc1-rack1' (mirrors code), got '%v'", zone1["name"])
	}
}

func TestFlattenZonesEmpty(t *testing.T) {
	result := flattenZones([]client.AvailabilityZone{})
	if len(result) != 0 {
		t.Errorf("expected 0 zones, got %d", len(result))
	}
}
