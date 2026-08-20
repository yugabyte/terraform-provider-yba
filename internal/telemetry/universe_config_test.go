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
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestBuildSpecMultipleExportersAllSections(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"audit_logs": []interface{}{map[string]interface{}{
			"ysql_audit_config": []interface{}{map[string]interface{}{
				"classes": []interface{}{"READ", "WRITE", "DDL"},
			}},
			"exporter": []interface{}{
				map[string]interface{}{"exporter_uuid": "shared"},
				map[string]interface{}{"exporter_uuid": "audit-only"},
			},
		}},
		"query_logs": []interface{}{map[string]interface{}{
			"ysql_query_log_config": []interface{}{map[string]interface{}{}},
			"exporter": []interface{}{
				map[string]interface{}{
					"exporter_uuid":       "shared",
					"send_batch_max_size": 500,
				},
				map[string]interface{}{"exporter_uuid": "query-only"},
			},
		}},
		"metrics": []interface{}{map[string]interface{}{
			"scrape_config_targets": []interface{}{"MASTER_EXPORT", "TSERVER_EXPORT"},
			"exporter": []interface{}{
				map[string]interface{}{
					"exporter_uuid":  "shared",
					"metrics_prefix": "ybdb.",
				},
				map[string]interface{}{"exporter_uuid": "metrics-2"},
				map[string]interface{}{"exporter_uuid": "metrics-3"},
			},
		}},
	})

	spec := buildExportTelemetryConfigSpec(d)
	tc := spec.TelemetryConfig
	if tc == nil || tc.AuditLogs == nil || tc.QueryLogs == nil || tc.Metrics == nil {
		t.Fatalf("expected all three sections populated, got %+v", tc)
	}
	if len(tc.AuditLogs.Exporters) != 2 {
		t.Errorf("audit exporters = %d want 2", len(tc.AuditLogs.Exporters))
	}
	if len(tc.QueryLogs.Exporters) != 2 {
		t.Errorf("query exporters = %d want 2", len(tc.QueryLogs.Exporters))
	}
	if len(tc.Metrics.Exporters) != 3 {
		t.Errorf("metrics exporters = %d want 3", len(tc.Metrics.Exporters))
	}

	// Audit's v2 type has no batching fields — structurally uuid + tags only.
	for _, e := range tc.AuditLogs.Exporters {
		if e.ExporterUuid == "" {
			t.Errorf("audit exporter missing uuid: %+v", e)
		}
	}
	// Query/metrics carry batching (schema defaults fill them even when omitted).
	for _, e := range tc.QueryLogs.Exporters {
		if e.SendBatchMaxSize == nil || e.MemoryLimitMib == nil {
			t.Errorf("query exporter %q missing batching fields: %+v",
				e.ExporterUuid, e)
		}
	}
	var prefixed, bare int
	for _, e := range tc.Metrics.Exporters {
		if e.MetricsPrefix != nil && *e.MetricsPrefix == "ybdb." {
			prefixed++
		}
		if e.MetricsPrefix == nil || *e.MetricsPrefix == "" {
			bare++
		}
	}
	if prefixed != 1 {
		t.Errorf("expected exactly one exporter with metrics_prefix=ybdb., got %d", prefixed)
	}
	if bare != 2 {
		t.Errorf("expected two exporters with no metrics_prefix, got %d", bare)
	}
}

// Regression: repeated exporter blocks each map to a distinct array entry, in order.
func TestBuildMetricsMultipleExporters(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"metrics": []interface{}{map[string]interface{}{
			"exporter": []interface{}{
				map[string]interface{}{"exporter_uuid": "exp-a", "metrics_prefix": "a."},
				map[string]interface{}{"exporter_uuid": "exp-b", "metrics_prefix": "b."},
				map[string]interface{}{"exporter_uuid": "exp-c"},
			},
		}},
	})

	got := buildExportTelemetryConfigSpec(d).TelemetryConfig.Metrics.Exporters
	if len(got) != 3 {
		t.Fatalf("expected 3 metrics exporters, got %d: %+v", len(got), got)
	}
	for i, want := range []string{"exp-a", "exp-b", "exp-c"} {
		if got[i].ExporterUuid != want {
			t.Errorf("exporter[%d] uuid = %q want %q (order/identity not preserved)",
				i, got[i].ExporterUuid, want)
		}
	}
	if got[0].MetricsPrefix == nil || *got[0].MetricsPrefix != "a." {
		t.Errorf("exporter[0] metrics_prefix = %v want \"a.\"", got[0].MetricsPrefix)
	}
	if got[1].MetricsPrefix == nil || *got[1].MetricsPrefix != "b." {
		t.Errorf("exporter[1] metrics_prefix = %v want \"b.\"", got[1].MetricsPrefix)
	}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// Regression: YBA echoes audit classes / scrape targets in any order (Java Sets);
