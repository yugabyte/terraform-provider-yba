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
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// TestBuildSpecMultipleExportersAllSections drives the headline "multiple
// exporters" case: every pipeline carries more than one exporter, and the
// same provider is shared across audit + query + metrics on a single
// universe. It asserts the exporter counts, the per-exporter batching fields
// (present on query/metrics, ABSENT on audit), and the metrics-only
// metrics_prefix.
func TestBuildSpecMultipleExportersAllSections(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"audit_logs": []interface{}{map[string]interface{}{
			"ysql_audit_config": []interface{}{map[string]interface{}{
				"enabled": true,
				"classes": []interface{}{"READ", "WRITE", "DDL"},
			}},
			"exporter": []interface{}{
				map[string]interface{}{"exporter_uuid": "shared"},
				map[string]interface{}{"exporter_uuid": "audit-only"},
			},
		}},
		"query_logs": []interface{}{map[string]interface{}{
			"ysql_query_log_config": []interface{}{map[string]interface{}{
				"enabled": true,
			}},
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

	// Audit exporters use the UniverseLogsExporterConfig type, which the v2
	// SDK models WITHOUT batching fields — so the audit pipeline structurally
	// cannot carry them (exporter_uuid + additional_tags only). The other two
	// pipelines use their own types that DO carry batching.
	for _, e := range tc.AuditLogs.Exporters {
		if e.ExporterUuid == "" {
			t.Errorf("audit exporter missing uuid: %+v", e)
		}
	}
	// Query/metrics exporters DO carry batching fields (schema defaults fill
	// them in even when the user omits them).
	for _, e := range tc.QueryLogs.Exporters {
		if e.SendBatchMaxSize == nil || e.MemoryLimitMib == nil {
			t.Errorf("query exporter %q missing batching fields: %+v",
				e.ExporterUuid, e)
		}
	}
	// metrics_prefix only set on the exporter that asked for it.
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

// sortedStrings returns a sorted copy so set contents can be compared
// independent of order.
func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// TestTypeSetIgnoresOrder is the regression test for commit 6f3df3a. YBA
// persists audit-log classes and metric scrape targets as Java Sets, so it
// can echo them back in any order. Modeled as a TypeSet, two different
// orderings must produce identical Sets — otherwise every refresh shows a
// phantom diff and Terraform proposes a no-op rolling restart forever.
func TestTypeSetIgnoresOrder(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()

	mk := func(classes, targets []interface{}) *schema.ResourceData {
		return schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"universe_uuid": "uni-1",
			"audit_logs": []interface{}{map[string]interface{}{
				"ysql_audit_config": []interface{}{map[string]interface{}{
					"enabled": true,
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

	// And the build path must extract the same membership from either.
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

// TestFlattenRoundTripNoPhantomDiff feeds a fully-populated v1 config (as YBA
// would return it on Read) through the flatten helpers into resource data and
// back out through the build helpers, asserting the values survive the round
// trip. A drift here is exactly what produces a perpetual phantom diff on
// `terraform plan` after a successful apply.
func TestFlattenRoundTripNoPhantomDiff(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()

	auditTags := map[string]string{"env": "prod"}
	audit := &clientv2.AuditLogsTelemetrySpec{
		YsqlAuditConfig: &clientv2.YSQLAuditConfig{
			Enabled:  true,
			Classes:  []string{"WRITE", "READ", "DDL"}, // server order differs
			LogLevel: "WARNING",
		},
		YcqlAuditConfig: &clientv2.YCQLAuditConfig{
			Enabled:            true,
			LogLevel:           "ERROR",
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
	if !ysql.Enabled || ysql.LogLevel != "WARNING" {
		t.Errorf("ysql audit lost in round trip: %+v", ysql)
	}
	if got := sortedStrings(ysql.Classes); !equalStrings(got, []string{"DDL", "READ", "WRITE"}) {
		t.Errorf("audit classes round trip = %v", got)
	}
	ycql := spec.TelemetryConfig.AuditLogs.YcqlAuditConfig
	if ycql == nil || ycql.LogLevel != "ERROR" {
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

// TestEmptySectionsDisabled verifies that a resource with no telemetry blocks
// at all produces a spec with all three sections nil (so the unified endpoint
// disables everything) rather than empty structs that would trip YBA's
// "export active but no exporter" validation.
func TestEmptySectionsDisabled(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema,
		map[string]interface{}{"universe_uuid": "uni-1"})
	spec := buildExportTelemetryConfigSpec(d)
	if spec.TelemetryConfig == nil {
		t.Fatal("telemetry_config must always be set (empty disables exporters)")
	}
	if spec.TelemetryConfig.AuditLogs != nil ||
		spec.TelemetryConfig.QueryLogs != nil ||
		spec.TelemetryConfig.Metrics != nil {
		t.Errorf("all sections must be nil when unconfigured: %+v", spec.TelemetryConfig)
	}
}
