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
	"testing"

	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestBuildAWSRegions(t *testing.T) {
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
					"code":              "us-west-2",
					"security_group_id": "sg-12345",
					"vpc_id":            "vpc-12345",
					"zones": []interface{}{
						map[string]interface{}{
							"code":             "us-west-2a",
							"subnet":           "subnet-a",
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
					"code":              "us-west-2",
					"security_group_id": "sg-west",
					"vpc_id":            "vpc-west",
					"zones":             []interface{}{},
				},
				map[string]interface{}{
					"code":              "us-east-1",
					"security_group_id": "sg-east",
					"vpc_id":            "vpc-east",
					"zones":             []interface{}{},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAWSRegions(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d regions, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildAWSRegionsValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"code":              "us-west-2",
			"security_group_id": "sg-test",
			"vpc_id":            "vpc-test",
			"zones": []interface{}{
				map[string]interface{}{
					"code":             "us-west-2a",
					"subnet":           "subnet-primary",
					"secondary_subnet": "subnet-secondary",
				},
				map[string]interface{}{
					"code":             "us-west-2b",
					"subnet":           "subnet-primary-b",
					"secondary_subnet": "",
				},
			},
		},
	}

	result := buildAWSRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region.GetCode() != "us-west-2" {
		t.Errorf("expected code 'us-west-2', got '%s'", region.GetCode())
	}
	if region.GetName() != "us-west-2" {
		t.Errorf("expected name 'us-west-2', got '%s'", region.GetName())
	}

	// Check AWS cloud info
	details := region.GetDetails()
	cloudInfo := details.GetCloudInfo()
	awsInfo := cloudInfo.GetAws()
	if awsInfo.GetSecurityGroupId() != "sg-test" {
		t.Errorf("expected security_group_id 'sg-test', got '%s'", awsInfo.GetSecurityGroupId())
	}
	if awsInfo.GetVnet() != "vpc-test" {
		t.Errorf("expected vpc_id 'vpc-test', got '%s'", awsInfo.GetVnet())
	}

	// Check zones
	if len(region.Zones) != 2 {
		t.Errorf("expected 2 zones, got %d", len(region.Zones))
	}
}

func TestBuildAWSZones(t *testing.T) {
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
					"code":             "us-west-2a",
					"subnet":           "subnet-123",
					"secondary_subnet": "subnet-456",
				},
			},
			expected: 1,
		},
		{
			name: "multiple zones",
			input: []interface{}{
				map[string]interface{}{
					"code":             "us-west-2a",
					"subnet":           "subnet-a",
					"secondary_subnet": "",
				},
				map[string]interface{}{
					"code":             "us-west-2b",
					"subnet":           "subnet-b",
					"secondary_subnet": "",
				},
				map[string]interface{}{
					"code":             "us-west-2c",
					"subnet":           "subnet-c",
					"secondary_subnet": "",
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAWSZones(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d zones, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildAWSZonesValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"code":             "us-west-2a",
			"subnet":           "subnet-primary",
			"secondary_subnet": "subnet-secondary",
		},
	}

	result := buildAWSZones(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result))
	}

	zone := result[0]
	if zone.GetCode() != "us-west-2a" {
		t.Errorf("expected code 'us-west-2a', got '%s'", zone.GetCode())
	}
	// Name is set to code value in the API request
	if zone.Name != "us-west-2a" {
		t.Errorf("expected name 'us-west-2a', got '%s'", zone.Name)
	}
	if zone.GetSubnet() != "subnet-primary" {
		t.Errorf("expected subnet 'subnet-primary', got '%s'", zone.GetSubnet())
	}
	if zone.GetSecondarySubnet() != "subnet-secondary" {
		t.Errorf("expected secondary_subnet 'subnet-secondary', got '%s'",
			zone.GetSecondarySubnet())
	}
}

func TestFlattenAWSRegions(t *testing.T) {
	input := []client.Region{
		{
			Uuid: utils.GetStringPointer("region-uuid-1"),
			Code: utils.GetStringPointer("us-west-2"),
			Name: utils.GetStringPointer("US West 2"),
			Details: &client.RegionDetails{
				CloudInfo: &client.RegionCloudInfo{
					Aws: &client.AWSRegionCloudInfo{
						Vnet:            utils.GetStringPointer("vpc-12345"),
						SecurityGroupId: utils.GetStringPointer("sg-12345"),
					},
				},
			},
			Zones: []client.AvailabilityZone{
				{
					Uuid:   utils.GetStringPointer("zone-uuid-1"),
					Code:   utils.GetStringPointer("us-west-2a"),
					Name:   "us-west-2a",
					Subnet: utils.GetStringPointer("subnet-a"),
				},
			},
		},
	}

	result := flattenAWSRegions(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 region, got %d", len(result))
	}

	region := result[0]
	if region["uuid"] != "region-uuid-1" {
		t.Errorf("expected uuid 'region-uuid-1', got '%v'", region["uuid"])
	}
	if region["code"] != "us-west-2" {
		t.Errorf("expected code 'us-west-2', got '%v'", region["code"])
	}
	if region["vpc_id"] != "vpc-12345" {
		t.Errorf("expected vpc_id 'vpc-12345', got '%v'", region["vpc_id"])
	}
	if region["security_group_id"] != "sg-12345" {
		t.Errorf("expected security_group_id 'sg-12345', got '%v'", region["security_group_id"])
	}

	zones := region["zones"].([]map[string]interface{})
	if len(zones) != 1 {
		t.Errorf("expected 1 zone, got %d", len(zones))
	}
}