// TypeSet must treat orderings as equal, else a phantom diff loops forever.
func TestTypeSetIgnoresOrder(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()

	mk := func(classes, targets []interface{}) *schema.ResourceData {
		return schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"universe_uuid": "uni-1",
			"audit_logs": []interface{}{map[string]interface{}{
				"ysql_audit_config": []interface{}{map[string]interface{}{
					"classes": classes,
				}},
				"exporter": []interface{}{
					map[string]interface{}{"exporter_uuid": "e"},
				},
			}},
			"metrics": []interface{}{map[string]interface{}{
				"scrape_config_targets": targets,
				"exporter": []interface{}{
					map[string]interface{}{"exporter_uuid": "e"},
				},
			}},
		})
	}

	a := mk(
		[]interface{}{"READ", "WRITE", "DDL"},
		[]interface{}{"MASTER_EXPORT", "TSERVER_EXPORT", "NODE_EXPORT"},
	)
	b := mk(
		[]interface{}{"DDL", "READ", "WRITE"},
		[]interface{}{"NODE_EXPORT", "MASTER_EXPORT", "TSERVER_EXPORT"},
	)

	classesA := a.Get("audit_logs.0.ysql_audit_config.0.classes").(*schema.Set)
	classesB := b.Get("audit_logs.0.ysql_audit_config.0.classes").(*schema.Set)
	if !classesA.Equal(classesB) {
		t.Errorf("audit classes Sets differ by order: %v vs %v",
			classesA.List(), classesB.List())
	}

	targetsA := a.Get("metrics.0.scrape_config_targets").(*schema.Set)
	targetsB := b.Get("metrics.0.scrape_config_targets").(*schema.Set)
	if !targetsA.Equal(targetsB) {
		t.Errorf("scrape targets Sets differ by order: %v vs %v",
			targetsA.List(), targetsB.List())
	}

	specA := buildExportTelemetryConfigSpec(a)
	specB := buildExportTelemetryConfigSpec(b)
	clA := sortedStrings(specA.TelemetryConfig.AuditLogs.YsqlAuditConfig.Classes)
	clB := sortedStrings(specB.TelemetryConfig.AuditLogs.YsqlAuditConfig.Classes)
	if len(clA) != 3 || !equalStrings(clA, clB) {
		t.Errorf("audit classes membership differs: %v vs %v", clA, clB)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Round-trips a populated config Read->state->build; drift here is the perpetual
// phantom diff after a successful apply.
func TestFlattenRoundTripNoPhantomDiff(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()

	auditTags := map[string]string{"env": "prod"}
	audit := &clientv2.AuditLogsTelemetrySpec{
		YsqlAuditConfig: &clientv2.YSQLAuditConfig{
			Enabled:  true,
			Classes:  []string{"WRITE", "READ", "DDL"}, // server order differs
			LogLevel: utils.GetStringPointer("WARNING"),
		},
		YcqlAuditConfig: &clientv2.YCQLAuditConfig{
			Enabled:            true,
			LogLevel:           utils.GetStringPointer("ERROR"),
			IncludedCategories: []string{"DML", "DDL"},
		},
		Exporters: []clientv2.UniverseLogsExporterConfig{
			{ExporterUuid: "exp-1", AdditionalTags: &auditTags},
		},
	}
	metrics := &clientv2.MetricsTelemetrySpec{
		ScrapeIntervalSeconds: utils.GetInt32Pointer(45),
		ScrapeTimeoutSeconds:  utils.GetInt32Pointer(15),
		CollectionLevel:       utils.GetStringPointer("NORMAL"),
		ScrapeConfigTargets: []clientv2.ScrapeConfigTargetType{
			"TSERVER_EXPORT", "MASTER_EXPORT",
		},
		Exporters: []clientv2.UniverseMetricsExporterConfig{
			{
				ExporterUuid:   "exp-1",
				SendBatchSize:  utils.GetInt32Pointer(200),
				MemoryLimitMib: utils.GetInt32Pointer(4096),
				MetricsPrefix:  utils.GetStringPointer("yb."),
			},
		},
	}

	d := schema.TestResourceDataRaw(t, res.Schema,
		map[string]interface{}{"universe_uuid": "uni-1"})
	if err := d.Set("audit_logs", flattenAuditLogsSpec(audit)); err != nil {
		t.Fatalf("set audit_logs: %v", err)
	}
	if err := d.Set("metrics", flattenMetricsSpec(metrics)); err != nil {
		t.Fatalf("set metrics: %v", err)
	}

	spec := buildExportTelemetryConfigSpec(d)
	ysql := spec.TelemetryConfig.AuditLogs.YsqlAuditConfig
	if !ysql.Enabled || ysql.GetLogLevel() != "WARNING" {
		t.Errorf("ysql audit lost in round trip: %+v", ysql)
	}
	if got := sortedStrings(ysql.Classes); !equalStrings(got, []string{"DDL", "READ", "WRITE"}) {
		t.Errorf("audit classes round trip = %v", got)
	}
	ycql := spec.TelemetryConfig.AuditLogs.YcqlAuditConfig
	if ycql == nil || ycql.GetLogLevel() != "ERROR" {
		t.Errorf("ycql audit lost in round trip: %+v", ycql)
	}
	m := spec.TelemetryConfig.Metrics
	if m.ScrapeIntervalSeconds == nil || *m.ScrapeIntervalSeconds != 45 {
		t.Errorf("scrape interval round trip = %v", m.ScrapeIntervalSeconds)
	}
	if len(m.ScrapeConfigTargets) != 2 {
		t.Errorf("scrape targets round trip = %v", m.ScrapeConfigTargets)
	}
	if len(m.Exporters) != 1 || m.Exporters[0].ExporterUuid != "exp-1" {
		t.Fatalf("metrics exporter round trip = %+v", m.Exporters)
	}
	if e := m.Exporters[0]; e.MetricsPrefix == nil || *e.MetricsPrefix != "yb." {
		t.Errorf("metrics_prefix round trip = %v", e.MetricsPrefix)
	}
	if a := spec.TelemetryConfig.AuditLogs.Exporters; len(a) != 1 ||
		a[0].AdditionalTags == nil || (*a[0].AdditionalTags)["env"] != "prod" {
		t.Errorf("audit exporter tags round trip = %+v", spec.TelemetryConfig.AuditLogs.Exporters)
	}
}

// No blocks -> every section nil (disables everything), not empty structs
// that trip YBA's "export active but no exporter" check.
func TestEmptySectionsDisabled(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema,
		map[string]interface{}{"universe_uuid": "uni-1"})
	spec := buildExportTelemetryConfigSpec(d)
	if spec.TelemetryConfig == nil {
		t.Fatal("telemetry_config must always be set (empty disables exporters)")
	}
	if !telemetryConfigIsEmpty(spec.TelemetryConfig) {
		t.Errorf("all sections must be nil when unconfigured: %+v", spec.TelemetryConfig)
	}
}

