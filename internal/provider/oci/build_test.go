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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestFlattenOCIRegionsDropsInactive(t *testing.T) {
	active := false
	code := "us-phoenix-1"
	zoneCode := "us-phoenix-1-AD-1"
	regions := []client.Region{{
		Code:   &code,
		Active: &active,
		Zones: []client.AvailabilityZone{{
			Code:   &zoneCode,
			Active: &active,
			Subnet: utils.GetStringPointer("ocid1.subnet.oc1.phx.aaaa"),
		}},
	}}
	if got := flattenOCIRegions(regions); len(got) != 0 {
		t.Fatalf("inactive region returned: %#v", got)
	}
}

func TestMergeRegionUUIDsDeactivatesRemoved(t *testing.T) {
	oldRegions := []interface{}{
		map[string]interface{}{
			"uuid":                      "region-1",
			"code":                      "us-ashburn-1",
			"vcn_id":                    "ocid1.vcn.oc1.iad.aaaa",
			"instance_configuration_id": "ocid1.instanceconfiguration.oc1.iad.aaaa",
			"zones": []interface{}{
				map[string]interface{}{
					"uuid": "zone-1", "code": "us-ashburn-1-AD-1",
					"subnet": "ocid1.subnet.oc1.iad.aaaa",
				},
			},
		},
		map[string]interface{}{
			"uuid":   "region-2",
			"code":   "us-phoenix-1",
			"vcn_id": "ocid1.vcn.oc1.phx.aaaa",
			"zones": []interface{}{
				map[string]interface{}{
					"uuid": "zone-2", "code": "us-phoenix-1-AD-1",
					"subnet": "ocid1.subnet.oc1.phx.aaaa",
				},
			},
		},
	}
	newRegions := []interface{}{oldRegions[0]}
	merged := mergeRegionUUIDs(oldRegions, newRegions)
	if len(merged) != 2 {
		t.Fatalf("len = %d", len(merged))
	}
	var removed client.Region
	for _, r := range merged {
		if r.GetCode() == "us-phoenix-1" {
			removed = r
		}
	}
	if !removed.HasActive() || removed.GetActive() {
		t.Fatal("removed region was not sent Active=false")
	}
	removedDetails := removed.GetDetails()
	removedCloud := removedDetails.GetCloudInfo()
	removedOCI := removedCloud.GetOci()
	if removedOCI.GetVnet() != "ocid1.vcn.oc1.phx.aaaa" {
		t.Fatal("removed region lost its VCN")
	}
	if len(removed.Zones) != 1 || removed.Zones[0].GetSubnet() != "ocid1.subnet.oc1.phx.aaaa" ||
		removed.Zones[0].GetActive() {
		t.Fatalf("removed zone = %+v", removed.Zones)
	}
}

// TestOCIProviderGuardrails pins the schema contract: these properties ship
// into customer state files.
func TestOCIProviderGuardrails(t *testing.T) {
	res := ResourceOCIProvider()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema failed InternalValidate: %v", err)
	}
	if res.Description == "" {
		t.Error("resource description is required")
	}
	if res.Importer == nil {
		t.Error("the resource must be importable")
	}
	if res.Timeouts != providerutil.DefaultTimeouts {
		t.Error("timeouts must come from providerutil.DefaultTimeouts")
	}
	if _, ok := res.Schema["customer_uuid"]; ok {
		t.Error("customer_uuid is a noise field and must not be exposed")
	}
	secrets := map[string]bool{"private_key_content": true, "ssh_private_key_content": true}
	var walk func(prefix string, s map[string]*schema.Schema)
	walk = func(prefix string, s map[string]*schema.Schema) {
		for name, f := range s {
			if f.Description == "" {
				t.Errorf("%s%s needs a Description: it renders into the docs", prefix, name)
			}
			if prefix == "" && f.Sensitive != secrets[name] {
				t.Errorf("%s: Sensitive = %t, want %t", name, f.Sensitive, secrets[name])
			}
			if elem, ok := f.Elem.(*schema.Resource); ok {
				walk(prefix+name+".", elem.Schema)
			}
		}
	}
	walk("", res.Schema)
}

func TestBuildOCICloudInfoInstancePrincipal(t *testing.T) {
	info, err := buildOCICloudInfo(ociCloudInput{
		CompartmentID: "ocid1.compartment.oc1..c", DefaultRegion: "us-ashburn-1",
		UseInstancePrincipal: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetOciCompartmentId() != "ocid1.compartment.oc1..c" ||
		info.GetOciRegion() != "us-ashburn-1" ||
		info.GetOciAuthType() != ociAuthInstancePrincipal ||
		info.OciPrivateKeyContent != nil {
		t.Fatalf("info = %+v", info)
	}
}

func TestFlattenOCIImageBundlesSkipsYBAManaged(t *testing.T) {
	name := "YBA-Managed-x86"
	bundles := []client.ImageBundle{{
		Name:     &name,
		Metadata: &client.Metadata{Type: providerutil.ImageBundleTypeYBAActive},
		Details:  &client.ImageBundleDetails{},
	}}
	if got := flattenOCIImageBundles(bundles); len(got) != 0 {
		t.Fatalf("YBA-managed bundle leaked into custom bundles: %#v", got)
	}
}

func TestEnsureOCIRegionEntries(t *testing.T) {
	bundles := buildOCIImageBundles([]interface{}{
		map[string]interface{}{
			"name": "custom-alma9", "use_as_default": true,
			"details": []interface{}{
				map[string]interface{}{
					"arch": "x86_64", "ssh_user": "opc", "ssh_port": 22,
					"region_overrides": map[string]interface{}{
						"us-ashburn-1": "ocid1.image.oc1.iad.aaaa",
					},
				},
			},
		},
	})
	filled := ensureOCIRegionEntries(bundles, []string{"us-ashburn-1", "us-phoenix-1"})
	regions := filled[0].Details.GetRegions()
	if _, ok := regions["us-phoenix-1"]; !ok {
		t.Fatal("missing empty entry for us-phoenix-1")
	}
	ashburn := regions["us-ashburn-1"]
	if ashburn.GetYbImage() != "ocid1.image.oc1.iad.aaaa" {
		t.Fatal("existing image was overwritten")
	}
}
