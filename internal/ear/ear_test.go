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

package ear

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

const (
	testConfigUUID = "1f0a3d0e-8f9b-4a0c-9c1d-2e3f4a5b6c7d"
	testTaskUUID   = "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"
)

// fakeYBA is a minimal kms_configs API stub. Fields that were not hit stay
// zero-valued so tests can assert which endpoints were exercised.
type fakeYBA struct {
	listBody       string
	createBody     map[string]interface{}
	createResponse map[string]interface{} // nil: {"taskUUID": testTaskUUID}
	deleteCalled   bool
}

func (f *fakeYBA) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		task := map[string]interface{}{"taskUUID": testTaskUUID}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/kms_configs"):
			_, _ = w.Write([]byte(f.listBody))
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/kms_configs/GCP"):
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &f.createBody); err != nil {
				t.Errorf("create body is not a JSON object: %s", body)
			}
			if f.createResponse != nil {
				task = f.createResponse
			}
			_ = json.NewEncoder(w).Encode(task)
		case r.Method == http.MethodDelete && strings.Contains(path, "/kms_configs/"):
			f.deleteCalled = true
			_ = json.NewEncoder(w).Encode(task)
		case r.Method == http.MethodGet && strings.Contains(path, "/tasks/"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"title":   "CreateKMSConfig",
				"percent": 100.0,
				"status":  "Success",
				"details": map[string]interface{}{"taskDetails": []interface{}{}},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (f *fakeYBA) apiClient(t *testing.T) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")

	cfg := client.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = addr
	return &api.APIClient{
		VanillaClient: &api.VanillaClient{
			Client: srv.Client(), Host: addr, EnableHTTPS: false,
		},
		YugawareClient: client.NewAPIClient(cfg),
		CustomerID:     "cust",
		APIKey:         "tok",
	}
}

const gcpKeyFileSettings = `{
	"GCP_CONFIG": {"type": "service_account", "project_id": "sa-project",
	               "private_key": "-----BEGIN P*****Y-----"},
	"LOCATION_ID": "global", "KEY_RING_ID": "yb-kr", "CRYPTO_KEY_ID": "yb-ck",
	"PROTECTION_LEVEL": "HSM"
}`

func listEntry(uuid, name, provider string, inUse bool, settings string) string {
	universes := "[]"
	if inUse {
		universes = `[{"uuid": "u-1", "name": "prod"}, {"uuid": "u-2", "name": "staging"}]`
	}
	return fmt.Sprintf(`{"credentials": %s, "metadata": {"configUUID": %q, "name": %q,
		"provider": %q, "in_use": %t, "universeDetails": %s}}`,
		settings, uuid, name, provider, inUse, universes)
}

func listOf(entries ...string) string {
	return "[" + strings.Join(entries, ",") + "]"
}

// --- create -----------------------------------------------------------------

func TestCreateEARConfigUsesResourceUUID(t *testing.T) {
	f := &fakeYBA{
		listBody: "[]",
		createResponse: map[string]interface{}{
			"taskUUID": testTaskUUID, "resourceUUID": testConfigUUID,
		},
	}
	c := f.apiClient(t)
	uuid, err := createEARConfig(context.Background(), c.YugawareClient, c.CustomerID,
		providerGCP, "ring", map[string]interface{}{gcpKeyLocationID: "global"}, time.Minute)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if uuid != testConfigUUID {
		t.Errorf("uuid = %q want %q (from task resourceUUID)", uuid, testConfigUUID)
	}
	if f.createBody["name"] != "ring" || f.createBody[gcpKeyLocationID] != "global" {
		t.Errorf("create body = %v: want name and settings side by side", f.createBody)
	}
}

func TestCreateEARConfigFallsBackToNameLookup(t *testing.T) {
	// Older YBA: the task response carries no resourceUUID, so the new config
	// is found by name.
	f := &fakeYBA{listBody: listOf(
		listEntry("other", "other", providerGCP, false, gcpKeyFileSettings),
		listEntry(testConfigUUID, "ring", providerGCP, false, gcpKeyFileSettings),
	)}
	c := f.apiClient(t)
	uuid, err := createEARConfig(context.Background(), c.YugawareClient, c.CustomerID,
		providerGCP, "ring", map[string]interface{}{}, time.Minute)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if uuid != testConfigUUID {
		t.Errorf("uuid = %q want %q (from list by name)", uuid, testConfigUUID)
	}
}

// --- delete -----------------------------------------------------------------