// Regression: YBA forces enabled=true from block presence (readOnly). Provider must
// send true when the block is declared and never surface "enabled" (would diff forever).
func TestEnabledDerivedFromBlockPresence(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"audit_logs": []interface{}{map[string]interface{}{
			"ysql_audit_config": []interface{}{map[string]interface{}{
				"classes": []interface{}{"DDL"},
			}},
			"ycql_audit_config": []interface{}{map[string]interface{}{
				"log_level": "WARNING",
			}},
			"exporter": []interface{}{map[string]interface{}{"exporter_uuid": "e"}},
		}},
		"query_logs": []interface{}{map[string]interface{}{
			"ysql_query_log_config": []interface{}{map[string]interface{}{
				"log_statement": "ALL",
			}},
			"exporter": []interface{}{map[string]interface{}{"exporter_uuid": "e"}},
		}},
	})

	tc := buildExportTelemetryConfigSpec(d).TelemetryConfig
	if tc.AuditLogs == nil || tc.AuditLogs.YsqlAuditConfig == nil ||
		!tc.AuditLogs.YsqlAuditConfig.Enabled {
		t.Errorf("ysql audit enabled must be true from block presence: %+v", tc.AuditLogs)
	}
	if tc.AuditLogs.YcqlAuditConfig == nil || !tc.AuditLogs.YcqlAuditConfig.Enabled {
		t.Error("ycql audit enabled must be true from block presence")
	}
	if tc.QueryLogs == nil || tc.QueryLogs.YsqlQueryLogConfig == nil ||
		!tc.QueryLogs.YsqlQueryLogConfig.Enabled {
		t.Error("ysql query-log enabled must be true from block presence")
	}

	// Flatten (Read) must not emit an "enabled" key.
	flatAudit := flattenAuditLogsSpec(tc.AuditLogs)
	auditMap := flatAudit[0].(map[string]interface{})
	for _, block := range []string{"ysql_audit_config", "ycql_audit_config"} {
		sub := auditMap[block].([]interface{})[0].(map[string]interface{})
		if _, ok := sub["enabled"]; ok {
			t.Errorf("flattened %s must not contain an 'enabled' key", block)
		}
	}
	flatQuery := flattenQueryLogsSpec(tc.QueryLogs)
	qsub := flatQuery[0].(map[string]interface{})["ysql_query_log_config"].([]interface{})[0].(map[string]interface{})
	if _, ok := qsub["enabled"]; ok {
		t.Error("flattened ysql_query_log_config must not contain an 'enabled' key")
	}

	// Flattened shape must round-trip into state (a stray unknown key surfaces here).
	if err := d.Set("audit_logs", flatAudit); err != nil {
		t.Fatalf("set flattened audit_logs: %v", err)
	}
	if err := d.Set("query_logs", flatQuery); err != nil {
		t.Fatalf("set flattened query_logs: %v", err)
	}
}

