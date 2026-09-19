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
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// newTestAPIClient points both the real generated client and the
// VanillaClient at the same stub server, mirroring how the provider wires
// them in production.
func newTestAPIClient(t *testing.T, handler http.Handler) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := client.NewConfiguration()
	cfg.Host = host
	cfg.Scheme = "http"
	return &api.APIClient{
		YugawareClient: client.NewAPIClient(cfg),
		VanillaClient:  &api.VanillaClient{Client: srv.Client(), Host: host, EnableHTTPS: false},
		APIKey:         "test-token",
		CustomerID:     "cust-1",
	}
}

func newReleaseMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/app_version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"2024.2.0.0-b5"}`))
	})
	return mux
}

func writeTestTarball(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "yugabyte-2.21.0.0-b1-linux-x86_64.tar.gz")
	if err := os.WriteFile(path, []byte("test-tarball-content"), 0600); err != nil {
		t.Fatalf("write test tarball: %v", err)
	}
	return path
}

func TestResourceReleaseSchemaSanity(t *testing.T) {
	r := ResourceRelease()
	if r.Importer == nil || r.Importer.StateContext == nil {
		t.Error("importer must be set")
	}
	if r.Timeouts == nil || r.Timeouts.Create == nil || r.Timeouts.Update == nil ||
		r.Timeouts.Delete == nil {
		t.Error("create/update/delete timeouts must be set")
	}
	if !r.Schema["version"].Required || !r.Schema["version"].ForceNew {
		t.Error("version must be Required and ForceNew")
	}
	if !r.Schema["release_type"].ForceNew {
		t.Error("release_type must be ForceNew: the update API cannot change it")
	}
	artifact := r.Schema["artifact"]
	if artifact.Type != schema.TypeList || !artifact.Required || artifact.MinItems != 1 {
		t.Error("artifact must be a Required TypeList with MinItems 1")
	}
	artifactSchema := artifact.Elem.(*schema.Resource).Schema
	for _, computed := range []string{"package_file_id", "sha256"} {
		if !artifactSchema[computed].Computed {
			t.Errorf("artifact %s must be Computed", computed)
		}
	}
	if !artifactSchema["platform"].Required {
		t.Error("artifact platform must be Required")
	}
	if r.Description == "" {
		t.Error("resource must have a Description")
	}
	for name, field := range r.Schema {
		if field.Description == "" {
			t.Errorf("field %s must have a Description", name)
		}
	}
	for name, field := range artifactSchema {
		if field.Description == "" {
			t.Errorf("artifact field %s must have a Description", name)
		}
	}
}

func TestResourceReleaseCreate(t *testing.T) {
	tarball := writeTestTarball(t)

	var createBody map[string]interface{}
	uploadHits := 0
	mux := newReleaseMux(t)
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release/upload",
		func(w http.ResponseWriter, r *http.Request) {
			uploadHits++
			if _, err := r.MultipartReader(); err != nil {
				t.Errorf("upload is not multipart: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resourceUUID":"file-1"}`))
		})
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/upload/file-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"metadata_uuid":"file-1","version":"2.21.0.0-b1","yb_type":"YBDB",
				"sha256":"sha-1","platform":"LINUX","architecture":"x86_64",
				"release_type":"PREVIEW","release_date_msecs":1722128523000,
				"status":"success"
			}`))
		})
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release",
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resourceUUID":"rel-1"}`))
		})
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"release_uuid":"rel-1","version":"2.21.0.0-b1","yb_type":"YBDB",
				"release_type":"PREVIEW","state":"ACTIVE","release_date_msecs":1722128523000,
				"artifacts":[{"platform":"LINUX","architecture":"x86_64",
					"package_file_id":"file-1","package_url":""}],
				"universes":[]
			}`))
		})

	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{
		"version": "2.21.0.0-b1",
		"artifact": []interface{}{
			map[string]interface{}{
				"platform":     "LINUX",
				"architecture": "x86_64",
				"local_file":   tarball,
			},
		},
	})

	diags := resourceReleaseCreate(context.Background(), d, c)
	if diags.HasError() {
		t.Fatalf("create failed: %+v", diags)
	}
	if d.Id() != "rel-1" {
		t.Errorf("expected id rel-1, got %q", d.Id())
	}
	if uploadHits != 1 {
		t.Errorf("expected 1 upload, got %d", uploadHits)
	}

	if createBody["yb_type"] != "YBDB" || createBody["version"] != "2.21.0.0-b1" {
		t.Errorf("create body basics wrong: %+v", createBody)
	}
	// release_type and release_date_msecs inferred from extracted metadata.
	if createBody["release_type"] != "PREVIEW" {
		t.Errorf("release_type not inferred from metadata: %+v", createBody)
	}
	if createBody["release_date_msecs"] != float64(1722128523000) {
		t.Errorf("release_date_msecs not inferred from metadata: %+v", createBody)
	}
	artifacts := createBody["artifacts"].([]interface{})
	created := artifacts[0].(map[string]interface{})
	if created["package_file_id"] != "file-1" || created["sha256"] != "sha-1" {
		t.Errorf("artifact must carry uploaded file id and sha256: %+v", created)
	}

	// Uploaded identifiers must survive into state (Read cannot recover sha256).
	if got := d.Get("artifact.0.package_file_id").(string); got != "file-1" {
		t.Errorf("package_file_id not in state: %q", got)
	}
	if got := d.Get("artifact.0.sha256").(string); got != "sha-1" {
		t.Errorf("sha256 not in state: %q", got)
	}
	if got := d.Get("state").(string); got != "ACTIVE" {
		t.Errorf("state not read back: %q", got)
	}
}

func TestResourceReleaseCreateVersionMismatch(t *testing.T) {
	tarball := writeTestTarball(t)

	registerHit := false
	mux := newReleaseMux(t)
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release/upload",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resourceUUID":"file-1"}`))
		})
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/upload/file-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"version":"2.22.0.0-b7","sha256":"sha-1","platform":"LINUX",
				"architecture":"x86_64","release_type":"PREVIEW","status":"success"
			}`))
		})
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release",
		func(_ http.ResponseWriter, _ *http.Request) { registerHit = true })

	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{
		"version": "2.21.0.0-b1",
		"artifact": []interface{}{
			map[string]interface{}{
				"platform":     "LINUX",
				"architecture": "x86_64",
				"local_file":   tarball,
			},
		},
	})

	diags := resourceReleaseCreate(context.Background(), d, c)
	if !diags.HasError() {
		t.Fatal("expected error for tarball/release version mismatch")
	}
	if !strings.Contains(diags[0].Summary, "does not match release version") {
		t.Errorf("unexpected error: %+v", diags)
	}
	if registerHit {
		t.Error("release must not be registered after a metadata mismatch")
	}
	if d.Id() != "" {
		t.Errorf("id must stay unset, got %q", d.Id())
	}
}

func TestResourceReleaseReadGone(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, ``},
		{
			"400 invalid release uuid",
			http.StatusBadRequest,
			`{"success":false,"error":"Invalid Release UUID: rel-1"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := newReleaseMux(t)
			mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/rel-1",
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				})

			c := newTestAPIClient(t, mux)
			d := schema.TestResourceDataRaw(t, ResourceRelease().Schema,
				map[string]interface{}{})
			d.SetId("rel-1")

			diags := resourceReleaseRead(context.Background(), d, c)
			if diags.HasError() {
				t.Fatalf("read of a missing release must not error: %+v", diags)
			}
			if d.Id() != "" {
				t.Errorf("id must be cleared for a missing release, got %q", d.Id())
			}
		})
	}
}

