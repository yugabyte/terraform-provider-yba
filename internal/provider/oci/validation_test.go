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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

func TestRegionsContentChangedIgnoresOrder(t *testing.T) {
	a := []interface{}{
		map[string]interface{}{
			"code": "us-ashburn-1", "vcn_id": "vcn-a",
			"instance_configuration_id": "cfg-a",
			"zones": []interface{}{
				map[string]interface{}{"code": "us-ashburn-1-AD-1", "subnet": "sub-1"},
			},
		},
	}
	b := []interface{}{
		map[string]interface{}{
			"code": "us-ashburn-1", "vcn_id": "vcn-a",
			"instance_configuration_id": "cfg-a",
			"zones": []interface{}{
				map[string]interface{}{"code": "us-ashburn-1-AD-1", "subnet": "sub-1"},
			},
		},
	}
	if regionsContentChanged(a, b) {
		t.Fatal("identical regions reported as changed")
	}
	changed := []interface{}{
		map[string]interface{}{
			"code": "us-ashburn-1", "vcn_id": "vcn-b",
			"instance_configuration_id": "cfg-a",
			"zones": []interface{}{
				map[string]interface{}{"code": "us-ashburn-1-AD-1", "subnet": "sub-1"},
			},
		},
	}
	if !regionsContentChanged(a, changed) {
		t.Fatal("VCN change was ignored")
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
	if err := ociAuthPlanError(true, false, fields); err != nil {
		t.Fatal(err)
	}
	mixed := knownAPIKey("t", "", "", "", "")
	if err := ociAuthPlanError(true, true, mixed); err == nil {
		t.Fatal("instance principal with a known tenancy_id must fail")
	}
	// A value still unknown at plan time is not an empty value.
	unknownTenancy := knownAPIKey("", "u", "f", "pem", "")
	unknownTenancy["tenancy_id"] = authField{known: false}
	if err := ociAuthPlanError(true, false, unknownTenancy); err != nil {
		t.Fatalf("unknown tenancy_id must be left for apply, got %v", err)
	}
	if err := ociAuthPlanError(false, false, knownAPIKey("", "", "", "", "")); err != nil {
		t.Fatalf("unknown use_instance_principal must be left for apply, got %v", err)
	}
}

func TestOCIProviderSchemaContract(t *testing.T) {
	r := ResourceOCIProvider()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema failed InternalValidate: %v", err)
	}
	if r.Importer == nil {
		t.Fatal("the resource must be importable")
	}
	if r.Timeouts == nil || r.Timeouts.Create == nil ||
		r.Timeouts.Update == nil || r.Timeouts.Delete == nil {
		t.Fatal("create, update, and delete timeouts are required")
	}
	if !strings.Contains(r.Description, "2026.1.2.0") {
		t.Fatal("description must name stable release 2026.1.2.0")
	}
	if strings.Contains(r.Description, "2.31.0.0") || strings.Contains(r.Description, "-b29") {
		t.Fatal("description must not name the preview line or a build number")
	}
	s := r.Schema
	if _, ok := s["customer_uuid"]; ok {
		t.Fatal("customer_uuid is a noise field and must not be exposed")
	}
	if !s["compartment_id"].Required || !s["default_region"].Required {
		t.Fatal("compartment_id and default_region must be required")
	}
	if !s["private_key_content"].Sensitive || s["private_key_content"].WriteOnly {
		t.Fatal("private_key_content is a sensitive value stored in state")
	}
	if s["private_key_file_path"].Sensitive || s["fingerprint"].Sensitive {
		t.Fatal("private_key_file_path and fingerprint must not be sensitive")
	}
	if s["use_instance_principal"].Default != false {
		t.Fatal("use_instance_principal must default to false")
	}
	if len(s["use_instance_principal"].ConflictsWith) != 0 {
		t.Fatal("use_instance_principal must not ConflictsWith API-key fields")
	}
	if got := s["private_key_content"].ConflictsWith; len(got) != 1 ||
		got[0] != "private_key_file_path" {
		t.Fatalf("private_key_content ConflictsWith = %v", got)
	}
	for _, locked := range []string{
		"use_instance_principal", "tenancy_id", "user_id", "fingerprint",
		"compartment_id", "hosted_zone_id",
	} {
		if !strings.Contains(s[locked].Description, "universe") {
			t.Errorf("%s description must state the in-use restriction", locked)
		}
	}
	var walk func(map[string]*schema.Schema, string)
	walk = func(fields map[string]*schema.Schema, prefix string) {
		for name, field := range fields {
			if field.Description == "" {
				t.Errorf("%s%s needs a Description", prefix, name)
			}
			nested, ok := field.Elem.(*schema.Resource)
			if ok {
				walk(nested.Schema, prefix+name+".")
			}
		}
	}
	walk(s, "")
	regionSchema := s["regions"].Elem.(*schema.Resource).Schema
	if _, ok := regionSchema["zones"].Elem.(*schema.Resource).Schema["secondary_subnet"]; ok {
		t.Fatal("zones must not have secondary_subnet")
	}
	details := s["image_bundles"].Elem.(*schema.Resource).Schema["details"]
	detailSchema := details.Elem.(*schema.Resource).Schema
	if _, ok := detailSchema["use_imds_v2"]; ok {
		t.Fatal("image bundle details must not have use_imds_v2")
	}
	if _, ok := detailSchema["global_yb_image"]; ok {
		t.Fatal("image bundle details must not have global_yb_image")
	}
	if _, ok := s["skip_ssh_keypair_validation"]; ok {
		t.Fatal("skip_ssh_keypair_validation must not exist")
	}
}
