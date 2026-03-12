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