func TestDeleteEARConfigInUseFailsBeforeAPICall(t *testing.T) {
	f := &fakeYBA{listBody: listOf(
		listEntry(testConfigUUID, "ring", providerGCP, true, gcpKeyFileSettings))}
	d := ResourceGCPEARConfig().TestResourceData()
	d.SetId(testConfigUUID)

	diags := resourceEARConfigDelete(context.Background(), d, f.apiClient(t))
	if !diags.HasError() {
		t.Fatal("expected an error for a config with key history")
	}
	msg := diags[0].Summary
	for _, want := range []string{`"ring"`, "prod (u-1)", "staging (u-2)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %s", msg, want)
		}
	}
	if f.deleteCalled {
		t.Error("DELETE must not be sent for a config YBA would refuse to delete")
	}
	if d.Id() != testConfigUUID {
		t.Error("a failed delete must keep the resource in state")
	}
}

func TestDeleteEARConfigAlreadyGone(t *testing.T) {
	f := &fakeYBA{listBody: "[]"}
	d := ResourceGCPEARConfig().TestResourceData()
	d.SetId(testConfigUUID)

	if diags := resourceEARConfigDelete(context.Background(), d, f.apiClient(t)); diags.HasError() {
		t.Fatalf("delete of a missing config must succeed, got %v", diags)
	}
	if f.deleteCalled {
		t.Error("DELETE must not be sent for a config that is already gone")
	}
	if d.Id() != "" {
		t.Error("ID must be cleared")
	}
}

// --- read -------------------------------------------------------------------

func TestReadRejectsProviderMismatch(t *testing.T) {
	f := &fakeYBA{listBody: listOf(
		listEntry(testConfigUUID, "aws-ring", "AWS", false, `{"AWS_REGION": "us-west-2"}`))}
	d := ResourceGCPEARConfig().TestResourceData()
	d.SetId(testConfigUUID)

	diags := earRead(gcpEARSpec())(context.Background(), d, f.apiClient(t))
	if !diags.HasError() || !strings.Contains(diags[0].Summary, "uses key provider AWS") {
		t.Fatalf("want a provider mismatch error, got %v", diags)
	}
}

func TestReadFlattensGCPKeyFileConfig(t *testing.T) {
	f := &fakeYBA{listBody: listOf(
		listEntry(testConfigUUID, "ring", providerGCP, true, gcpKeyFileSettings))}
	d := ResourceGCPEARConfig().TestResourceData()
	d.SetId(testConfigUUID)
	_ = d.Set("credentials", `{"type":"service_account"}`)

	if diags := earRead(gcpEARSpec())(context.Background(), d, f.apiClient(t)); diags.HasError() {
		t.Fatalf("read: %v", diags)
	}
	want := map[string]interface{}{
		"name": "ring", "uuid": testConfigUUID, "in_use": true,
		"location_id": "global", "key_ring_id": "yb-kr", "crypto_key_id": "yb-ck",
		"protection_level": "HSM", "use_gcp_iam": false,
		// Never read back: the masked key must not replace the configured one.
		"credentials": `{"type":"service_account"}`,
	}
	for k, v := range want {
		if got := d.Get(k); got != v {
			t.Errorf("%s = %v want %v", k, got, v)
		}
	}
}

func TestReadGoneClearsID(t *testing.T) {
	f := &fakeYBA{listBody: "[]"}
	d := ResourceGCPEARConfig().TestResourceData()
	d.SetId(testConfigUUID)

	if diags := earRead(gcpEARSpec())(context.Background(), d, f.apiClient(t)); diags.HasError() {
		t.Fatalf("read: %v", diags)
	}
	if d.Id() != "" {
		t.Error("a config YBA no longer lists must be removed from state")
	}
}

// --- data source ------------------------------------------------------------

func TestDataSourceEARConfigByName(t *testing.T) {
	f := &fakeYBA{listBody: listOf(
		listEntry("aws-uuid", "aws-ring", "AWS", false, `{"AWS_REGION": "us-west-2"}`),
		listEntry(testConfigUUID, "ring", providerGCP, true, gcpKeyFileSettings))}
	d := DataSourceEARConfig().TestResourceData()
	_ = d.Set("name", "ring")

	if diags := dataSourceEARConfigRead(context.Background(), d, f.apiClient(t)); diags.HasError() {
		t.Fatalf("read: %v", diags)
	}
	if d.Id() != testConfigUUID || d.Get("uuid") != testConfigUUID {
		t.Errorf("id/uuid = %q/%v want %q", d.Id(), d.Get("uuid"), testConfigUUID)
	}
	if d.Get("key_provider") != providerGCP || d.Get("in_use") != true {
		t.Errorf("key_provider/in_use = %v/%v", d.Get("key_provider"), d.Get("in_use"))
	}
	universes := d.Get("universes").([]interface{})
	if len(universes) != 2 || universes[0].(map[string]interface{})["name"] != "prod" {
		t.Errorf("universes = %v want prod and staging", universes)
	}
}
