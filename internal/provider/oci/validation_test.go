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

package oci

import (
	"context"
	"strings"
	"testing"
)

func TestCheckOCIProviderVersion(t *testing.T) {
	ctx := context.Background()
	if err := checkOCIProviderVersion(ctx, "2026.1.2.0-b29"); err != nil {
		t.Fatal(err)
	}
	if err := checkOCIProviderVersion(ctx, "2.31.0.0-b133"); err != nil {
		t.Fatal(err)
	}
	err := checkOCIProviderVersion(ctx, "2026.1.2.0-b28")
	if err == nil || !strings.Contains(err.Error(), "yba_oci_provider") {
		t.Fatalf("stable err = %v", err)
	}
	err = checkOCIProviderVersion(ctx, "2.31.0.0-b132")
	if err == nil || !strings.Contains(err.Error(), "yba_oci_provider") {
		t.Fatalf("preview err = %v", err)
	}
	if err := checkOCIProviderVersion(ctx, "not-a-version"); err != nil {
		t.Fatalf("unreadable version must pass, got %v", err)
	}
}

func TestCheckImageBundleRegionCoverageOCI(t *testing.T) {
	bundle := func(overrides map[string]interface{}) interface{} {
		return map[string]interface{}{
			"name": "custom-alma9",
			"details": []interface{}{
				map[string]interface{}{"region_overrides": overrides},
			},
		}
	}
	regions := []string{"us-ashburn-1", "us-phoenix-1"}
	err := checkImageBundleRegionCoverage([]interface{}{
		bundle(map[string]interface{}{"us-ashburn-1": "ocid1.image.oc1.iad.aaaa"}),
	}, regions)
	if err == nil || !strings.Contains(err.Error(), "us-phoenix-1") {
		t.Fatalf("missing image err = %v", err)
	}
	err = checkImageBundleRegionCoverage([]interface{}{
		bundle(map[string]interface{}{
			"us-ashburn-1": "ocid1.image.oc1.iad.aaaa",
			"us-phoenix-1": "",
		}),
	}, regions)
	if err == nil || !strings.Contains(err.Error(), `non-empty image`) {
		t.Fatalf("empty image err = %v", err)
	}
	err = checkImageBundleRegionCoverage([]interface{}{
		bundle(map[string]interface{}{
			"us-ashburn-1": "ocid1.image.oc1.iad.aaaa",
			"us-phoenix-1": "ocid1.image.oc1.phx.aaaa",
		}),
	}, regions)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckDuplicateRegionsZonesAndArch(t *testing.T) {
	regions := []interface{}{
		map[string]interface{}{
			"code": "us-ashburn-1",
			"zones": []interface{}{
				map[string]interface{}{"code": "us-ashburn-1-AD-1"},
				map[string]interface{}{"code": "us-ashburn-1-AD-1"},
			},
		},
	}
	if err := checkDuplicateRegionsAndZones(regions); err == nil ||
		!strings.Contains(err.Error(), "us-ashburn-1-AD-1") {
		t.Fatalf("zone err = %v", err)
	}
	dupRegions := []interface{}{
		map[string]interface{}{"code": "us-ashburn-1", "zones": []interface{}{}},
		map[string]interface{}{"code": "us-ashburn-1", "zones": []interface{}{}},
	}
	if err := checkDuplicateRegionsAndZones(dupRegions); err == nil ||
		!strings.Contains(err.Error(), "us-ashburn-1") {
		t.Fatalf("region err = %v", err)
	}
	arches := []interface{}{
		map[string]interface{}{"arch": "x86_64"},
		map[string]interface{}{"arch": "x86_64"},
	}
	if err := checkDuplicateManagedArch(arches); err == nil ||
		!strings.Contains(err.Error(), "x86_64") {
		t.Fatalf("arch err = %v", err)
	}
	unknown := []interface{}{
		map[string]interface{}{
			"code": "",
			"zones": []interface{}{
				map[string]interface{}{"code": ""},
			},
		},
		map[string]interface{}{
			"code": "",
			"zones": []interface{}{
				map[string]interface{}{"code": ""},
			},
		},
	}
	if err := checkDuplicateRegionsAndZones(unknown); err != nil {
		t.Fatalf("unknown codes: %v", err)
	}
}

func knownAPIKey(tenancy, user, fingerprint, content, path string) map[string]authField {
	return map[string]authField{
		"tenancy_id":            {known: true, value: tenancy},
		"user_id":               {known: true, value: user},
		"fingerprint":           {known: true, value: fingerprint},
		"private_key_content":   {known: true, value: content},
		"private_key_file_path": {known: true, value: path},
	}
}

func TestOCIAuthPlanError(t *testing.T) {
	// An explicit false next to the API-key fields is API-key auth.
	fields := knownAPIKey("t", "u", "f", "", "/tmp/k.pem")
	if err := ociAuthPlanError(true, false, fields, true); err != nil {
		t.Fatal(err)
	}
	mixed := knownAPIKey("t", "", "", "", "")
	if err := ociAuthPlanError(true, true, mixed, true); err == nil {
		t.Fatal("instance principal with a known tenancy_id must fail")
	}
	// A value still unknown at plan time is not an empty value.
	unknownTenancy := knownAPIKey("", "u", "f", "pem", "")
	unknownTenancy["tenancy_id"] = authField{known: false}
	if err := ociAuthPlanError(true, false, unknownTenancy, true); err != nil {
		t.Fatalf("unknown tenancy_id must be left for apply, got %v", err)
	}
	if err := ociAuthPlanError(false, false, knownAPIKey("", "", "", "", ""), true); err != nil {
		t.Fatalf("unknown use_instance_principal must be left for apply, got %v", err)
	}
	// ignore_changes keeps the empty imported key. A new config, or a plan
	// that clears the key, still requires exactly one.
	imported := knownAPIKey("t", "u", "f", "", "")
	if err := ociAuthPlanError(true, false, imported, false); err != nil {
		t.Fatalf("unchanged imported key must plan, got %v", err)
	}
	if err := ociAuthPlanError(true, false, imported, true); err == nil ||
		!strings.Contains(err.Error(), "set exactly one") {
		t.Fatalf("missing key err = %v", err)
	}
}
