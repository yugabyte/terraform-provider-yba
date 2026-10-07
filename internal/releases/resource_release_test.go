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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func newTestAPIClient(t *testing.T, handler http.Handler) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := client.NewConfiguration()
	cfg.Host = host
	cfg.Scheme = "http"
	c := &api.APIClient{
		YugawareClient: client.NewAPIClient(cfg),
		VanillaClient:  &api.VanillaClient{Client: srv.Client(), Host: host},
		APIKey:         "test-token",
		CustomerID:     "cust-1",
	}
	c.SetAppVersion("2024.2.0.0-b5")
	return c
}

func newReleaseMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/customers/cust-1/runtime_config/"+
		utils.GlobalRuntimeConfigScope+"/key/"+releasesRedesignKey,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("true"))
		})
	return mux
}

func handleJSON(mux *http.ServeMux, pattern string, status int, body string) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func localFileConfig(t *testing.T) map[string]interface{} {
	t.Helper()
	path := filepath.Join(t.TempDir(), "yugabyte-2.21.0.0-b1-linux-x86_64.tar.gz")
	if err := os.WriteFile(path, []byte("test-tarball-content"), 0600); err != nil {
		t.Fatalf("write test tarball: %v", err)
	}
	return map[string]interface{}{
		"version": "2.21.0.0-b1",
		"artifact": []interface{}{map[string]interface{}{
			"platform": "LINUX", "architecture": "x86_64", "local_file": path,
		}},
	}
}

func TestResourceReleaseGuardrails(t *testing.T) {
	res := ResourceRelease()
	if res.Importer == nil {
		t.Error("yba_ybdb_release must be importable")
	}
	if res.Description == "" {
		t.Error("resource description is required")
	}
	if _, ok := res.Schema["customer_uuid"]; ok {
		t.Error("customer_uuid is a noise field and must not be exposed")
	}
	for op, timeout := range map[string]*time.Duration{
		"create": res.Timeouts.Create,
		"update": res.Timeouts.Update,
		"delete": res.Timeouts.Delete,
	} {
		if timeout == nil || *timeout != releaseOperationTimeout {
			t.Errorf("%s timeout must default to releaseOperationTimeout", op)
		}
	}
	// The update API cannot change either field.
	for _, name := range []string{"version", "release_type"} {
		if !res.Schema[name].ForceNew {
			t.Errorf("%s must be ForceNew", name)
		}
	}
	for name, s := range res.Schema {
		if s.Description == "" {
			t.Errorf("field %s is missing a Description", name)
		}
	}
	for name, s := range res.Schema["artifact"].Elem.(*schema.Resource).Schema {
		if s.Description == "" {
			t.Errorf("artifact field %s is missing a Description", name)
		}
	}
}

func TestResourceReleaseCreateInfersTypeAndDateFromTarball(t *testing.T) {
	var createBody map[string]interface{}
	mux := newReleaseMux(t)
	handleJSON(mux, "POST /api/v1/customers/cust-1/ybdb_release/upload", http.StatusOK,
		`{"resourceUUID":"file-1"}`)
	handleJSON(mux, "GET /api/v1/customers/cust-1/ybdb_release/upload/file-1", http.StatusOK,
		`{"version":"2.21.0.0-b1","sha256":"sha-1","platform":"LINUX",
			"architecture":"x86_64","release_type":"PREVIEW",
			"release_date_msecs":1722128523000,"status":"success"}`)
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resourceUUID":"rel-1"}`))
		})
	handleJSON(mux, "GET /api/v1/customers/cust-1/ybdb_release/rel-1", http.StatusOK,
		`{"version":"2.21.0.0-b1","release_type":"PREVIEW","state":"ACTIVE",
			"artifacts":[{"platform":"LINUX","architecture":"x86_64",
				"package_file_id":"file-1"}]}`)
	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, localFileConfig(t))

	diags := resourceReleaseCreate(context.Background(), d, c)

	if diags.HasError() {
		t.Fatalf("create failed: %+v", diags)
	}
	if createBody["release_type"] != "PREVIEW" {
		t.Errorf("release_type = %v, want PREVIEW from metadata", createBody["release_type"])
	}
	if createBody["release_date_msecs"] != float64(1722128523000) {
		t.Errorf("release_date_msecs = %v, want metadata value",
			createBody["release_date_msecs"])
	}
	created := createBody["artifacts"].([]interface{})[0].(map[string]interface{})
	if created["package_file_id"] != "file-1" || created["sha256"] != "sha-1" {
		t.Errorf("create artifact = %+v, want uploaded file-1/sha-1", created)
	}
	// GET omits sha256: only Create can record it.
	if got := d.Get("artifact.0.sha256").(string); got != "sha-1" {
		t.Errorf("state sha256 = %q, want sha-1", got)
	}
}

