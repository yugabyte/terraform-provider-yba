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

// --- pure-function detach edge cases -----------------------------------------

// auditUniverse builds a single-cluster universe whose audit/query/metrics
// exporter lists are the supplied UUID slices. Empty slices omit the section.
func auditUniverse(name string, audit, query, metrics []string) client.UniverseResp {
	intent := client.UserIntent{}
	if len(audit) > 0 {
		exp := make([]client.UniverseLogsExporterConfig, 0, len(audit))
		for _, u := range audit {
			exp = append(exp, client.UniverseLogsExporterConfig{ExporterUuid: u})
		}
		intent.AuditLogConfig = &client.AuditLogConfig{
			YsqlAuditConfig:            &client.YSQLAuditConfig{Enabled: true, LogLevel: "LOG"},
			UniverseLogsExporterConfig: exp,
		}
	}
	if len(query) > 0 {
		exp := make([]client.UniverseQueryLogsExporterConfig, 0, len(query))
		for _, u := range query {
			exp = append(exp, client.UniverseQueryLogsExporterConfig{
				ExporterUuid:     u,
				SendBatchMaxSize: utils.GetInt32Pointer(1000),
				MemoryLimitMib:   utils.GetInt32Pointer(2048),
			})
		}
		intent.QueryLogConfig = &client.QueryLogConfig{UniverseLogsExporterConfig: exp}
	}
	if len(metrics) > 0 {
		exp := make([]client.UniverseMetricsExporterConfig, 0, len(metrics))
		for _, u := range metrics {
			exp = append(exp, client.UniverseMetricsExporterConfig{
				ExporterUuid:  u,
				MetricsPrefix: utils.GetStringPointer("yb."),
			})
		}
		intent.MetricsExportConfig = &client.MetricsExportConfig{
			UniverseMetricsExporterConfig: exp,
			ScrapeConfigTargets:           []string{"MASTER_EXPORT"},
		}
	}
	return client.UniverseResp{
		UniverseUUID: utils.GetStringPointer(name),
		Name:         utils.GetStringPointer(name),
		UniverseDetails: &client.UniverseDefinitionTaskParamsResp{
			Clusters: []client.Cluster{{UserIntent: intent}},
		},
	}
}

// TestBuildDetachSpecDropsAllWhenSoleExporter verifies that when the target
// provider is the ONLY exporter in every section, all sections are dropped and
// the resulting empty telemetry_config disables exporting entirely — without
// touching the universe itself.
func TestBuildDetachSpecDropsAllWhenSoleExporter(t *testing.T) {
	u := auditUniverse("u", []string{"P"}, []string{"P"}, []string{"P"})
	spec := buildDetachSpec(&u, "P")
	if spec.TelemetryConfig == nil {
		t.Fatal("telemetry_config must be non-nil even when empty")
	}
	if spec.TelemetryConfig.AuditLogs != nil ||
		spec.TelemetryConfig.QueryLogs != nil ||
		spec.TelemetryConfig.Metrics != nil {
		t.Errorf("all sections must drop when P is the sole exporter: %+v",
			spec.TelemetryConfig)
	}
	if spec.UpgradeOptions == nil || spec.UpgradeOptions.RollingUpgrade == nil ||
		!*spec.UpgradeOptions.RollingUpgrade {
		t.Error("detach must request a rolling upgrade")
	}
}

// TestBuildDetachSpecPreservesUnrelated verifies a no-op rewrite: when the
// universe does not reference the provider at all, every exporter and its
// per-exporter fields survive untouched.
func TestBuildDetachSpecPreservesUnrelated(t *testing.T) {
	u := auditUniverse("u", []string{"keep"}, []string{"keep"}, []string{"keep"})
	spec := buildDetachSpec(&u, "P-not-here")
	tc := spec.TelemetryConfig
	if tc.AuditLogs == nil || len(tc.AuditLogs.Exporters) != 1 ||
		tc.AuditLogs.Exporters[0].ExporterUuid != "keep" {
		t.Errorf("audit exporters not preserved: %+v", tc.AuditLogs)
	}
	if tc.QueryLogs == nil || len(tc.QueryLogs.Exporters) != 1 {
		t.Errorf("query exporters not preserved: %+v", tc.QueryLogs)
	}
	// Per-exporter batching fields must survive the rewrite.
	if e := tc.QueryLogs.Exporters[0]; e.SendBatchMaxSize == nil || e.MemoryLimitMib == nil {
		t.Errorf("query exporter batching fields lost: %+v", e)
	}
	if tc.Metrics == nil || len(tc.Metrics.Exporters) != 1 {
		t.Errorf("metrics exporters not preserved: %+v", tc.Metrics)
	}
	if e := tc.Metrics.Exporters[0]; e.MetricsPrefix == nil || *e.MetricsPrefix != "yb." {
		t.Errorf("metrics_prefix lost: %+v", e)
	}
	if len(tc.Metrics.ScrapeConfigTargets) != 1 {
		t.Errorf("scrape targets lost: %+v", tc.Metrics.ScrapeConfigTargets)
	}
}

// TestBuildDetachSpecKeepsSiblingExporter verifies that when one section has
// the target alongside a sibling exporter, only the target is filtered and the
// section's sub-config (ysql audit) is preserved for the survivor.
func TestBuildDetachSpecKeepsSiblingExporter(t *testing.T) {
	u := auditUniverse("u", []string{"keep", "P"}, nil, nil)
	spec := buildDetachSpec(&u, "P")
	a := spec.TelemetryConfig.AuditLogs
	if a == nil {
		t.Fatal("audit section dropped even though a sibling exporter remains")
	}
	if len(a.Exporters) != 1 || a.Exporters[0].ExporterUuid != "keep" {
		t.Errorf("audit exporters after detach = %+v want [keep]", a.Exporters)
	}
	if a.YsqlAuditConfig == nil || !a.YsqlAuditConfig.Enabled {
		t.Error("ysql audit sub-config must be preserved for the surviving exporter")
	}
}