func TestFlattenAWSZones(t *testing.T) {
	input := []client.AvailabilityZone{
		{
			Uuid:            utils.GetStringPointer("zone-uuid-1"),
			Code:            utils.GetStringPointer("us-west-2a"),
			Name:            "us-west-2a",
			Subnet:          utils.GetStringPointer("subnet-primary"),
			SecondarySubnet: utils.GetStringPointer("subnet-secondary"),
		},
		{
			Uuid:   utils.GetStringPointer("zone-uuid-2"),
			Code:   utils.GetStringPointer("us-west-2b"),
			Name:   "us-west-2b",
			Subnet: utils.GetStringPointer("subnet-b"),
		},
	}

	result := flattenAWSZones(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(result))
	}

	zone1 := result[0]
	if zone1["uuid"] != "zone-uuid-1" {
		t.Errorf("expected uuid 'zone-uuid-1', got '%v'", zone1["uuid"])
	}
	if zone1["code"] != "us-west-2a" {
		t.Errorf("expected code 'us-west-2a', got '%v'", zone1["code"])
	}
	if zone1["subnet"] != "subnet-primary" {
		t.Errorf("expected subnet 'subnet-primary', got '%v'", zone1["subnet"])
	}
	if zone1["secondary_subnet"] != "subnet-secondary" {
		t.Errorf("expected secondary_subnet 'subnet-secondary', got '%v'",
			zone1["secondary_subnet"])
	}
}

func TestBuildAWSImageBundles(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected int
	}{
		{
			name:     "empty bundles",
			input:    []interface{}{},
			expected: 0,
		},
		{
			name: "bundle without details skipped",
			input: []interface{}{
				map[string]interface{}{
					"name":           "test-bundle",
					"use_as_default": true,
					"details":        []interface{}{},
				},
			},
			expected: 0,
		},
		{
			name: "single bundle with details",
			input: []interface{}{
				map[string]interface{}{
					"name":           "test-bundle",
					"use_as_default": true,
					"details": []interface{}{
						map[string]interface{}{
							"arch":             "x86_64",
							"ssh_user":         "ec2-user",
							"ssh_port":         22,
							"use_imds_v2":      true,
							"global_yb_image":  "ami-12345",
							"region_overrides": map[string]interface{}{},
						},
					},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAWSImageBundles(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d bundles, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildAWSImageBundlesValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"name":           "aws-custom-bundle",
			"use_as_default": true,
			"details": []interface{}{
				map[string]interface{}{
					"arch":            "x86_64",
					"ssh_user":        "centos",
					"ssh_port":        22,
					"use_imds_v2":     true,
					"global_yb_image": "ami-global-123",
					"region_overrides": map[string]interface{}{
						"us-west-2": "ami-west-123",
						"us-east-1": "ami-east-123",
					},
				},
			},
		},
	}

	result := buildAWSImageBundles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(result))
	}

	bundle := result[0]
	if bundle.GetName() != "aws-custom-bundle" {
		t.Errorf("expected name 'aws-custom-bundle', got '%s'", bundle.GetName())
	}
	if !bundle.GetUseAsDefault() {
		t.Error("expected use_as_default to be true")
	}

	details := bundle.GetDetails()
	if details.GetArch() != "x86_64" {
		t.Errorf("expected arch 'x86_64', got '%s'", details.GetArch())
	}
	if details.GetSshUser() != "centos" {
		t.Errorf("expected ssh_user 'centos', got '%s'", details.GetSshUser())
	}
	if details.GetSshPort() != 22 {
		t.Errorf("expected ssh_port 22, got %d", details.GetSshPort())
	}
	if !details.GetUseIMDSv2() {
		t.Error("expected use_imds_v2 to be true")
	}
	if details.GetGlobalYbImage() != "ami-global-123" {
		t.Errorf("expected global_yb_image 'ami-global-123', got '%s'", details.GetGlobalYbImage())
	}

	regions := details.GetRegions()
	if len(regions) != 2 {
		t.Errorf("expected 2 region overrides, got %d", len(regions))
	}
	usWest := regions["us-west-2"]
	if usWest.GetYbImage() != "ami-west-123" {
		t.Errorf("expected us-west-2 AMI 'ami-west-123', got '%s'",
			usWest.GetYbImage())
	}
	usEast := regions["us-east-1"]
	if usEast.GetYbImage() != "ami-east-123" {
		t.Errorf("expected us-east-1 AMI 'ami-east-123', got '%s'",
			usEast.GetYbImage())
	}
}

