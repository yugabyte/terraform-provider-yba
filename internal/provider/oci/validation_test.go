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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestCheckOCIProviderVersion(t *testing.T) {
	ctx := context.Background()
	if err := checkOCIProviderVersion(ctx, "2026.1.2.0-b29"); err != nil {
		t.Fatal(err)
	}
	if err := checkOCIProviderVersion(ctx, "2.31.0.0-b133"); err != nil {
		t.Fatal(err)
	}
	if err := checkOCIProviderVersion(ctx, "2026.1.2.0-b28"); err == nil {
		t.Fatal("a stable build below the minimum must fail")
	}
	if err := checkOCIProviderVersion(ctx, "2.31.0.0-b132"); err == nil {
		t.Fatal("a preview build below the minimum must fail")
	}
	if err := checkOCIProviderVersion(ctx, "not-a-version"); err != nil {
		t.Fatalf("unreadable version must pass, got %v", err)
	}
}

func TestCheckImageBundleRegionCoverageOCI(t *testing.T) {
	bundle := func(overrides map[string]interface{}) []interface{} {
		return []interface{}{map[string]interface{}{
			"name": "custom-alma9",
			"details": []interface{}{
				map[string]interface{}{"region_overrides": overrides},
			},
		}}
	}
	allKnown := func(string) bool { return true }
	regions := []string{"us-ashburn-1", "us-phoenix-1"}
	if err := checkImageBundleRegionCoverage(bundle(map[string]interface{}{
		"us-ashburn-1": "ocid1.image.oc1.iad.aaaa",
	}), regions, allKnown); err == nil {
		t.Fatal("a missing region image must fail")
	}
	partial := bundle(map[string]interface{}{
		"us-ashburn-1": "ocid1.image.oc1.iad.aaaa",
		"us-phoenix-1": "",
	})
	if err := checkImageBundleRegionCoverage(partial, regions, allKnown); err == nil {
		t.Fatal("an empty region image must fail")
	}
	if err := checkImageBundleRegionCoverage(bundle(map[string]interface{}{
		"us-ashburn-1": "ocid1.image.oc1.iad.aaaa",
		"us-phoenix-1": "ocid1.image.oc1.phx.aaaa",
	}), regions, allKnown); err != nil {
		t.Fatal(err)
	}
}

// An image OCID from another resource reads as "" until it is known; the plan
// leaves it to YBA.
func TestOCIPlanAcceptsUnknownImage(t *testing.T) {
	c := terraform.NewResourceConfigRaw(map[string]interface{}{
		"name": "oci", "compartment_id": "c", "default_region": "r",
		"use_instance_principal": true,
		"regions": []interface{}{map[string]interface{}{
			"code": "r", "vcn_id": "v",
			"zones": []interface{}{map[string]interface{}{"code": "r-AD-1", "subnet": "s"}},
		}},
		"image_bundles": []interface{}{map[string]interface{}{
			"name": "b", "use_as_default": true,
			"details": []interface{}{map[string]interface{}{
				"arch": "x86_64", "ssh_user": "opc",
				"region_overrides": map[string]interface{}{
					"r": "74D93920-ED26-11E3-AC10-0800200C9A66",
				},
			}},
		}},
	})
	if _, err := ResourceOCIProvider().Diff(context.Background(), nil, c, nil); err != nil {
		t.Fatalf("unknown image OCID: %v", err)
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
	if err := checkDuplicateRegionsAndZones(regions); err == nil {
		t.Fatal("a duplicate zone must fail")
	}
	dupRegions := []interface{}{
		map[string]interface{}{"code": "us-ashburn-1", "zones": []interface{}{}},
		map[string]interface{}{"code": "us-ashburn-1", "zones": []interface{}{}},
	}
	if err := checkDuplicateRegionsAndZones(dupRegions); err == nil {
		t.Fatal("a duplicate region must fail")
	}
	arches := []interface{}{
		map[string]interface{}{"arch": "x86_64"},
		map[string]interface{}{"arch": "x86_64"},
	}
	if err := checkDuplicateManagedArch(arches); err == nil {
		t.Fatal("a duplicate architecture must fail")
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
	if err := ociAuthPlanError(true, false, imported, true); err == nil {
		t.Fatal("a missing key must fail")
	}
}