// TestBuildDetachSpecNoClusters guards the degenerate universe shape (no
// clusters) — buildDetachSpec must not panic and must return an empty config.
func TestBuildDetachSpecNoClusters(t *testing.T) {
	u := client.UniverseResp{
		UniverseUUID:    utils.GetStringPointer("u"),
		UniverseDetails: &client.UniverseDefinitionTaskParamsResp{},
	}
	spec := buildDetachSpec(&u, "P")
	if spec.TelemetryConfig == nil || spec.TelemetryConfig.AuditLogs != nil {
		t.Errorf("expected empty telemetry_config for clusterless universe: %+v",
			spec.TelemetryConfig)
	}
}

// TestFormatUniverseRefs covers the human-readable rendering used in delete
// log lines and error messages.
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

// --- full delete-with-detach integration -------------------------------------

// fakeYBA is an httptest-backed stand-in for the subset of the YBA API the
// telemetry-provider delete flow touches: list universes, configure export
// telemetry (per universe), poll task status, and delete the provider. It
// records the calls so tests can assert the detach fanned out to exactly the
// referencing universes.
type fakeYBA struct {
	mu              sync.Mutex
	universes       []client.UniverseResp
	configuredUnis  []string // uniUUIDs that received an export-telemetry POST
	deleteCalls     int
	deleteStatus    int    // status code the DELETE handler returns (0 = 200 OK)
	deleteBody      string // body the DELETE handler returns
	deleteFailFirst bool   // when set, the FIRST DELETE returns a 400 "in use"
	relistOnSecond  []client.UniverseResp
	listCallCount   int

	// getConfig is returned by the GET export-telemetry-configs handler (used
	// by the Read tests). getStatus, when non-zero, is returned instead.
	getConfig *clientv2.TelemetryConfig
	getStatus int
	getBody   string

	// getProviderStatus/getProviderBody, when getProviderStatus is non-zero,
	// drive the GET telemetry_provider/{uuid} handler so the provider Read
	// tests can exercise YBA's non-404 "missing provider" responses.
	getProviderStatus int
	getProviderBody   string
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
			if cfg == nil {
				cfg = &clientv2.TelemetryConfig{}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cfg)
		case r.Method == http.MethodPost && strings.Contains(path, "/export-telemetry-configs"):
			// .../universes/{uniUUID}/export-telemetry-configs
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
		case r.Method == http.MethodGet && strings.Contains(path, "/telemetry_provider/"):
			if f.getProviderStatus != 0 {
				w.WriteHeader(f.getProviderStatus)
				_, _ = w.Write([]byte(f.getProviderBody))
				return
			}
			w.Header().Set("Content-Type", "application/json")
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

// TestProviderDeleteDetachesReferencingUniverses is the headline integration
// test: provider "P" is shared by two universes (one via audit, one via
// metrics) and unrelated to a third. Deleting it must detach P from exactly
// the two referencing universes (each through its own export-telemetry POST +
// task wait), then delete the provider and clear the resource id. The third
// universe must be left alone.
func TestProviderDeleteDetachesReferencingUniverses(t *testing.T) {
	f := &fakeYBA{
		universes: []client.UniverseResp{
			auditUniverse("uni-A", []string{"P"}, nil, nil),
			auditUniverse("uni-B", nil, nil, []string{"P"}),
			auditUniverse("uni-C", []string{"other"}, nil, nil),
		},
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceTelemetryProvider()
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

// TestProviderDeleteSurfacesUnrelatedError verifies the careful branch in the
// delete flow: when YBA rejects the DELETE but NO universe references the
// provider (so it is not the in-use race we know how to recover from), the
// original error is surfaced verbatim and the resource id is preserved — we
// must never silently drop a resource we failed to delete.
func TestProviderDeleteSurfacesUnrelatedError(t *testing.T) {
	f := &fakeYBA{
		universes: []client.UniverseResp{
			auditUniverse("uni-C", []string{"other"}, nil, nil),
		},
		deleteStatus: http.StatusForbidden,
		deleteBody:   `{"error":"permission denied"}`,
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceTelemetryProvider()
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

// TestProviderDeleteRecoversFromReattachRace exercises the trickiest delete
// branch: the provider is detached and the first DELETE still fails with YBA's
// "in use" 400 because an external actor re-attached it in the gap. The flow
// must re-list, notice P is referenced again, detach a second time, and retry
// the DELETE — succeeding without ever surfacing the transient 400 or leaving
// a half-deleted resource. This is the safeguard that keeps a
// destroy-and-recreate plan from corrupting a universe's collector config.
func TestProviderDeleteRecoversFromReattachRace(t *testing.T) {
	f := &fakeYBA{
		universes: []client.UniverseResp{
			auditUniverse("uni-A", []string{"P"}, nil, nil),
		},
		// The first DELETE fails "in use"; the re-list then shows P attached
		// again (the race), driving a second detach + a second DELETE.
		deleteFailFirst: true,
		relistOnSecond: []client.UniverseResp{
			auditUniverse("uni-A", []string{"P"}, nil, nil),
		},
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceTelemetryProvider()
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
