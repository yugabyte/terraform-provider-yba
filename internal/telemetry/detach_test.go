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

// auditUniverse builds a single-cluster universe with the given audit/query/metrics
// exporter UUIDs. An empty slice omits the section.
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

// twoClusterUniverse puts the ASYNC cluster at index 0 and PRIMARY at index 1, to
// pin that the primary is identified by clusterType, not array position.
func twoClusterUniverse(asyncExp, primaryExp string) client.UniverseResp {
	mkCluster := func(clusterType, exp string) client.Cluster {
		intent := client.UserIntent{}
		if exp != "" {
			intent.AuditLogConfig = &client.AuditLogConfig{
				YsqlAuditConfig: &client.YSQLAuditConfig{Enabled: true, LogLevel: "LOG"},
				UniverseLogsExporterConfig: []client.UniverseLogsExporterConfig{
					{ExporterUuid: exp},
				},
			}
		}
		return client.Cluster{ClusterType: clusterType, UserIntent: intent}
	}
	return client.UniverseResp{
		UniverseUUID: utils.GetStringPointer("u"),
		Name:         utils.GetStringPointer("u"),
		UniverseDetails: &client.UniverseDefinitionTaskParamsResp{
			Clusters: []client.Cluster{
				mkCluster("ASYNC", asyncExp),
				mkCluster(clusterTypePrimary, primaryExp),
			},
		},
	}
}

// TestPrimaryClusterNotFirst: when the primary isn't clusters[0], detection and
// detach must follow clusterType, not the array index.
func TestPrimaryClusterNotFirst(t *testing.T) {
	u := twoClusterUniverse("" /* async */, "P" /* primary */)
	if !universeReferencesProvider(&u, "P") {
		t.Fatal("provider on the primary cluster (index 1) must be detected")
	}
	spec := buildDetachSpec(&u, "P")
	if spec.TelemetryConfig == nil || spec.TelemetryConfig.AuditLogs != nil {
		t.Errorf("detach must strip P from the primary cluster's config: %+v",
			spec.TelemetryConfig)
	}
}

// TestReadReplicaOnlyReferenceIgnored: a reference only on a read-replica (ASYNC)
// cluster isn't "in use" — YBA's isProviderInUse reads only the primary.
func TestReadReplicaOnlyReferenceIgnored(t *testing.T) {
	u := twoClusterUniverse("P" /* async */, "" /* primary */)
	if universeReferencesProvider(&u, "P") {
		t.Error("a reference only on a read-replica must not count as in-use " +
			"(YBA's isProviderInUse reads only the primary cluster)")
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
	getConfig *clientv2.TelemetryConfig
	getStatus int
	getBody   string

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

// TestProviderDeleteDetachesReferencingUniverses: P is shared by two universes and
// unrelated to a third; delete must detach only the two, then delete P.
func TestProviderDeleteDetachesReferencingUniverses(t *testing.T) {
	f := &fakeYBA{
		universes: []client.UniverseResp{
			auditUniverse("uni-A", []string{"P"}, nil, nil),
			auditUniverse("uni-B", nil, nil, []string{"P"}),
			auditUniverse("uni-C", []string{"other"}, nil, nil),
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
		universes: []client.UniverseResp{
			auditUniverse("uni-C", []string{"other"}, nil, nil),
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
		universes: []client.UniverseResp{
			auditUniverse("uni-A", []string{"P"}, nil, nil),
		},
		// re-list shows P attached again (the race) -> second detach + DELETE.
		deleteFailFirst: true,
		relistOnSecond: []client.UniverseResp{
			auditUniverse("uni-A", []string{"P"}, nil, nil),
		},
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