// Guards the sha256/local_file preservation contract: the GET response omits
// sha256 and never knew local paths, so Read must carry both over from state
// while refreshing server-owned fields.
func TestResourceReleaseReadPreservesLocalState(t *testing.T) {
	mux := newReleaseMux(t)
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"release_uuid":"rel-1","version":"2.21.0.0-b1","yb_type":"YBDB",
				"release_type":"PREVIEW","state":"ACTIVE",
				"artifacts":[
					{"platform":"LINUX","architecture":"x86_64",
						"package_file_id":"file-new","package_url":""},
					{"platform":"LINUX","architecture":"aarch64",
						"package_file_id":"file-arm","package_url":""}
				],
				"universes":[]
			}`))
		})

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
		},
	})
	d.SetId("rel-1")

	if diags := resourceReleaseRead(context.Background(), d, c); diags.HasError() {
		t.Fatalf("read failed: %+v", diags)
	}
	if got := d.Get("artifact.0.local_file").(string); got != "/tmp/yugabyte-x86_64.tar.gz" {
		t.Errorf("local_file must be preserved from state, got %q", got)
	}
	if got := d.Get("artifact.0.sha256").(string); got != "sha-keep" {
		t.Errorf("sha256 must be preserved from state, got %q", got)
	}
	if got := d.Get("artifact.0.package_file_id").(string); got != "file-new" {
		t.Errorf("package_file_id must be refreshed from YBA, got %q", got)
	}
	// The aarch64 artifact YBA reports but state does not know appears as a
	// trailing block with no local identifiers.
	if got := d.Get("artifact.1.architecture").(string); got != "aarch64" {
		t.Errorf("out-of-band artifact not appended, got architecture %q", got)
	}
	if got := d.Get("artifact.1.sha256").(string); got != "" {
		t.Errorf("out-of-band artifact must have empty sha256, got %q", got)
	}
}

func TestResourceReleaseUpdate(t *testing.T) {
	var putBody map[string]interface{}
	putHits := 0
	mux := newReleaseMux(t)
	mux.HandleFunc("PUT /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, r *http.Request) {
			putHits++
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &putBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
	mux.HandleFunc("POST /api/v1/customers/cust-1/ybdb_release/upload",
		func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("no upload expected when local_file is unchanged")
		})
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"release_uuid":"rel-1","version":"2.21.0.0-b1","yb_type":"YBDB",
				"release_type":"PREVIEW","state":"ACTIVE","release_tag":"new-tag",
				"artifacts":[{"platform":"LINUX","architecture":"x86_64",
					"package_file_id":"file-1","package_url":""}],
				"universes":[]
			}`))
		})

	c := newTestAPIClient(t, mux)
	// Prior state (not raw config): the unchanged-artifact reuse path only
	// triggers when the old value carries the uploaded identifiers.
	d := ResourceRelease().Data(&terraform.InstanceState{
		ID: "rel-1",
		Attributes: map[string]string{
			"version":                    "2.21.0.0-b1",
			"release_tag":                "new-tag",
			"release_date_msecs":         "1722128523123",
			"state":                      "ACTIVE",
			"artifact.#":                 "1",
			"artifact.0.platform":        "LINUX",
			"artifact.0.architecture":    "x86_64",
			"artifact.0.local_file":      "/tmp/yugabyte-x86_64.tar.gz",
			"artifact.0.package_url":     "",
			"artifact.0.package_file_id": "file-1",
			"artifact.0.sha256":          "sha-1",
		},
	})

	if diags := resourceReleaseUpdate(context.Background(), d, c); diags.HasError() {
		t.Fatalf("update failed: %+v", diags)
	}
	if putHits != 1 {
		t.Fatalf("expected exactly one PUT, got %d", putHits)
	}
	// The update API takes seconds, not msecs.
	if putBody["release_date"] != float64(1722128523) {
		t.Errorf("release_date must be in seconds: %+v", putBody["release_date"])
	}
	if putBody["release_tag"] != "new-tag" || putBody["state"] != "ACTIVE" {
		t.Errorf("plan values lost in PUT body: %+v", putBody)
	}
	artifacts := putBody["artifacts"].([]interface{})
	updated := artifacts[0].(map[string]interface{})
	// Built from state, never from a GET response (which omits sha256).
	if updated["package_file_id"] != "file-1" || updated["sha256"] != "sha-1" {
		t.Errorf("artifact identifiers lost in PUT body: %+v", updated)
	}
	if _, present := updated["package_url"]; present {
		t.Errorf("file-based artifact must not carry package_url: %+v", updated)
	}
}

