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

	client "github.com/yugabyte/platform-go-client"
)

func TestBuildAccessKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected int
	}{
		{
			name:     "empty input",
			input:    []interface{}{},
			expected: 0,
		},
		{
			name: "single access key",
			input: []interface{}{
				map[string]interface{}{
					"key_pair_name":           "my-key",
					"ssh_private_key_content": "-----BEGIN RSA KEY-----\ntest\n-----END RSA KEY-----",
					"skip_key_validation":     true,
				},
			},
			expected: 1,
		},
		{
			name: "multiple access keys",
			input: []interface{}{
				map[string]interface{}{
					"key_pair_name":           "key1",
					"ssh_private_key_content": "content1",
					"skip_key_validation":     false,
				},
				map[string]interface{}{
					"key_pair_name":           "key2",
					"ssh_private_key_content": "content2",
					"skip_key_validation":     true,
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildAccessKeys(tt.input)
			if tt.expected == 0 && result != nil {
				t.Errorf("expected nil, got %v", result)
			}
			if tt.expected > 0 && len(result) != tt.expected {
				t.Errorf("expected %d access keys, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildAccessKeysValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"key_pair_name":           "test-key",
			"ssh_private_key_content": "ssh-content",
			"skip_key_validation":     true,
		},
	}

	result := BuildAccessKeys(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 access key, got %d", len(result))
	}

	ak := result[0]
	if ak.KeyInfo.GetKeyPairName() != "test-key" {
		t.Errorf("expected key_pair_name 'test-key', got '%s'", ak.KeyInfo.GetKeyPairName())
	}
	if ak.KeyInfo.GetSshPrivateKeyContent() != "ssh-content" {
		t.Errorf("expected ssh_private_key_content 'ssh-content', got '%s'",
			ak.KeyInfo.GetSshPrivateKeyContent())
	}
	if !ak.KeyInfo.GetSkipKeyValidateAndUpload() {
		t.Error("expected skip_key_validation to be true")
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
				map[string]interface{}{
					"code":             "us-west-2a",
					"name":             "us-west-2a",
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
					"name":             "us-west-2a",
					"subnet":           "subnet-a",
					"secondary_subnet": "",
				},
				map[string]interface{}{
					"code":             "us-west-2b",
					"name":             "us-west-2b",
					"subnet":           "subnet-b",
					"secondary_subnet": "",
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildZones(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d zones, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildZonesValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"code":             "zone-1",
			"name":             "Zone One",
			"subnet":           "subnet-primary",
			"secondary_subnet": "subnet-secondary",
		},
	}

	result := BuildZones(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result))
	}

	zone := result[0]
	if zone.GetCode() != "zone-1" {
		t.Errorf("expected code 'zone-1', got '%s'", zone.GetCode())
	}
	if zone.Name != "Zone One" {
		t.Errorf("expected name 'Zone One', got '%s'", zone.Name)
	}
	if zone.GetSubnet() != "subnet-primary" {
		t.Errorf("expected subnet 'subnet-primary', got '%s'", zone.GetSubnet())
	}
	if zone.GetSecondarySubnet() != "subnet-secondary" {
		t.Errorf("expected secondary_subnet 'subnet-secondary', got '%s'",
			zone.GetSecondarySubnet())
	}
}

func TestBuildImageBundles(t *testing.T) {
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
			name: "single bundle",
			input: []interface{}{
				map[string]interface{}{
					"name":           "custom-bundle",
					"use_as_default": true,
					"details": []interface{}{
						map[string]interface{}{
							"arch":            "x86_64",
							"ssh_user":        "centos",
							"ssh_port":        22,
							"global_yb_image": "projects/my-project/global/images/my-image",
						},
					},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildImageBundles(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d bundles, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestBuildImageBundlesValues(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"name":           "test-bundle",
			"use_as_default": true,
			"details": []interface{}{
				map[string]interface{}{
					"arch":            "x86_64",
					"ssh_user":        "centos",
					"ssh_port":        22,
					"global_yb_image": "projects/my-project/global/images/my-image",
				},
			},
		},
	}

	result := BuildImageBundles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(result))
	}

	bundle := result[0]
	if bundle.GetName() != "test-bundle" {
		t.Errorf("expected name 'test-bundle', got '%s'", bundle.GetName())
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
	if details.GetGlobalYbImage() != "projects/my-project/global/images/my-image" {
		t.Errorf("expected global_yb_image 'projects/my-project/global/images/my-image', got '%s'",
			details.GetGlobalYbImage())
	}
}

func TestBuildProviderDetails(t *testing.T) {
	ntpServers := []string{"0.pool.ntp.org", "1.pool.ntp.org"}
	cloudInfo := &client.CloudInfo{
		Aws: &client.AWSCloudInfo{},
	}

	result := BuildProviderDetails(true, ntpServers, true, cloudInfo)

	if !result.GetAirGapInstall() {
		t.Error("expected air_gap_install to be true")
	}
	if !result.GetSetUpChrony() {
		t.Error("expected set_up_chrony to be true")
	}
	if len(result.NtpServers) != 2 {
		t.Errorf("expected 2 ntp servers, got %d", len(result.NtpServers))
	}
	if result.NtpServers[0] != "0.pool.ntp.org" {
		t.Errorf("expected first ntp server '0.pool.ntp.org', got '%s'", result.NtpServers[0])
	}
	if result.CloudInfo == nil {
		t.Error("expected cloud_info to be set")
	}
}

func TestGetNTPServers(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{
			name:     "empty list",
			input:    []interface{}{},
			expected: []string{},
		},
		{
			name:     "single server",
			input:    []interface{}{"ntp.example.com"},
			expected: []string{"ntp.example.com"},
		},
		{
			name:     "multiple servers",
			input:    []interface{}{"ntp1.example.com", "ntp2.example.com", "ntp3.example.com"},
			expected: []string{"ntp1.example.com", "ntp2.example.com", "ntp3.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNTPServers(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d servers, got %d", len(tt.expected), len(result))
			}
			for i, server := range result {
				if server != tt.expected[i] {
					t.Errorf("expected server '%s' at index %d, got '%s'",
						tt.expected[i], i, server)
				}
			}
		})
	}
}

func TestBuildImageBundleDetailsNilDetails(t *testing.T) {
	// Test with empty details list
	result := buildImageBundleDetails([]interface{}{})
	if result != nil {
		t.Error("expected nil for empty details list")
	}
}