func TestResourceReleaseCreateRejectsTarballVersionMismatch(t *testing.T) {
	registered := false
	mux := newReleaseMux(t)
	handleJSON(mux, "POST /api/v1/customers/cust-1/ybdb_release/upload", http.StatusOK,
		`{"resourceUUID":"file-1"}`)
	handleJSON(mux, "GET /api/v1/customers/cust-1/ybdb_release/upload/file-1", http.StatusOK,
		`{"version":"2.22.0.0-b7","platform":"LINUX","architecture":"x86_64",
			"status":"success"}`)
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release",
		func(_ http.ResponseWriter, _ *http.Request) { registered = true })
	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, localFileConfig(t))

	diags := resourceReleaseCreate(context.Background(), d, c)

	if !diags.HasError() || !strings.Contains(diags[0].Summary, "does not match release version") {
		t.Fatalf("want a version mismatch error, got %+v", diags)
	}
	if registered || d.Id() != "" {
		t.Errorf("release registered after a mismatch (id %q)", d.Id())
	}
}

// YBA answers a missing release with 400; a 404 means an older YBA lacks the
// route and must not drop a live release from state.
func TestResourceReleaseReadGone(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantGone bool
	}{
		{
			name:     "400 invalid release uuid",
			status:   http.StatusBadRequest,
			body:     `{"success":false,"error":"Invalid Release UUID: rel-1"}`,
			wantGone: true,
		},
		{name: "404 unknown route", status: http.StatusNotFound, body: `Not Found`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := newReleaseMux(t)
			handleJSON(mux, "GET /api/v1/customers/cust-1/ybdb_release/rel-1", tc.status, tc.body)
			c := newTestAPIClient(t, mux)
			d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{})
			d.SetId("rel-1")

			diags := resourceReleaseRead(context.Background(), d, c)

			if gone := d.Id() == "" && !diags.HasError(); gone != tc.wantGone {
				t.Errorf("gone = %v, want %v (id %q, diags %+v)", gone, tc.wantGone, d.Id(), diags)
			}
		})
	}
}

// GET omits sha256 and local_file, and hides package_url once YBA caches a
// downloaded Kubernetes chart.
func TestResourceReleaseReadKeepsWhatGetOmits(t *testing.T) {
	mux := newReleaseMux(t)
	handleJSON(mux, "GET /api/v1/customers/cust-1/ybdb_release/rel-1", http.StatusOK,
		`{"version":"2.21.0.0-b1","release_type":"PREVIEW","state":"ACTIVE",
			"artifacts":[
				{"platform":"LINUX","architecture":"x86_64","package_file_id":"file-new"},
				{"platform":"KUBERNETES","package_file_id":"chart-1"},
				{"platform":"LINUX","architecture":"aarch64","package_file_id":"file-arm"}
			]}`)
	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{
		"version": "2.21.0.0-b1",
		"artifact": []interface{}{
			map[string]interface{}{
				"platform":        "LINUX",
				"architecture":    "x86_64",
				"local_file":      "/tmp/yugabyte-x86_64.tar.gz",
				"package_file_id": "file-old",
				"sha256":          "sha-keep",
			},
			map[string]interface{}{
				"platform":    "KUBERNETES",
				"package_url": "https://example.com/helm.tgz",
			},
		},
	})
	d.SetId("rel-1")

	diags := resourceReleaseRead(context.Background(), d, c)

	if diags.HasError() {
		t.Fatalf("read failed: %+v", diags)
	}
	want := []interface{}{
		map[string]interface{}{
			"platform": "LINUX", "architecture": "x86_64",
			"local_file": "/tmp/yugabyte-x86_64.tar.gz", "package_url": "",
			"package_file_id": "file-new", "sha256": "sha-keep",
		},
		map[string]interface{}{
			"platform": "KUBERNETES", "architecture": "",
			"local_file": "", "package_url": "https://example.com/helm.tgz",
			"package_file_id": "chart-1", "sha256": "",
		},
		// Added out-of-band: appended with no local identifiers.
		map[string]interface{}{
			"platform": "LINUX", "architecture": "aarch64",
			"local_file": "", "package_url": "",
			"package_file_id": "file-arm", "sha256": "",
		},
	}
	if got := d.Get("artifact"); !reflect.DeepEqual(got, want) {
		t.Errorf("artifact = %+v\nwant %+v", got, want)
	}
}

