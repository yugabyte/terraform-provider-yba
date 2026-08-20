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

package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	client "github.com/yugabyte/platform-go-client"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// plainUniverse builds a minimal universe list entry. Telemetry configs are
// served per-universe by fakeYBA's v2 GET handler (getConfigByUni), matching
// the detach flow, which no longer reads them from the v1 UserIntent.
func plainUniverse(name string) client.UniverseResp {
	return client.UniverseResp{
		UniverseUUID: utils.GetStringPointer(name),
		Name:         utils.GetStringPointer(name),
	}
}

// auditConfig returns a v2 telemetry config exporting audit logs to the given
// providers.
func auditConfig(providers ...string) *clientv2.TelemetryConfig {
	exp := make([]clientv2.UniverseLogsExporterConfig, 0, len(providers))
	for _, p := range providers {
		exp = append(exp, clientv2.UniverseLogsExporterConfig{ExporterUuid: p})
	}
	return &clientv2.TelemetryConfig{
		AuditLogs: &clientv2.AuditLogsTelemetrySpec{Exporters: exp},
	}
}

// masterLogsConfig returns a v2 telemetry config exporting yb-master logs to
// the given providers — the detach flow must see server-log pipelines too.
func masterLogsConfig(providers ...string) *clientv2.TelemetryConfig {
	exp := make([]clientv2.UniverseServerLogsExporterConfig, 0, len(providers))
	for _, p := range providers {
		exp = append(exp, clientv2.UniverseServerLogsExporterConfig{ExporterUuid: p})
	}
	return &clientv2.TelemetryConfig{
		MasterLogs: &clientv2.MasterLogsTelemetrySpec{Exporters: exp},
	}
}

func TestFormatUniverseRefs(t *testing.T) {
	if got := formatUniverseRefs(nil); got != "(none)" {
		t.Errorf("empty = %q want (none)", got)
	}
	one := formatUniverseRefs([]universeRef{{UUID: "u1", Name: "alpha"}})
	if one != "alpha (u1)" {
		t.Errorf("single = %q", one)
	}
	two := formatUniverseRefs([]universeRef{
		{UUID: "u1", Name: "alpha"}, {UUID: "u2", Name: "beta"},
	})
	if two != "alpha (u1), beta (u2)" {
		t.Errorf("multi = %q", two)
	}
}

// fakeYBA is an httptest stand-in for the YBA API the delete flow touches; it
// records calls so tests can assert the detach fanned out to the right universes.
type fakeYBA struct {
	mu              sync.Mutex
	universes       []client.UniverseResp
	configuredUnis  []string // uniUUIDs that received an export-telemetry POST
	deleteCalls     int
	deleteStatus    int // status the DELETE handler returns (0 = 200 OK)
	deleteBody      string
	deleteFailFirst bool // first DELETE returns a 400 "in use"
	relistOnSecond  []client.UniverseResp
	listCallCount   int

	// getStatus, when non-zero, is returned instead of getConfig.
	// getConfigByUni serves per-universe configs (keyed by universe UUID) and
	// wins over getConfig, which is the single-universe fallback.
	getConfig      *clientv2.TelemetryConfig
	getConfigByUni map[string]*clientv2.TelemetryConfig
	getStatus      int
	getBody        string

	// getProviderStatus, when non-zero, drives the GET telemetry_provider handler
	// so Read tests can exercise YBA's non-404 "missing provider" responses.
	getProviderStatus int
	getProviderBody   string

	// createdProviders records every POST /telemetry_provider request body so
	// create tests can assert the exact payload each sink resource sends.
	createdProviders [][]byte
}

func (f *fakeYBA) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/universes"):
			f.listCallCount++
			unis := f.universes
			if f.listCallCount >= 2 && f.relistOnSecond != nil {
				unis = f.relistOnSecond
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(unis)
		case r.Method == http.MethodGet && strings.Contains(path, "/export-telemetry-configs"):
			if f.getStatus != 0 {
				w.WriteHeader(f.getStatus)
				_, _ = w.Write([]byte(f.getBody))
				return
			}
			cfg := f.getConfig
			if f.getConfigByUni != nil {
				parts := strings.Split(path, "/")
				for i, p := range parts {
					if p == "universes" && i+1 < len(parts) {
						cfg = f.getConfigByUni[parts[i+1]]
					}
				}
			}
			if cfg == nil {
				cfg = &clientv2.TelemetryConfig{}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cfg)
		case r.Method == http.MethodPost && strings.Contains(path, "/export-telemetry-configs"):
			parts := strings.Split(path, "/")
			for i, p := range parts {
				if p == "universes" && i+1 < len(parts) {
					f.configuredUnis = append(f.configuredUnis, parts[i+1])
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(clientv2.YBATask{
				TaskUuid: utils.GetStringPointer("task-1"),
			})
		case strings.Contains(path, "/tasks/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"title":"Configure Telemetry","percent":100,` +
				`"status":"Success","details":{"taskDetails":[]}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/telemetry_provider"):
			body, _ := io.ReadAll(r.Body)
			f.createdProviders = append(f.createdProviders, body)
			created := map[string]interface{}{}
			_ = json.Unmarshal(body, &created)
			created["uuid"] = "P"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodGet && strings.Contains(path, "/telemetry_provider/"):
			if f.getProviderStatus != 0 {
				w.WriteHeader(f.getProviderStatus)
				_, _ = w.Write([]byte(f.getProviderBody))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			// Echo the last created provider so the read-after-create in the
			// sink factory sees the type it just sent; default to DATA_DOG.
			if n := len(f.createdProviders); n > 0 {
				resp := map[string]interface{}{}
				_ = json.Unmarshal(f.createdProviders[n-1], &resp)
				resp["uuid"] = "P"
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"uuid":   "P",
				"name":   "p",
				"config": map[string]interface{}{"type": "DATA_DOG"},
			})
		case r.Method == http.MethodDelete && strings.Contains(path, "/telemetry_provider/"):
			f.deleteCalls++
			if f.deleteFailFirst && f.deleteCalls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(
					`{"error":"Cannot delete Telemetry Provider 'P', as it is in use."}`))
				return
			}
			if f.deleteStatus != 0 {
				w.WriteHeader(f.deleteStatus)
				_, _ = w.Write([]byte(f.deleteBody))
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"unhandled ` + r.Method + " " + path + `"}`))
		}
	}
}

