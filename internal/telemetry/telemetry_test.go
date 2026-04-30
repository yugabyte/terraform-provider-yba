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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TestBuildExportTelemetryConfigSpec verifies that the unified
// export-telemetry-configs payload produced by the resource (via the v2 SDK
// types) marshals to the snake_case JSON shape documented by YBA.
func TestBuildExportTelemetryConfigSpec(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "abc-uuid",
		"audit_logs": []interface{}{
			map[string]interface{}{
				"ysql_audit_config": []interface{}{
					map[string]interface{}{
						"enabled":                true,
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
		t.Errorf("expected exactly 1 audit exporter, got %d", len(out.TelemetryConfig.AuditLogs.Exporters))
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
		t.Errorf("upgrade_options.rolling_upgrade: got %v want true", out.UpgradeOptions["rolling_upgrade"])
	}
}

// TestBuildDisableSpec verifies the empty `telemetry_config: {}` body used
// when deleting a `yba_universe_telemetry_config` resource.
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

// TestTelemetryProviderType ensures the polymorphic block selector returns
// the correct YBA ProviderType enum.
func TestTelemetryProviderType(t *testing.T) {
	cases := []struct {
		block string
		want  string
	}{
		{"data_dog", typeDataDog},
		{"otlp", typeOTLP},
		{"aws_cloud_watch", typeAWSCloudWatch},
		{"gcp_cloud_monitoring", typeGCPCloudMonitor},
		{"splunk", typeSplunk},
		{"loki", typeLoki},
		{"dynatrace", typeDynatrace},
	}
	for _, tc := range cases {
		t.Run(tc.block, func(t *testing.T) {
			res := ResourceTelemetryProvider()
			raw := map[string]interface{}{
				"name": "test",
				tc.block: []interface{}{
					map[string]interface{}{},
				},
			}
			d := schema.TestResourceDataRaw(t, res.Schema, raw)
			got, err := telemetryProviderType(d)
			if err != nil {
				t.Fatalf("type: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