// Server-log pipelines: raw config -> spec -> flatten -> state -> spec must be
// stable (the perpetual-diff regression) and batching defaults must be filled.
func TestServerLogsBuildFlattenRoundTrip(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"master_logs": []interface{}{map[string]interface{}{
			"min_level":               "ERROR",
			"noise_sample_drop_ratio": 0.25,
			"exporter": []interface{}{map[string]interface{}{
				"exporter_uuid":   "exp-1",
				"additional_tags": map[string]interface{}{"env": "prod"},
			}},
		}},
		"tserver_logs": []interface{}{map[string]interface{}{
			"exporter": []interface{}{map[string]interface{}{
				"exporter_uuid":       "exp-1",
				"send_batch_max_size": 500,
			}},
		}},
		"controller_logs": []interface{}{map[string]interface{}{
			"exporter": []interface{}{map[string]interface{}{
				"exporter_uuid": "exp-2",
			}},
		}},
	})

	tc := buildExportTelemetryConfigSpec(d).TelemetryConfig
	ml := tc.MasterLogs
	if ml == nil || len(ml.Exporters) != 1 || ml.Exporters[0].ExporterUuid != "exp-1" {
		t.Fatalf("master_logs spec = %+v", ml)
	}
	if ml.MinLevel == nil || *ml.MinLevel != "ERROR" {
		t.Errorf("master min_level = %v want ERROR", ml.MinLevel)
	}
	if ml.NoiseSampleDropRatio == nil || *ml.NoiseSampleDropRatio != 0.25 {
		t.Errorf("noise_sample_drop_ratio = %v want 0.25", ml.NoiseSampleDropRatio)
	}
	if e := ml.Exporters[0]; e.SendBatchMaxSize == nil || e.MemoryLimitMib == nil {
		t.Errorf("master exporter batching defaults not filled: %+v", e)
	}
	tl := tc.TserverLogs
	if tl == nil || tl.MinLevel == nil || *tl.MinLevel != "WARNING" {
		t.Errorf("tserver min_level must default to WARNING, got %+v", tl)
	}
	if len(tl.Exporters) != 1 || tl.Exporters[0].SendBatchMaxSize == nil ||
		*tl.Exporters[0].SendBatchMaxSize != 500 {
		t.Errorf("tserver exporter override lost: %+v", tl.Exporters)
	}
	cl := tc.ControllerLogs
	if cl == nil || len(cl.Exporters) != 1 || cl.Exporters[0].ExporterUuid != "exp-2" {
		t.Errorf("controller_logs spec = %+v", cl)
	}
	if tc.YsqlConnMgrLogs != nil || tc.NodeAgentLogs != nil || tc.YnpLogs != nil {
		t.Errorf("undeclared server-log pipelines must stay nil: %+v", tc)
	}

	// Round trip through flatten + state, then rebuild and compare.
	if err := d.Set("master_logs", flattenMasterLogsSpec(ml)); err != nil {
		t.Fatalf("set master_logs: %v", err)
	}
	if err := d.Set("tserver_logs", flattenTserverLogsSpec(tl)); err != nil {
		t.Fatalf("set tserver_logs: %v", err)
	}
	if err := d.Set("controller_logs", flattenControllerLogsSpec(cl)); err != nil {
		t.Fatalf("set controller_logs: %v", err)
	}
	rt := buildExportTelemetryConfigSpec(d).TelemetryConfig
	if rt.MasterLogs == nil || *rt.MasterLogs.MinLevel != "ERROR" ||
		*rt.MasterLogs.NoiseSampleDropRatio != 0.25 {
		t.Errorf("master_logs round trip = %+v", rt.MasterLogs)
	}
	if a := rt.MasterLogs.Exporters; len(a) != 1 || a[0].AdditionalTags == nil ||
		(*a[0].AdditionalTags)["env"] != "prod" {
		t.Errorf("master exporter tags round trip = %+v", a)
	}
	if rt.TserverLogs == nil || *rt.TserverLogs.Exporters[0].SendBatchMaxSize != 500 {
		t.Errorf("tserver_logs round trip = %+v", rt.TserverLogs)
	}
	if rt.ControllerLogs == nil || len(rt.ControllerLogs.Exporters) != 1 {
		t.Errorf("controller_logs round trip = %+v", rt.ControllerLogs)
	}
}

