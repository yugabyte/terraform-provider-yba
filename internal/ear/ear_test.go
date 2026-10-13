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
	taskFails      bool // every task ends in Failure
	editRejected   bool // the edit answers 400, as for an edit YBA rejects before it saves
	// savedConfig answers the by-UUID GET: the stored settings with
	// credentials in clear. "" answers 500.
	savedConfig string
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
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/edit"):
			if f.editRejected {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"success": false, "error": "Invalid AWS Credentials"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(task)
		case r.Method == http.MethodGet && strings.Contains(path, "/kms_configs/"):
			if f.savedConfig == "" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(f.savedConfig))
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/app_version"):
			_, _ = w.Write([]byte(`{"version": "2024.2.0.0-b145"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/failed_subtasks"):
			_, _ = w.Write([]byte(`{"failedSubTasks": []}`))
		case r.Method == http.MethodGet && strings.Contains(path, "/tasks/"):
			status := "Success"
			if f.taskFails {
				status = "Failure"
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"title":   "CreateKMSConfig",
				"percent": 100.0,
				"status":  status,
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

	diags := earDelete(gcpEARSpec())(context.Background(), d, f.apiClient(t))
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

	if diags := earDelete(gcpEARSpec())(context.Background(), d, f.apiClient(t)); diags.HasError() {
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

// --- update -----------------------------------------------------------------

// YBA rejects some edits before it saves them. It saves others before its KMS
// check, which then fails the task but keeps the edit. After a failed edit,
// the credential fields in state hold what YBA saved, so the next plan shows
// what an apply changes.
func TestEARUpdateFailureKeepsWhatYBASaved(t *testing.T) {
	azureSaved := func(clientID, secret string) string {
		return fmt.Sprintf(`{"CLIENT_ID": %q, "TENANT_ID": "tenant", "CLIENT_SECRET": %q,
			"AZU_VAULT_URL": "https://v.vault.azure.net/", "AZU_KEY_NAME": "yb-key",
			"AZU_KEY_ALGORITHM": "RSA", "AZU_KEY_SIZE": 2048}`, clientID, secret)
	}
	awsKeys := `{"AWS_REGION": "us-west-2", "cmk_id": "key", "AWS_ACCESS_KEY_ID": "AKIA",
		"AWS_SECRET_ACCESS_KEY": "old", "AWS_KMS_ENDPOINT": "kms-a.example.com"}`

	cases := []struct {
		name       string
		spec       earSpec
		state      map[string]string
		raw        map[string]interface{}
		rejected   bool   // YBA answers the edit with 400; else the task fails
		saved      string // the settings YBA holds after the failed edit
		unreadable bool   // the by-UUID GET fails
		want       map[string]string
	}{
		{
			// The KMS check fails after the save. A return to the service
			// principal must then send its secret again.
			name: "azure switch to the managed identity saved", spec: azureEARSpec(),
			state: azureState, raw: azureConfig("mi", ""), saved: azureSaved("mi", ""),
			want: map[string]string{"client_secret": ""},
		},
		{
			// EditKMSConfig checks a universe's active key before the save.
			name: "azure rotation not saved", spec: azureEARSpec(),
			state: azureState, raw: azureConfig("app", "new"), saved: azureSaved("app", "old"),
			want: map[string]string{"client_secret": "old"},
		},
		{
			// isValidCredsKms rejects the host identity. The next plan must
			// try the removal again.
			name: "aws key removal rejected", spec: awsEARSpec(),
			state: awsState, raw: awsConfig(false, ""), rejected: true, saved: awsKeys,
			want: map[string]string{"access_key_id": "AKIA", "secret_access_key": "old"},
		},
		{
			name: "saved settings unreadable", spec: azureEARSpec(),
			state: azureState, raw: azureConfig("app", "new"), saved: azureSaved("app", "old"),
			unreadable: true,
			want:       map[string]string{"client_secret": "old"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeYBA{
				listBody: listOf(listEntry(testConfigUUID, "cfg", tc.spec.apiProvider, false,
					tc.saved)),
				editRejected: tc.rejected,
				taskFails:    !tc.rejected,
				savedConfig:  tc.saved,
			}
			if tc.unreadable {
				f.savedConfig = ""
			}
			d := plannedData(t, earResource(tc.spec), tc.state, tc.raw)
			update := earUpdate(tc.spec)
			if diags := update(context.Background(), d, f.apiClient(t)); !diags.HasError() {
				t.Fatal("want the failed edit as an error")
			}
			for field, want := range tc.want {
				if got := d.Get(field); got != want {
					t.Errorf("%s = %q want %q", field, got, want)
				}
			}
		})
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