func TestResourceReleaseDeleteInUse(t *testing.T) {
	deleteHit := false
	mux := newReleaseMux(t)
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"release_uuid":"rel-1","version":"2.21.0.0-b1","yb_type":"YBDB",
				"release_type":"PREVIEW","state":"ACTIVE","artifacts":[],
				"universes":[{"name":"prod-universe","uuid":"uni-1"}]
			}`))
		})
	mux.HandleFunc("DELETE /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(_ http.ResponseWriter, _ *http.Request) { deleteHit = true })

	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{})
	d.SetId("rel-1")

	diags := resourceReleaseDelete(context.Background(), d, c)
	if !diags.HasError() {
		t.Fatal("expected error deleting an in-use release")
	}
	if !strings.Contains(diags[0].Summary, "prod-universe (uni-1)") {
		t.Errorf("error must name the referencing universe: %+v", diags)
	}
	if deleteHit {
		t.Error("DELETE must not be attempted for an in-use release")
	}
	if d.Id() == "" {
		t.Error("id must not be cleared when delete fails")
	}
}

func TestResourceReleaseDeleteIdempotent(t *testing.T) {
	mux := newReleaseMux(t)
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"success":false,"error":"Invalid Release UUID: rel-1"}`))
		})

	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{})
	d.SetId("rel-1")

	if diags := resourceReleaseDelete(context.Background(), d, c); diags.HasError() {
		t.Fatalf("delete of an already-gone release must not error: %+v", diags)
	}
	if d.Id() != "" {
		t.Errorf("id must be cleared, got %q", d.Id())
	}
}

func TestResourceReleaseDeleteSurfacesErrors(t *testing.T) {
	mux := newReleaseMux(t)
	mux.HandleFunc("GET /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"release_uuid":"rel-1","version":"2.21.0.0-b1","yb_type":"YBDB",
				"release_type":"PREVIEW","state":"ACTIVE","artifacts":[],"universes":[]
			}`))
		})
	mux.HandleFunc("DELETE /api/v1/customers/cust-1/ybdb_release/rel-1",
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		})

	c := newTestAPIClient(t, mux)
	d := schema.TestResourceDataRaw(t, ResourceRelease().Schema, map[string]interface{}{})
	d.SetId("rel-1")

	if diags := resourceReleaseDelete(context.Background(), d, c); !diags.HasError() {
		t.Fatal("non-404 delete failures must surface")
	}
	if d.Id() == "" {
		t.Error("id must not be cleared when delete fails")
	}
}
