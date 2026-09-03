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
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestBuildExportTelemetryConfigSpec(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "abc-uuid",
		"audit_logs": []interface{}{
			map[string]interface{}{
				"ysql_audit_config": []interface{}{
					map[string]interface{}{
						"classes":                []interface{}{"READ", "WRITE"},
						"log_catalog":            true,
						"log_client":             true,
						"log_level":              "WARNING",
						"log_parameter":          true,
						"log_parameter_max_size": 4096,
						"log_relation":           true,
						"log_rows":               true,
						"log_statement":          true,
						"log_statement_once":     true,
					},
				},
				"exporter": []interface{}{
					map[string]interface{}{
						"exporter_uuid": "exp-1",
						"additional_tags": map[string]interface{}{
							"env": "prod",
						},
					},
				},
			},
		},
		"metrics": []interface{}{
			map[string]interface{}{
				"scrape_interval_seconds": 30,
				"scrape_timeout_seconds":  20,
				"collection_level":        "NORMAL",
				"scrape_config_targets":   []interface{}{"MASTER_EXPORT", "TSERVER_EXPORT"},
				"exporter": []interface{}{
					map[string]interface{}{
						"exporter_uuid":              "exp-1",
						"send_batch_size":            100,
						"send_batch_max_size":        1000,
						"send_batch_timeout_seconds": 60,
						"memory_limit_mib":           2048,
						"metrics_prefix":             "ybdb.",
					},
				},
			},
		},
	})

	spec := buildExportTelemetryConfigSpec(d)
	payload, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	var out struct {
		TelemetryConfig struct {
			AuditLogs *struct {
				YsqlAuditConfig map[string]interface{}   `json:"ysql_audit_config"`
				Exporters       []map[string]interface{} `json:"exporters"`
			} `json:"audit_logs"`
			Metrics *struct {
				ScrapeIntervalSeconds int                      `json:"scrape_interval_seconds"`
				CollectionLevel       string                   `json:"collection_level"`
				ScrapeConfigTargets   []string                 `json:"scrape_config_targets"`
				Exporters             []map[string]interface{} `json:"exporters"`
			} `json:"metrics"`
		} `json:"telemetry_config"`
		UpgradeOptions map[string]interface{} `json:"upgrade_options"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal payload: %v\n%s", err, payload)
	}
	if out.TelemetryConfig.AuditLogs == nil {
		t.Fatalf("audit_logs missing from payload: %s", payload)
	}
	if got := out.TelemetryConfig.AuditLogs.YsqlAuditConfig["log_level"]; got != "WARNING" {
		t.Errorf("ysql log_level: got %v want WARNING", got)
	}
	if len(out.TelemetryConfig.AuditLogs.Exporters) != 1 {
		t.Errorf(
			"expected exactly 1 audit exporter, got %d",
			len(out.TelemetryConfig.AuditLogs.Exporters),
		)
	}
	if out.TelemetryConfig.Metrics == nil {
		t.Fatalf("metrics missing from payload: %s", payload)
	}
	if out.TelemetryConfig.Metrics.CollectionLevel != "NORMAL" {
		t.Errorf("metrics collection_level: got %q want NORMAL",
			out.TelemetryConfig.Metrics.CollectionLevel)
	}
	if len(out.TelemetryConfig.Metrics.ScrapeConfigTargets) != 2 {
		t.Errorf("expected 2 scrape targets, got %d",
			len(out.TelemetryConfig.Metrics.ScrapeConfigTargets))
	}
	if len(out.TelemetryConfig.Metrics.Exporters) != 1 {
		t.Errorf("expected exactly 1 metrics exporter, got %d",
			len(out.TelemetryConfig.Metrics.Exporters))
	}
	mexp := out.TelemetryConfig.Metrics.Exporters[0]
	if mexp["metrics_prefix"] != "ybdb." {
		t.Errorf("metrics_prefix: got %v want ybdb.", mexp["metrics_prefix"])
	}
	if got, ok := out.UpgradeOptions["rolling_upgrade"].(bool); !ok || !got {
		t.Errorf(
			"upgrade_options.rolling_upgrade: got %v want true",
			out.UpgradeOptions["rolling_upgrade"],
		)
	}
}

func TestBuildDisableSpec(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "abc-uuid",
	})
	spec := buildDisableSpec(d)
	payload, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal disable spec: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, payload)
	}
	tc, ok := out["telemetry_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("telemetry_config not an object: %s", payload)
	}
	if len(tc) != 0 {
		t.Errorf("expected empty telemetry_config, got %v", tc)
	}
}

// TestFilterTelemetryConfig: the target's exporters are removed from every
// pipeline, an emptied pipeline is dropped, and untouched settings survive.
func TestFilterTelemetryConfig(t *testing.T) {
	keep := "keep-uuid"
	drop := "drop-uuid"
	tc := &clientv2.TelemetryConfig{
		AuditLogs: &clientv2.AuditLogsTelemetrySpec{
			YsqlAuditConfig: &clientv2.YSQLAuditConfig{
				Enabled:  true,
				LogLevel: utils.GetStringPointer("LOG"),
			},
			Exporters: []clientv2.UniverseLogsExporterConfig{
				{ExporterUuid: keep},
				{ExporterUuid: drop},
			},
		},
		QueryLogs: &clientv2.QueryLogsTelemetrySpec{
			Exporters: []clientv2.UniverseQueryLogsExporterConfig{
				{ExporterUuid: drop},
			},
		},
		Metrics: &clientv2.MetricsTelemetrySpec{
			Exporters: []clientv2.UniverseMetricsExporterConfig{
				{ExporterUuid: keep},
				{ExporterUuid: drop},
			},
			ScrapeConfigTargets: []clientv2.ScrapeConfigTargetType{"MASTER_EXPORT"},
		},
		MasterLogs: &clientv2.MasterLogsTelemetrySpec{
			Exporters: []clientv2.UniverseServerLogsExporterConfig{
				{ExporterUuid: keep},
				{ExporterUuid: drop},
			},
			MinLevel:             utils.GetStringPointer("ERROR"),
			NoiseSampleDropRatio: utils.GetFloat64Pointer(0.5),
		},
		TserverLogs: &clientv2.TServerLogsTelemetrySpec{
			Exporters: []clientv2.UniverseServerLogsExporterConfig{
				{ExporterUuid: drop},
			},
		},
	}

	out, changed := filterTelemetryConfig(tc, drop)
	if !changed {
		t.Fatal("filter must report a change when the target is referenced")
	}
	a := out.AuditLogs
	if a == nil || len(a.Exporters) != 1 || a.Exporters[0].ExporterUuid != keep {
		t.Errorf("audit exporters after filter = %+v; want [%s]", a, keep)
	}
	if a != nil && (a.YsqlAuditConfig == nil || !a.YsqlAuditConfig.Enabled) {
		t.Error("ysql audit sub-config must be preserved for the surviving exporter")
	}
	if out.QueryLogs != nil {
		t.Error("query_logs must be dropped when its only exporter is the target")
	}
	m := out.Metrics
	if m == nil || len(m.Exporters) != 1 || m.Exporters[0].ExporterUuid != keep {
		t.Errorf("metrics exporters after filter = %+v; want [%s]", m, keep)
	}
	if m != nil && len(m.ScrapeConfigTargets) != 1 {
		t.Errorf("metrics scrape targets not preserved: %+v", m)
	}
	ml := out.MasterLogs
	if ml == nil || len(ml.Exporters) != 1 || ml.Exporters[0].ExporterUuid != keep {
		t.Errorf("master_logs exporters after filter = %+v; want [%s]", ml, keep)
	}
	if ml != nil && (ml.MinLevel == nil || *ml.MinLevel != "ERROR" ||
		ml.NoiseSampleDropRatio == nil || *ml.NoiseSampleDropRatio != 0.5) {
		t.Errorf("master_logs settings not preserved: %+v", ml)
	}
	if out.TserverLogs != nil {
		t.Error("tserver_logs must be dropped when its only exporter is the target")
	}
}

// TestFilterTelemetryConfigDetection: a reference in any single pipeline —
// including the server-log ones — must be detected, and an unrelated config
// must be reported unchanged.
func TestFilterTelemetryConfigDetection(t *testing.T) {
	target := "target-uuid"
	server := func(uuid string) []clientv2.UniverseServerLogsExporterConfig {
		return []clientv2.UniverseServerLogsExporterConfig{{ExporterUuid: uuid}}
	}
	mk := func(mutate func(*clientv2.TelemetryConfig)) *clientv2.TelemetryConfig {
		tc := &clientv2.TelemetryConfig{
			AuditLogs: &clientv2.AuditLogsTelemetrySpec{
				Exporters: []clientv2.UniverseLogsExporterConfig{
					{ExporterUuid: "other"},
				},
			},
		}
		mutate(tc)
		return tc
	}
	cases := []struct {
		name string
		tc   *clientv2.TelemetryConfig
		want bool
	}{
		{"nil", nil, false},
		{"none", mk(func(*clientv2.TelemetryConfig) {}), false},
		{"audit", mk(func(tc *clientv2.TelemetryConfig) {
			tc.AuditLogs.Exporters = append(tc.AuditLogs.Exporters,
				clientv2.UniverseLogsExporterConfig{ExporterUuid: target})
		}), true},
		{"query", mk(func(tc *clientv2.TelemetryConfig) {
			tc.QueryLogs = &clientv2.QueryLogsTelemetrySpec{
				Exporters: []clientv2.UniverseQueryLogsExporterConfig{
					{ExporterUuid: target},
				},
			}
		}), true},
		{"metrics", mk(func(tc *clientv2.TelemetryConfig) {
			tc.Metrics = &clientv2.MetricsTelemetrySpec{
				Exporters: []clientv2.UniverseMetricsExporterConfig{
					{ExporterUuid: target},
				},
			}
		}), true},
		{"master_logs", mk(func(tc *clientv2.TelemetryConfig) {
			tc.MasterLogs = &clientv2.MasterLogsTelemetrySpec{Exporters: server(target)}
		}), true},
		{"tserver_logs", mk(func(tc *clientv2.TelemetryConfig) {
			tc.TserverLogs = &clientv2.TServerLogsTelemetrySpec{Exporters: server(target)}
		}), true},
		{"ysql_conn_mgr_logs", mk(func(tc *clientv2.TelemetryConfig) {
			tc.YsqlConnMgrLogs = &clientv2.YsqlConnMgrLogsTelemetrySpec{
				Exporters: server(target),
			}
		}), true},
		{"node_agent_logs", mk(func(tc *clientv2.TelemetryConfig) {
			tc.NodeAgentLogs = &clientv2.NodeAgentLogsTelemetrySpec{
				Exporters: server(target),
			}
		}), true},
		{"ynp_logs", mk(func(tc *clientv2.TelemetryConfig) {
			tc.YnpLogs = &clientv2.YnpLogsTelemetrySpec{Exporters: server(target)}
		}), true},
		{"controller_logs", mk(func(tc *clientv2.TelemetryConfig) {
			tc.ControllerLogs = &clientv2.ControllerLogsTelemetrySpec{
				Exporters: server(target),
			}
		}), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed := filterTelemetryConfig(c.tc, target)
			if changed != c.want {
				t.Errorf("filterTelemetryConfig changed = %v want %v", changed, c.want)
			}
			if !c.want && c.tc != nil && out.AuditLogs == nil {
				t.Error("an unreferenced pipeline must be carried over verbatim")
			}
		})
	}
}

func TestResourceUniverseTelemetryConfigSchema(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()

	uu, ok := res.Schema["universe_uuid"]
	if !ok || !uu.Required || !uu.ForceNew {
		t.Errorf("universe_uuid must be Required+ForceNew, got %+v", uu)
	}

	if res.Timeouts == nil {
		t.Fatal("Timeouts must be set on yba_universe_telemetry_config")
	}
	for name, got := range map[string]*time.Duration{
		"Create": res.Timeouts.Create,
		"Update": res.Timeouts.Update,
		"Delete": res.Timeouts.Delete,
	} {
		if got == nil {
			t.Errorf("%s timeout must be set", name)
			continue
		}
		if *got != telemetryUpgradeTimeout {
			t.Errorf("%s timeout = %s want %s",
				name, *got, telemetryUpgradeTimeout)
		}
	}
}

func TestBuildUpgradeOptionsDefault(t *testing.T) {
	out := buildUpgradeOptions(nil)
	if out.RollingUpgrade == nil || !*out.RollingUpgrade {
		t.Errorf("RollingUpgrade must default to true, got %v",
			out.RollingUpgrade)
	}
	if out.SleepAfterMasterRestartMillis != nil {
		t.Errorf("SleepAfterMasterRestartMillis must be nil when block "+
			"absent, got %d", *out.SleepAfterMasterRestartMillis)
	}
	if out.SleepAfterTserverRestartMillis != nil {
		t.Errorf("SleepAfterTserverRestartMillis must be nil when block "+
			"absent, got %d", *out.SleepAfterTserverRestartMillis)
	}
}

// Unconfigured pipelines must stay nil (disabled), not empty structs — empty
// structs would trip YBA's enum validation on missing required fields.
func TestBuildExportTelemetryConfigSpecMetricsOnly(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"metrics": []interface{}{
			map[string]interface{}{
				"exporter": []interface{}{
					map[string]interface{}{"exporter_uuid": "exp-1"},
				},
			},
		},
	})
	spec := buildExportTelemetryConfigSpec(d)
	if spec.TelemetryConfig == nil {
		t.Fatal("telemetry_config must be set")
	}
	if spec.TelemetryConfig.AuditLogs != nil {
		t.Error("audit_logs must be nil when not configured")
	}
	if spec.TelemetryConfig.QueryLogs != nil {
		t.Error("query_logs must be nil when not configured")
	}
	if spec.TelemetryConfig.Metrics == nil {
		t.Fatal("metrics must be set when configured")
	}
	if got := spec.TelemetryConfig.Metrics.Exporters; len(got) != 1 ||
		got[0].ExporterUuid != "exp-1" {
		t.Errorf("metrics exporters = %+v want [exp-1]", got)
	}
}
