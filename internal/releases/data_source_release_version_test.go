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

// The new list filters server-side by deployment_type but, unlike the legacy
// list, returns DELETED releases.
func TestDataSourceReleaseVersionDeploymentTypeSkipsDeleted(t *testing.T) {
	var gotDeploymentType string
	mux := newReleaseMux(t)
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release",
		func(w http.ResponseWriter, r *http.Request) {
			gotDeploymentType = r.URL.Query().Get("deployment_type")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"version":"2.21.1.0-b2","state":"ACTIVE"},
				{"version":"2024.2.1.0-b1","state":"DELETED"},
				{"version":"2024.2.0.0-b1","state":"DISABLED"}
			]`))
		})
	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ReleaseVersion().Schema, map[string]interface{}{
		"deployment_type": "aarch64",
	})

	diags := dataSourceReleaseVersionRead(context.Background(), d, c)

	if diags.HasError() {
		t.Fatalf("read failed: %+v", diags)
	}
	if gotDeploymentType != "aarch64" {
		t.Errorf("deployment_type query = %q, want aarch64", gotDeploymentType)
	}
	want := []interface{}{"2024.2.0.0-b1", "2.21.1.0-b2"}
	if got := d.Get("version_list").([]interface{}); !reflect.DeepEqual(got, want) {
		t.Errorf("version_list = %v, want %v", got, want)
	}
}
