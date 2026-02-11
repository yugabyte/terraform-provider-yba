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
	"testing"

	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestBuildAzureRegions(t *testing.T) {
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
					"name":              "eastus",
					"vnet":              "my-vnet",
					"security_group_id": "my-nsg",
					"zones": []interface{}{
						map[string]interface{}{
							"name":             "eastus-1",
							"subnet":           "my-subnet",
							"secondary_subnet": "",
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
					"name":              "eastus",
					"vnet":              "vnet-east",
					"security_group_id": "nsg-east",
					"zones":             []interface{}{},
				},
				map[string]interface{}{
					"name":              "westus2",
					"vnet":              "vnet-west",
					"security_group_id": "nsg-west",
					"zones":             []interface{}{},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAzureRegions(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d regions, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildAzureRegionsValues(t *testing.T) {
	testVnet := "/subscriptions/xxx/rg/Microsoft.Network/vnet/my-vnet"
	testSG := "/subscriptions/xxx/rg/Microsoft.Network/nsg/my-nsg"

	input := []interface{}{
		map[string]interface{}{
			"name":              "westus2",
			"vnet":              testVnet,
			"security_group_id": testSG,
			"zones": []interface{}{
				map[string]interface{}{
					"name":             "westus2-1",
					"subnet":           "subnet-primary",
					"secondary_subnet": "subnet-secondary",
				},
				map[string]interface{}{
					"name":             "westus2-2",
					"subnet":           "subnet-primary-2",
					"secondary_subnet": "",
				},
			},
		},
	}

	result := buildAzureRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region.GetCode() != "westus2" {
		t.Errorf("expected code 'westus2', got '%s'", region.GetCode())
	}
	if region.GetName() != "westus2" {
		t.Errorf("expected name 'westus2', got '%s'", region.GetName())
	}

	// Check Azure cloud info
	details := region.GetDetails()
	cloudInfo := details.GetCloudInfo()
	azureInfo := cloudInfo.GetAzu()
	if azureInfo.GetVnet() != testVnet {
		t.Errorf("expected vnet, got '%s'", azureInfo.GetVnet())
	}
	if azureInfo.GetSecurityGroupId() != testSG {
		t.Errorf("expected security_group_id, got '%s'", azureInfo.GetSecurityGroupId())
	}

	// Check zones
	if len(region.Zones) != 2 {
		t.Errorf("expected 2 zones, got %d", len(region.Zones))
	}
}

func TestBuildAzureZones(t *testing.T) {
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
				map[string]interface{}{
					"name":             "eastus-1",
					"subnet":           "my-subnet",
					"secondary_subnet": "",
				},
			},
			expected: 1,
		},
		{
			name: "multiple zones",
			input: []interface{}{
				map[string]interface{}{
					"name":             "eastus-1",
					"subnet":           "subnet-1",
					"secondary_subnet": "",
				},
				map[string]interface{}{
					"name":             "eastus-2",
					"subnet":           "subnet-2",
					"secondary_subnet": "",
				},
				map[string]interface{}{
					"name":             "eastus-3",
					"subnet":           "subnet-3",
					"secondary_subnet": "",
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAzureZones(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d zones, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildAzureZonesValues(t *testing.T) {
	testSubnet := "/subscriptions/xxx/rg/vnet/subnets/primary"
	testSecondarySubnet := "/subscriptions/xxx/rg/vnet/subnets/secondary"

	input := []interface{}{
		map[string]interface{}{
			"name":             "westus2-1",
			"subnet":           testSubnet,
			"secondary_subnet": testSecondarySubnet,
		},
	}

	result := buildAzureZones(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result))
	}

	zone := result[0]
	if zone.GetCode() != "westus2-1" {
		t.Errorf("expected code 'westus2-1', got '%s'", zone.GetCode())
	}
	if zone.Name != "westus2-1" {
		t.Errorf("expected name 'westus2-1', got '%s'", zone.Name)
	}
	if zone.GetSubnet() != testSubnet {
		t.Errorf("expected subnet, got '%s'", zone.GetSubnet())
	}
	if zone.GetSecondarySubnet() != testSecondarySubnet {
		t.Errorf("expected secondary_subnet, got '%s'", zone.GetSecondarySubnet())
	}
}

func TestFlattenAzureRegions(t *testing.T) {
	input := []client.Region{
		{
			Uuid: utils.GetStringPointer("region-uuid-1"),
			Code: utils.GetStringPointer("westus2"),
			Name: utils.GetStringPointer("West US 2"),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Azu: &client.AzureRegionCloudInfo{
						Vnet:            utils.GetStringPointer("my-vnet"),
						SecurityGroupId: utils.GetStringPointer("my-nsg"),
					},
				},
			},
			Zones: []client.AvailabilityZone{
				{
					Uuid:            utils.GetStringPointer("zone-uuid-1"),
					Code:            utils.GetStringPointer("westus2-1"),
					Name:            "westus2-1",
					Subnet:          utils.GetStringPointer("subnet-a"),
					SecondarySubnet: utils.GetStringPointer("subnet-b"),
				},
			},
		},
	}

	result := flattenAzureRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region["uuid"] != "region-uuid-1" {
		t.Errorf("expected uuid 'region-uuid-1', got '%v'", region["uuid"])
	}
	if region["code"] != "westus2" {
		t.Errorf("expected code 'westus2', got '%v'", region["code"])
	}
	if region["vnet"] != "my-vnet" {
		t.Errorf("expected vnet 'my-vnet', got '%v'", region["vnet"])
	}
	if region["security_group_id"] != "my-nsg" {
		t.Errorf("expected security_group_id 'my-nsg', got '%v'", region["security_group_id"])
	}

	zones := region["zones"].([]map[string]interface{})
	if len(zones) != 1 {
		t.Errorf("expected 1 zone, got %d", len(zones))
	}
}

func TestFlattenAzureZones(t *testing.T) {
	input := []client.AvailabilityZone{
		{
			Uuid:            utils.GetStringPointer("zone-uuid-1"),
			Code:            utils.GetStringPointer("westus2-1"),
			Name:            "westus2-1",
			Subnet:          utils.GetStringPointer("subnet-primary"),
			SecondarySubnet: utils.GetStringPointer("subnet-secondary"),
		},
		{
			Uuid:   utils.GetStringPointer("zone-uuid-2"),
			Code:   utils.GetStringPointer("westus2-2"),
			Name:   "westus2-2",
			Subnet: utils.GetStringPointer("subnet-b"),
		},
	}

	result := flattenAzureZones(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(result))
	}

	zone1 := result[0]
	if zone1["uuid"] != "zone-uuid-1" {
		t.Errorf("expected uuid 'zone-uuid-1', got '%v'", zone1["uuid"])
	}
	if zone1["code"] != "westus2-1" {
		t.Errorf("expected code 'westus2-1', got '%v'", zone1["code"])
	}
	if zone1["subnet"] != "subnet-primary" {
		t.Errorf("expected subnet 'subnet-primary', got '%v'", zone1["subnet"])
	}
	if zone1["secondary_subnet"] != "subnet-secondary" {
		t.Errorf("expected secondary_subnet 'subnet-secondary', got '%v'",
			zone1["secondary_subnet"])
	}
}

func TestFlattenAzureZonesEmpty(t *testing.T) {
	input := []client.AvailabilityZone{}
	result := flattenAzureZones(input)
	if len(result) != 0 {
		t.Errorf("expected 0 zones, got %d", len(result))
	}
}

func TestFlattenAzureZonesNoSecondarySubnet(t *testing.T) {
	input := []client.AvailabilityZone{
		{
			Uuid:   utils.GetStringPointer("zone-uuid-1"),
			Code:   utils.GetStringPointer("eastus-1"),
			Name:   "eastus-1",
			Subnet: utils.GetStringPointer("subnet-only"),
			// SecondarySubnet not set
		},
	}

	result := flattenAzureZones(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result))
	}

	zone := result[0]
	if zone["secondary_subnet"] != "" {
		t.Errorf("expected empty secondary_subnet, got '%v'", zone["secondary_subnet"])
	}
}