func newDetachTestClient(t *testing.T, f *fakeYBA) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")

	cfg := client.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = addr
	cfgV2 := clientv2.NewConfiguration()
	cfgV2.Scheme = "http"
	cfgV2.Host = addr

	return &api.APIClient{
		VanillaClient: &api.VanillaClient{
			Client: srv.Client(), Host: addr, EnableHTTPS: false,
		},
		YugawareClient:   client.NewAPIClient(cfg),
		YugawareClientV2: clientv2.NewAPIClient(cfgV2),
		CustomerID:       "cust",
		APIKey:           "tok",
	}
}

// TestProviderDeleteDetachesReferencingUniverses: P is referenced by two
// universes (one via audit logs, one via the master_logs server-log pipeline)
// and unrelated to a third; delete must detach only the two, then delete P.
func TestProviderDeleteDetachesReferencingUniverses(t *testing.T) {
	f := &fakeYBA{
		universes: []client.UniverseResp{
			plainUniverse("uni-A"),
			plainUniverse("uni-B"),
			plainUniverse("uni-C"),
		},
		getConfigByUni: map[string]*clientv2.TelemetryConfig{
			"uni-A": auditConfig("P"),
			"uni-B": masterLogsConfig("P"),
			"uni-C": auditConfig("other"),
		},
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceDatadogTelemetryProvider()
	d := res.TestResourceData()
	d.SetId("P")

	diags := resourceTelemetryProviderDelete(context.Background(), d, apiClient)
	if diags.HasError() {
		t.Fatalf("delete returned diags: %v", diags)
	}
	if d.Id() != "" {
		t.Errorf("resource id must be cleared after delete, got %q", d.Id())
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteCalls != 1 {
		t.Errorf("expected exactly 1 provider DELETE, got %d", f.deleteCalls)
	}
	if len(f.configuredUnis) != 2 {
		t.Fatalf("expected 2 universes detached, got %v", f.configuredUnis)
	}
	got := map[string]bool{f.configuredUnis[0]: true, f.configuredUnis[1]: true}
	if !got["uni-A"] || !got["uni-B"] {
		t.Errorf("detached the wrong universes: %v", f.configuredUnis)
	}
	if got["uni-C"] {
		t.Error("uni-C does not reference P and must not be reconfigured")
	}
}

// TestProviderDeleteSurfacesUnrelatedError: when DELETE fails but no universe
// references P (not the in-use race), surface the error and keep the id.
func TestProviderDeleteSurfacesUnrelatedError(t *testing.T) {
	f := &fakeYBA{
		universes: []client.UniverseResp{plainUniverse("uni-C")},
		getConfigByUni: map[string]*clientv2.TelemetryConfig{
			"uni-C": auditConfig("other"),
		},
		deleteStatus: http.StatusForbidden,
		deleteBody:   `{"error":"permission denied"}`,
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceDatadogTelemetryProvider()
	d := res.TestResourceData()
	d.SetId("P")

	diags := resourceTelemetryProviderDelete(context.Background(), d, apiClient)
	if !diags.HasError() {
		t.Fatal("expected delete to surface the 403 error")
	}
	if d.Id() == "" {
		t.Error("resource id must be preserved when delete fails for an unrelated reason")
	}
	if f.deleteCalls == 0 {
		t.Error("expected at least one DELETE attempt")
	}
	if len(f.configuredUnis) != 0 {
		t.Errorf("no universe references P, so none should be reconfigured: %v",
			f.configuredUnis)
	}
}

// TestProviderDeleteRecoversFromReattachRace: an external actor re-attaches P in
// the gap, so the first DELETE 400s; the flow must re-detach and retry once.
func TestProviderDeleteRecoversFromReattachRace(t *testing.T) {
	f := &fakeYBA{
		universes: []client.UniverseResp{plainUniverse("uni-A")},
		getConfigByUni: map[string]*clientv2.TelemetryConfig{
			"uni-A": auditConfig("P"),
		},
		// re-list shows P attached again (the race) -> second detach + DELETE.
		deleteFailFirst: true,
		relistOnSecond:  []client.UniverseResp{plainUniverse("uni-A")},
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceDatadogTelemetryProvider()
	d := res.TestResourceData()
	d.SetId("P")

	diags := resourceTelemetryProviderDelete(context.Background(), d, apiClient)
	if diags.HasError() {
		t.Fatalf("delete should recover from the re-attach race, got: %v", diags)
	}
	if d.Id() != "" {
		t.Errorf("resource id must be cleared after a successful retry, got %q", d.Id())
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteCalls != 2 {
		t.Errorf("expected 2 DELETE attempts (fail, then retry), got %d", f.deleteCalls)
	}
	if len(f.configuredUnis) != 2 {
		t.Errorf("expected uni-A detached twice (initial + race), got %v", f.configuredUnis)
	}
}
