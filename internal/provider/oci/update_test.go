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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

const stubOCIProviderList = `[{
	"uuid":"prov","name":"oci","code":"oci",
	"details":{"cloudInfo":{"oci":{
		"ociCompartmentId":"c","ociRegion":"r","ociTenancyId":"t",
		"ociUserId":"u","ociFingerprint":"f","ociPrivateKeyContent":"o***m"}}},
	"regions":[{"uuid":"reg","code":"r","active":true,
		"details":{"cloudInfo":{"oci":{"vnet":"v"}}},
		"zones":[{"uuid":"az","code":"r-AD-1","subnet":"s","active":true}]}]
}]`

// Update reverts the fields that Read cannot restore only while YBA has not
// saved them: all four before YBA accepts the edit, and only the API key
// after a failed task wait, because CloudProviderEdit saves the access keys
// before the steps that can fail. The two API-key fields conflict, so each
// case runs once per key field.
func TestOCIProviderUpdateRevertsUnsavedFields(t *testing.T) {
	// The plan checks that a changed key file exists; the old path is only in state.
	keyFile := filepath.Join(t.TempDir(), "oci_api_key.pem")
	if err := os.WriteFile(keyFile, []byte("new-pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	keys := map[string][2]string{
		"private_key_content":   {"old-pem", "new-pem"},
		"private_key_file_path": {"/old/oci_api_key.pem", keyFile},
	}
	cases := []struct {
		name        string
		edit        int  // EditProvider status
		taskFails   bool // TaskStatus answers 500
		readFails   bool // the GetProvider list after the edit answers 500
		keyReverted bool
		sshReverted bool
	}{
		{name: "edit rejected", edit: http.StatusBadRequest, keyReverted: true, sshReverted: true},
		{name: "task wait fails", edit: http.StatusOK, taskFails: true, keyReverted: true},
		{name: "read fails after edit", edit: http.StatusOK, readFails: true},
	}
	for key, values := range keys {
		old := map[string]string{
			key: values[0], "ssh_keypair_name": "old-key", "ssh_private_key_content": "old-ssh",
		}
		planned := map[string]string{
			key: values[1], "ssh_keypair_name": "new-key", "ssh_private_key_content": "new-ssh",
		}
		for _, tc := range cases {
			t.Run(key+"/"+tc.name, func(t *testing.T) {
				edited := false
				srv := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						switch {
						case r.Method == http.MethodGet &&
							r.URL.Path == "/api/v1/customers/cust-1/providers":
							if edited && tc.readFails {
								w.WriteHeader(http.StatusInternalServerError)
								_, _ = w.Write([]byte(`{"error":"down"}`))
								return
							}
							_, _ = w.Write([]byte(stubOCIProviderList))
						case r.Method == http.MethodPut &&
							strings.HasSuffix(r.URL.Path, "/providers/prov/edit"):
							w.WriteHeader(tc.edit)
							if tc.edit != http.StatusOK {
								_, _ = w.Write([]byte(`{"error":"rejected"}`))
								return
							}
							edited = true
							if tc.taskFails {
								_, _ = w.Write(
									[]byte(`{"taskUUID":"task-1","resourceUUID":"prov"}`),
								)
								return
							}
							_, _ = w.Write([]byte(`{"resourceUUID":"prov"}`))
						default:
							w.WriteHeader(http.StatusInternalServerError)
							_, _ = w.Write([]byte(`{"error":"unexpected"}`))
						}
					}),
				)
				defer srv.Close()

				d := plannedOCIUpdate(t, old, planned)
				cfg := client.NewConfiguration()
				cfg.Host = strings.TrimPrefix(srv.URL, "http://")
				cfg.Scheme = "http"
				diags := resourceOCIProviderUpdate(context.Background(), d, &api.APIClient{
					YugawareClient: client.NewAPIClient(cfg), CustomerID: "cust-1",
				})
				if !diags.HasError() {
					t.Fatal("expected an error")
				}
				for k := range planned {
					want := planned[k]
					if (k == key && tc.keyReverted) || (k != key && tc.sshReverted) {
						want = old[k]
					}
					if got := d.Get(k).(string); got != want {
						t.Errorf("%s = %q, want %q", k, got, want)
					}
				}
			})
		}
	}
}

// plannedOCIUpdate returns ResourceData for an API-key provider whose plan
// changes the fields in planned from the values in old.
func plannedOCIUpdate(t *testing.T, old, planned map[string]string) *schema.ResourceData {
	t.Helper()
	attrs := map[string]string{
		"id": "prov", "name": "oci", "compartment_id": "c", "default_region": "r",
		"use_instance_principal": "false", "tenancy_id": "t", "user_id": "u",
		"fingerprint": "f", "regions.#": "1", "regions.0.code": "r",
		"regions.0.vcn_id": "v", "regions.0.zones.#": "1", "regions.0.zones.0.code": "r-AD-1",
		"regions.0.zones.0.subnet": "s", "yba_managed_image_bundles.#": "1",
		"yba_managed_image_bundles.0.arch": "x86_64",
	}
	for k, v := range old {
		attrs[k] = v
	}
	raw := map[string]interface{}{
		"name": "oci", "compartment_id": "c", "default_region": "r",
		"use_instance_principal": false, "tenancy_id": "t", "user_id": "u",
		"fingerprint": "f",
		"regions": []interface{}{map[string]interface{}{
			"code": "r", "vcn_id": "v",
			"zones": []interface{}{map[string]interface{}{"code": "r-AD-1", "subnet": "s"}},
		}},
		"yba_managed_image_bundles": []interface{}{map[string]interface{}{"arch": "x86_64"}},
	}
	for k, v := range planned {
		raw[k] = v
	}
	res := ResourceOCIProvider()
	state := &terraform.InstanceState{ID: "prov", Attributes: attrs}
	diff, err := res.Diff(context.Background(), state, terraform.NewResourceConfigRaw(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err := schema.InternalMap(res.Schema).Data(state, diff)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