// A file-to-URL switch is delete-then-recreate: the first PUT drops the
// artifact, the second adds it back with only the URL.
func TestResourceReleaseUpdateSourceSwitchDeletesThenRecreates(t *testing.T) {
	var puts []map[string]interface{}
	mux := newReleaseMux(t)
	mux.HandleFunc("PUT /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			puts = append(puts, body)
			_, _ = w.Write([]byte(`{"success":true}`))
		})
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release/upload",
		func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("unchanged local_file must not be re-uploaded")
		})
	handleJSON(mux, "GET /api/v1/customers/cust-1/ybdb_release/rel-1", http.StatusOK,
		`{"version":"2.21.0.0-b1","release_type":"PREVIEW","state":"ACTIVE","universes":[],
			"artifacts":[
				{"platform":"LINUX","architecture":"x86_64","package_file_id":"file-1"},
				{"platform":"LINUX","architecture":"aarch64",
					"package_url":"https://example.com/arm.tar.gz"}
			]}`)
	c := newTestAPIClient(t, mux)
	d := ResourceRelease().Data(&terraform.InstanceState{
		ID: "rel-1",
		Attributes: map[string]string{
			"version":                    "2.21.0.0-b1",
			"release_date_msecs":         "1722128523123",
			"state":                      "ACTIVE",
			"artifact.#":                 "2",
			"artifact.0.platform":        "LINUX",
			"artifact.0.architecture":    "x86_64",
			"artifact.0.local_file":      "/tmp/x86.tar.gz",
			"artifact.0.package_file_id": "file-1",
			"artifact.0.sha256":          "sha-1",
			"artifact.1.platform":        "LINUX",
			"artifact.1.architecture":    "aarch64",
			"artifact.1.local_file":      "/tmp/arm.tar.gz",
			"artifact.1.package_file_id": "file-2",
			"artifact.1.sha256":          "sha-2",
		},
	})
	// The plan: x86_64 unchanged, aarch64 switched to a URL.
	if err := d.Set("artifact", []interface{}{
		map[string]interface{}{
			"platform": "LINUX", "architecture": "x86_64", "local_file": "/tmp/x86.tar.gz",
		},
		map[string]interface{}{
			"platform": "LINUX", "architecture": "aarch64",
			"package_url": "https://example.com/arm.tar.gz",
		},
	}); err != nil {
		t.Fatal(err)
	}

	diags := resourceReleaseUpdate(context.Background(), d, c)

	if diags.HasError() {
		t.Fatalf("update failed: %+v", diags)
	}
	if len(puts) != 2 {
		t.Fatalf("got %d PUTs, want 2", len(puts))
	}
	x86 := map[string]interface{}{
		"platform": "LINUX", "architecture": "x86_64",
		"package_file_id": "file-1", "sha256": "sha-1",
	}
	arm := map[string]interface{}{
		"platform": "LINUX", "architecture": "aarch64",
		"package_url": "https://example.com/arm.tar.gz",
	}
	if got, want := puts[0]["artifacts"], []interface{}{x86}; !reflect.DeepEqual(got, want) {
		t.Errorf("first PUT artifacts = %+v, want %+v", got, want)
	}
	if got, want := puts[1]["artifacts"], []interface{}{x86, arm}; !reflect.DeepEqual(got, want) {
		t.Errorf("second PUT artifacts = %+v, want %+v", got, want)
	}
	// The update API takes seconds; create takes milliseconds.
	if puts[1]["release_date"] != float64(1722128523) {
		t.Errorf("release_date = %v, want 1722128523", puts[1]["release_date"])
	}
}

func TestResourceReleaseDelete(t *testing.T) {
	const activeRelease = `{"version":"2.21.0.0-b1","state":"ACTIVE","universes":[]}`
	cases := []struct {
		name         string
		getStatus    int
		getBody      string
		deleteStatus int
		wantErr      string
		wantDelete   bool
		wantIDKept   bool
	}{
		{
			name:      "in use: names the universe, never deletes",
			getStatus: http.StatusOK,
			getBody: `{"version":"2.21.0.0-b1","state":"ACTIVE",
				"universes":[{"name":"prod-universe","uuid":"uni-1"}]}`,
			wantErr:    "prod-universe (uni-1)",
			wantIDKept: true,
		},
		{
			name:      "already gone",
			getStatus: http.StatusBadRequest,
			getBody:   `{"success":false,"error":"Invalid Release UUID: rel-1"}`,
		},
		{
			name:         "server error surfaces",
			getStatus:    http.StatusOK,
			getBody:      activeRelease,
			deleteStatus: http.StatusInternalServerError,
			wantErr:      "Delete",
			wantDelete:   true,
			wantIDKept:   true,
		},
		{
			name:         "deleted",
			getStatus:    http.StatusOK,
			getBody:      activeRelease,
			deleteStatus: http.StatusOK,
			wantDelete:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deleted := false
			mux := newReleaseMux(t)
			handleJSON(mux, "GET /api/v1/customers/cust-1/ybdb_release/rel-1",
				tc.getStatus, tc.getBody)
			mux.HandleFunc("DELETE /api/v1/customers/cust-1/ybdb_release/rel-1",
				func(w http.ResponseWriter, _ *http.Request) {
					deleted = true
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.deleteStatus)
					_, _ = w.Write([]byte(`{"success":true}`))
				})
			c := newTestAPIClient(t, mux)
			d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{})
			d.SetId("rel-1")

			diags := resourceReleaseDelete(context.Background(), d, c)

			if tc.wantErr == "" && diags.HasError() {
				t.Errorf("unexpected error: %+v", diags)
			}
			if tc.wantErr != "" &&
				(!diags.HasError() || !strings.Contains(diags[0].Summary, tc.wantErr)) {
				t.Errorf("want error containing %q, got %+v", tc.wantErr, diags)
			}
			if deleted != tc.wantDelete {
				t.Errorf("DELETE sent = %v, want %v", deleted, tc.wantDelete)
			}
			if kept := d.Id() != ""; kept != tc.wantIDKept {
				t.Errorf("id kept = %v, want %v", kept, tc.wantIDKept)
			}
		})
	}
}
