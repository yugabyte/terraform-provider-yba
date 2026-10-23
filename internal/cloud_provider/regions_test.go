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

package cloud_provider

import (
	"testing"

	client "github.com/yugabyte/platform-go-client"
)

func TestFlattenRegionsOCI(t *testing.T) {
	code := "us-ashburn-1"
	vnet := "ocid1.vcn.oc1.iad.aaaa"
	tmpl := "ocid1.instanceconfiguration.oc1.iad.aaaa"
	regions := []client.Region{{
		Code: &code,
		Details: &client.RegionDetails{
			CloudInfo: &client.RegionCloudInfo{
				Oci: &client.OCIRegionCloudInfo{
					Vnet:             &vnet,
					InstanceTemplate: &tmpl,
				},
			},
		},
	}}

	got := flattenRegions(regions, "oci")
	if len(got) != 1 {
		t.Fatalf("expected 1 region, got %d", len(got))
	}
	if got[0]["vnet_name"] != vnet {
		t.Errorf("vnet_name = %#v, want %q", got[0]["vnet_name"], vnet)
	}
	if got[0]["instance_template"] != tmpl {
		t.Errorf("instance_template = %#v, want %q", got[0]["instance_template"], tmpl)
	}
}