func TestBuildAWSImageBundlesUseIMDSv2False(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"name":           "bundle-no-imdsv2",
			"use_as_default": false,
			"details": []interface{}{
				map[string]interface{}{
					"arch":        "x86_64",
					"ssh_user":    "centos",
					"ssh_port":    22,
					"use_imds_v2": false,
				},
			},
		},
	}

	result := buildAWSImageBundles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(result))
	}

	details := result[0].GetDetails()
	if details.GetUseIMDSv2() {
		t.Error("expected use_imds_v2 to be false")
	}
}

func TestFlattenAWSImageBundles(t *testing.T) {
	input := []client.ImageBundle{
		{
			Uuid:         utils.GetStringPointer("bundle-uuid-1"),
			Name:         utils.GetStringPointer("test-bundle"),
			UseAsDefault: utils.GetBoolPointer(true),
			Details: &client.ImageBundleDetails{
				Arch:          utils.GetStringPointer("x86_64"),
				SshUser:       utils.GetStringPointer("ec2-user"),
				SshPort:       utils.GetInt32Pointer(22),
				UseIMDSv2:     utils.GetBoolPointer(true),
				GlobalYbImage: utils.GetStringPointer("ami-global"),
				Regions: &map[string]client.BundleInfo{
					"us-west-2": {YbImage: utils.GetStringPointer("ami-west")},
					"us-east-1": {YbImage: utils.GetStringPointer("ami-east")},
				},
			},
		},
	}

	result := flattenAWSImageBundles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(result))
	}

	bundle := result[0]
	if bundle["uuid"] != "bundle-uuid-1" {
		t.Errorf("expected uuid 'bundle-uuid-1', got '%v'", bundle["uuid"])
	}
	if bundle["name"] != "test-bundle" {
		t.Errorf("expected name 'test-bundle', got '%v'", bundle["name"])
	}
	if bundle["use_as_default"] != true {
		t.Error("expected use_as_default to be true")
	}

	detailsList := bundle["details"].([]interface{})
	if len(detailsList) != 1 {
		t.Fatalf("expected 1 details entry, got %d", len(detailsList))
	}

	details := detailsList[0].(map[string]interface{})
	if details["arch"] != "x86_64" {
		t.Errorf("expected arch 'x86_64', got '%v'", details["arch"])
	}
	if details["ssh_user"] != "ec2-user" {
		t.Errorf("expected ssh_user 'ec2-user', got '%v'", details["ssh_user"])
	}
	if details["ssh_port"] != int32(22) {
		t.Errorf("expected ssh_port 22, got '%v'", details["ssh_port"])
	}
	if details["use_imds_v2"] != true {
		t.Error("expected use_imds_v2 to be true")
	}
	if details["global_yb_image"] != "ami-global" {
		t.Errorf("expected global_yb_image 'ami-global', got '%v'", details["global_yb_image"])
	}

	regionOverrides := details["region_overrides"].(map[string]interface{})
	if len(regionOverrides) != 2 {
		t.Errorf("expected 2 region overrides, got %d", len(regionOverrides))
	}
	if regionOverrides["us-west-2"] != "ami-west" {
		t.Errorf("expected us-west-2 AMI 'ami-west', got '%v'", regionOverrides["us-west-2"])
	}
}

func TestFlattenAWSImageBundlesNoRegionOverrides(t *testing.T) {
	input := []client.ImageBundle{
		{
			Uuid:         utils.GetStringPointer("bundle-uuid-1"),
			Name:         utils.GetStringPointer("simple-bundle"),
			UseAsDefault: utils.GetBoolPointer(false),
			Details: &client.ImageBundleDetails{
				Arch:          utils.GetStringPointer("aarch64"),
				SshUser:       utils.GetStringPointer("ubuntu"),
				SshPort:       utils.GetInt32Pointer(22),
				GlobalYbImage: utils.GetStringPointer("ami-arm"),
			},
		},
	}

	result := flattenAWSImageBundles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(result))
	}

	bundle := result[0]
	detailsList := bundle["details"].([]interface{})
	details := detailsList[0].(map[string]interface{})

	// region_overrides should not be set when empty
	if _, ok := details["region_overrides"]; ok {
		t.Error("expected region_overrides to not be set for empty overrides")
	}
}

// Helper function to create a test image bundle
func createTestImageBundle(
	uuid, name, arch string,
	useAsDefault bool,
	bundleType string,
) client.ImageBundle {
	bundle := client.ImageBundle{
		Uuid:         utils.GetStringPointer(uuid),
		Name:         utils.GetStringPointer(name),
		UseAsDefault: utils.GetBoolPointer(useAsDefault),
		Details: &client.ImageBundleDetails{
			Arch:    utils.GetStringPointer(arch),
			SshUser: utils.GetStringPointer("ec2-user"),
			SshPort: utils.GetInt32Pointer(22),
		},
		Metadata: &client.Metadata{
			Type: bundleType,
		},
	}
	return bundle
}
