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

package releases

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestDataSourceReleaseVersionDeploymentType(t *testing.T) {
	var gotDeploymentType string
	legacyHit := false
	mux := newReleaseMux(t)
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release",
		func(w http.ResponseWriter, r *http.Request) {
			gotDeploymentType = r.URL.Query().Get("deployment_type")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"release_uuid":"r1","version":"2.21.1.0-b2","yb_type":"YBDB",
					"release_type":"PREVIEW","state":"ACTIVE","artifacts":[],"universes":[]},
				{"release_uuid":"r2","version":"2.20.0.0-b1","yb_type":"YBDB",
					"release_type":"LTS","state":"DELETED","artifacts":[],"universes":[]},
				{"release_uuid":"r3","version":"2024.2.0.0-b1","yb_type":"YBDB",
					"release_type":"LTS","state":"DISABLED","artifacts":[],"universes":[]}
			]`))
		})
	mux.HandleFunc("/api/v1/customers/cust-1/releases",
		func(_ http.ResponseWriter, _ *http.Request) { legacyHit = true })

	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ReleaseVersion().Schema, map[string]interface{}{
		"deployment_type": "aarch64",
	})

	if diags := dataSourceReleaseVersionRead(context.Background(), d, c); diags.HasError() {
		t.Fatalf("read failed: %+v", diags)
	}
	if gotDeploymentType != "aarch64" {
		t.Errorf("deployment_type query param not sent, got %q", gotDeploymentType)
	}
	if legacyHit {
		t.Error("legacy /releases endpoint must not be used when deployment_type is set")
	}

	// DELETED filtered out; DISABLED kept; stable sorted before preview.
	want := []interface{}{"2024.2.0.0-b1", "2.21.1.0-b2"}
	if got := d.Get("version_list").([]interface{}); !reflect.DeepEqual(got, want) {
		t.Errorf("version_list = %v, want %v", got, want)
	}
	if got := d.Get("selected_version").(string); got != "2024.2.0.0-b1" {
		t.Errorf("selected_version = %q, want 2024.2.0.0-b1", got)
	}
}

func TestSelectVersions(t *testing.T) {
	all := []string{"2.21.1.0-b2", "2024.2.0.0-b1", "2.20.0.0-b5", "2.23.0.0-b7"}
	cases := []struct {
		name         string
		raw          map[string]interface{}
		wantList     []interface{}
		wantSelected string
	}{
		{
			name: "no filters: stable descending then preview descending",
			raw:  map[string]interface{}{},
			wantList: []interface{}{
				"2024.2.0.0-b1",
				"2.20.0.0-b5",
				"2.23.0.0-b7",
				"2.21.1.0-b2",
			},
			wantSelected: "2024.2.0.0-b1",
		},
		{
			name:         "preview track",
			raw:          map[string]interface{}{"track": "preview"},
			wantList:     []interface{}{"2.23.0.0-b7", "2.21.1.0-b2"},
			wantSelected: "2.23.0.0-b7",
		},
		{
			name:         "version prefix",
			raw:          map[string]interface{}{"version": "2.21"},
			wantList:     []interface{}{"2.21.1.0-b2"},
			wantSelected: "2.21.1.0-b2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, ReleaseVersion().Schema, tc.raw)
			if diags := selectVersions(d, all); diags.HasError() {
				t.Fatalf("selectVersions failed: %+v", diags)
			}
			if got := d.Get("version_list").([]interface{}); !reflect.DeepEqual(got, tc.wantList) {
				t.Errorf("version_list = %v, want %v", got, tc.wantList)
			}
			if got := d.Get("selected_version").(string); got != tc.wantSelected {
				t.Errorf("selected_version = %q, want %q", got, tc.wantSelected)
			}
		})
	}
}