// noise_sample_drop_ratio = 0.0 means "keep every line" and must reach the API
// as an explicit 0 — a drop-zero-values helper would silently fall back to the
// server default of 0.99.
func TestMasterLogsNoiseRatioZeroIsSent(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"master_logs": []interface{}{map[string]interface{}{
			"noise_sample_drop_ratio": 0.0,
			"exporter": []interface{}{map[string]interface{}{
				"exporter_uuid": "exp-1",
			}},
		}},
	})
	ml := buildExportTelemetryConfigSpec(d).TelemetryConfig.MasterLogs
	if ml == nil || ml.NoiseSampleDropRatio == nil {
		t.Fatalf("noise_sample_drop_ratio must be sent explicitly, got %+v", ml)
	}
	if *ml.NoiseSampleDropRatio != 0.0 {
		t.Errorf("noise_sample_drop_ratio = %v want 0.0", *ml.NoiseSampleDropRatio)
	}
}

// TestAuditLogDefaultsReachTheServer pins that the audit-log fields the
// regenerated client made optional pointers still go on the wire when they
// hold the schema default. YBA's bean validation rejects a request without
// logParameterMaxSize ("must not be null"); building the pointer with
// utils.GetInt32Pointer(0) returns nil and drops the field, which is what broke
// TestAccLong_UniverseTelemetryConfig_GCP.
func TestAuditLogDefaultsReachTheServer(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"audit_logs": []interface{}{map[string]interface{}{
			// Same shape as the long acceptance test: log_parameter_max_size
			// is not set, so the schema default 0 applies.
			"ysql_audit_config": []interface{}{map[string]interface{}{
				"classes":   []interface{}{"READ", "WRITE", "DDL"},
				"log_level": "LOG",
			}},
			"ycql_audit_config": []interface{}{map[string]interface{}{}},
			"exporter": []interface{}{map[string]interface{}{
				"exporter_uuid": "exp-1",
			}},
		}},
	})
	a := buildExportTelemetryConfigSpec(d).TelemetryConfig.AuditLogs
	if a == nil || a.YsqlAuditConfig == nil || a.YcqlAuditConfig == nil {
		t.Fatalf("audit sub-configs missing: %+v", a)
	}
	ysql, ycql := a.YsqlAuditConfig, a.YcqlAuditConfig
	if ysql.LogParameterMaxSize == nil || *ysql.LogParameterMaxSize != 0 {
		t.Errorf("ysql log_parameter_max_size = %v, want pointer to 0", ysql.LogParameterMaxSize)
	}
	if ysql.LogLevel == nil || *ysql.LogLevel != "LOG" {
		t.Errorf("ysql log_level = %v, want pointer to LOG", ysql.LogLevel)
	}
	if ycql.LogLevel == nil || *ycql.LogLevel != "WARNING" {
		t.Errorf("ycql log_level = %v, want pointer to schema default WARNING", ycql.LogLevel)
	}
	// The pointers only matter for what the client serializes: omitempty drops
	// a nil pointer, so check the request body itself.
	body, err := json.Marshal(ysql)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"log_parameter_max_size":0`, `"log_level":"LOG"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("ysql audit body %s lacks %s", body, want)
		}
	}
}
