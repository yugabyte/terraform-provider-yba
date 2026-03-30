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
	"testing"

	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestBuildGCPRegions(t *testing.T) {
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
					"code":              "us-west1",
					"instance_template": "",
					"zones": []interface{}{
						map[string]interface{}{
							"code":   "us-west1-a",
							"subnet": "default",
						},
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple regions",
			input: []interface{}{
				map[string]interface{}{
					"code":              "us-west1",
					"instance_template": "",
					"zones":             []interface{}{},
				},
				map[string]interface{}{
					"code":              "us-east1",
					"instance_template": "projects/my-project/global/instanceTemplates/my-template",
					"zones":             []interface{}{},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildGCPRegions(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d regions, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildGCPRegionsValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"code":              "us-west1",
			"instance_template": "projects/my-project/global/instanceTemplates/my-template",
			"zones": []interface{}{
				map[string]interface{}{
					"code":   "us-west1-a",
					"subnet": "subnet-west-a",
				},
				map[string]interface{}{
					"code":   "us-west1-b",
					"subnet": "subnet-west-b",
				},
			},
		},
	}

	result := buildGCPRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region.GetCode() != "us-west1" {
		t.Errorf("expected code 'us-west1', got '%s'", region.GetCode())
	}
	if region.GetName() != "us-west1" {
		t.Errorf("expected name 'us-west1', got '%s'", region.GetName())
	}

	// Check GCP cloud info
	details := region.GetDetails()
	cloudInfo := details.GetCloudInfo()
	gcpInfo := cloudInfo.GetGcp()
	if gcpInfo.GetInstanceTemplate() != "projects/my-project/global/instanceTemplates/my-template" {
		t.Errorf("expected instance_template, got '%s'", gcpInfo.GetInstanceTemplate())
	}

	// Check zones - buildGCPZones creates 1 zone entry with shared_subnet
	// YBA will expand to all zones in the region
	if len(region.Zones) != 1 {
		t.Errorf("expected 1 zone entry (YBA expands), got %d", len(region.Zones))
	}
}

func TestBuildGCPZones(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected int
	}{
		{
			name: "region with shared_subnet",
			input: map[string]interface{}{
				"code":          "us-west1",
				"shared_subnet": "default",
			},
			expected: 1, // buildGCPZones creates a single zone entry for YBA to expand
		},
		{
			name: "region without shared_subnet",
			input: map[string]interface{}{
				"code": "us-west1",
			},
			expected: 1, // Still creates a zone entry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := tt.input["code"].(string)
			result := buildGCPZones(code, tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d zones, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildGCPZonesValues(t *testing.T) {
	input := map[string]interface{}{
		"code":          "us-west1",
		"shared_subnet": "projects/my-project/regions/us-west1/subnetworks/my-subnet",
	}

	result := buildGCPZones("us-west1", input)
	if len(result) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result))
	}

	zone := result[0]
	if zone.GetCode() != "us-west1" {
		t.Errorf("expected zone code 'us-west1', got '%s'", zone.GetCode())
	}
	if zone.GetSubnet() != "projects/my-project/regions/us-west1/subnetworks/my-subnet" {
		t.Errorf("expected subnet, got '%s'", zone.GetSubnet())
	}
}

func TestFlattenGCPRegions(t *testing.T) {
	input := []client.Region{
		{
			Uuid: utils.GetStringPointer("region-uuid-1"),
			Code: utils.GetStringPointer("us-west1"),
			Name: utils.GetStringPointer("US West 1"),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Gcp: &client.GCPRegionCloudInfo{
						InstanceTemplate: utils.GetStringPointer("my-template"),
					},
				},
			},
			Zones: []client.AvailabilityZone{
				{
					Uuid:   utils.GetStringPointer("zone-uuid-1"),
					Code:   utils.GetStringPointer("us-west1-a"),
					Name:   "us-west1-a",
					Subnet: utils.GetStringPointer("default"),
				},
			},
		},
	}

	result := flattenGCPRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region["uuid"] != "region-uuid-1" {
		t.Errorf("expected uuid 'region-uuid-1', got '%v'", region["uuid"])
	}
	if region["code"] != "us-west1" {
		t.Errorf("expected code 'us-west1', got '%v'", region["code"])
	}
	if region["instance_template"] != "my-template" {
		t.Errorf("expected instance_template 'my-template', got '%v'", region["instance_template"])
	}

	zones := region["zones"].([]map[string]interface{})
	if len(zones) != 1 {
		t.Errorf("expected 1 zone, got %d", len(zones))
	}
}

func TestFlattenGCPZones(t *testing.T) {
	input := []client.AvailabilityZone{
		{
			Uuid:   utils.GetStringPointer("zone-uuid-1"),
			Code:   utils.GetStringPointer("us-west1-a"),
			Name:   "us-west1-a",
			Subnet: utils.GetStringPointer("subnet-a"),
		},
		{
			Uuid:   utils.GetStringPointer("zone-uuid-2"),
			Code:   utils.GetStringPointer("us-west1-b"),
			Name:   "us-west1-b",
			Subnet: utils.GetStringPointer("subnet-b"),
		},
	}

	result := flattenGCPZones(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(result))
	}

	zone1 := result[0]
	if zone1["uuid"] != "zone-uuid-1" {
		t.Errorf("expected uuid 'zone-uuid-1', got '%v'", zone1["uuid"])
	}
	if zone1["code"] != "us-west1-a" {
		t.Errorf("expected code 'us-west1-a', got '%v'", zone1["code"])
	}
	if zone1["subnet"] != "subnet-a" {
		t.Errorf("expected subnet 'subnet-a', got '%v'", zone1["subnet"])
	}
}

func TestFlattenGCPZonesEmpty(t *testing.T) {
	input := []client.AvailabilityZone{}
	result := flattenGCPZones(input)
	if len(result) != 0 {
		t.Errorf("expected 0 zones, got %d", len(result))
	}
}

func TestBuildGCPRegionsWithInstanceTemplate(t *testing.T) {
	// Test that instance template is properly set
	input := []interface{}{
		map[string]interface{}{
			"code":              "europe-west1",
			"instance_template": "projects/test-project/global/instanceTemplates/test-template",
			"zones": []interface{}{
				map[string]interface{}{
					"code":   "europe-west1-b",
					"subnet": "default",
				},
			},
		},
	}

	result := buildGCPRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	details := region.GetDetails()
	cloudInfo := details.GetCloudInfo()
	gcpInfo := cloudInfo.GetGcp()

	expected := "projects/test-project/global/instanceTemplates/test-template"
	if gcpInfo.GetInstanceTemplate() != expected {
		t.Errorf(
			"expected instance_template '%s', got '%s'",
			expected,
			gcpInfo.GetInstanceTemplate(),
		)
	}
}
