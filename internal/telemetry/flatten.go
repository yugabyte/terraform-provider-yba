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
	clientv2 "github.com/yugabyte/platform-go-client/v2"
)

// flatten helpers convert the v2 TelemetryConfig from GetExportTelemetryConfig
// back into the nested-map shapes Terraform's schema expects — the mirror of the
// build* helpers in resource_universe_telemetry_config.go (same v2 types, so
// read/write stay in lock-step). Each returns nil for a nil section so the caller
// clears the block, surfacing an "export disabled" change as drift.

func flattenAuditLogsSpec(a *clientv2.AuditLogsTelemetrySpec) []interface{} {
	if a == nil {
		return nil
	}
	out := map[string]interface{}{}
	if y := a.YsqlAuditConfig; y != nil {
		// `enabled` not surfaced: readOnly + derived from block presence, so
		// exposing it means a perpetual diff. See buildYsqlAuditConfig.
		out["ysql_audit_config"] = []interface{}{map[string]interface{}{
			"classes":                stringSliceToInterface(y.Classes),
			"log_catalog":            y.LogCatalog,
			"log_client":             y.LogClient,
			"log_level":              y.LogLevel,
			"log_parameter":          y.LogParameter,
			"log_parameter_max_size": int(y.LogParameterMaxSize),
			"log_relation":           y.LogRelation,
			"log_rows":               y.LogRows,
			"log_statement":          y.LogStatement,
			"log_statement_once":     y.LogStatementOnce,
		}}
	}
	if y := a.YcqlAuditConfig; y != nil {
		out["ycql_audit_config"] = []interface{}{map[string]interface{}{
			"log_level":           y.LogLevel,
			"included_categories": stringSliceToInterface(y.IncludedCategories),
			"excluded_categories": stringSliceToInterface(y.ExcludedCategories),
			"included_keyspaces":  stringSliceToInterface(y.IncludedKeyspaces),
			"excluded_keyspaces":  stringSliceToInterface(y.ExcludedKeyspaces),
			"included_users":      stringSliceToInterface(y.IncludedUsers),
			"excluded_users":      stringSliceToInterface(y.ExcludedUsers),
		}}
	}
	exporters := make([]interface{}, 0, len(a.Exporters))
	for _, e := range a.Exporters {
		exporters = append(exporters, map[string]interface{}{
			"exporter_uuid":   e.ExporterUuid,
			"additional_tags": tagsToInterface(e.AdditionalTags),
		})
	}
	if len(exporters) > 0 {
		out["exporter"] = exporters
	}
	return []interface{}{out}
}

func flattenQueryLogsSpec(q *clientv2.QueryLogsTelemetrySpec) []interface{} {
	if q == nil {
		return nil
	}
	out := map[string]interface{}{}
	if y := q.YsqlQueryLogConfig; y != nil {
		out["ysql_query_log_config"] = []interface{}{map[string]interface{}{
			"log_statement":              y.LogStatement,
			"log_min_error_statement":    y.LogMinErrorStatement,
			"log_error_verbosity":        y.LogErrorVerbosity,
			"log_duration":               y.LogDuration,
			"debug_print_plan":           y.DebugPrintPlan,
			"log_connections":            y.LogConnections,
			"log_disconnections":         y.LogDisconnections,
			"log_min_duration_statement": int(y.LogMinDurationStatement),
		}}
	}
	exporters := make([]interface{}, 0, len(q.Exporters))
	for _, e := range q.Exporters {
		entry := map[string]interface{}{
			"exporter_uuid":   e.ExporterUuid,
			"additional_tags": tagsToInterface(e.AdditionalTags),
		}
		addBatchingFields(entry, e.SendBatchMaxSize, e.SendBatchSize,
			e.SendBatchTimeoutSeconds, e.MemoryLimitMib, e.MemoryLimitCheckIntervalSeconds)
		exporters = append(exporters, entry)
	}
	if len(exporters) > 0 {
		out["exporter"] = exporters
	}
	return []interface{}{out}
}

func flattenMetricsSpec(m *clientv2.MetricsTelemetrySpec) []interface{} {
	if m == nil {
		return nil
	}
	out := map[string]interface{}{}
	if m.ScrapeIntervalSeconds != nil {
		out["scrape_interval_seconds"] = int(*m.ScrapeIntervalSeconds)
	}
	if m.ScrapeTimeoutSeconds != nil {
		out["scrape_timeout_seconds"] = int(*m.ScrapeTimeoutSeconds)
	}
	if m.CollectionLevel != nil {
		out["collection_level"] = *m.CollectionLevel
	}
	if len(m.ScrapeConfigTargets) > 0 {
		targets := make([]interface{}, 0, len(m.ScrapeConfigTargets))
		for _, t := range m.ScrapeConfigTargets {
			targets = append(targets, string(t))
		}
		out["scrape_config_targets"] = targets
	}
	exporters := make([]interface{}, 0, len(m.Exporters))
	for _, e := range m.Exporters {
		entry := map[string]interface{}{
			"exporter_uuid":   e.ExporterUuid,
			"additional_tags": tagsToInterface(e.AdditionalTags),
		}
		addBatchingFields(entry, e.SendBatchMaxSize, e.SendBatchSize,
			e.SendBatchTimeoutSeconds, e.MemoryLimitMib, e.MemoryLimitCheckIntervalSeconds)
		if e.MetricsPrefix != nil {
			entry["metrics_prefix"] = *e.MetricsPrefix
		}
		exporters = append(exporters, entry)
	}
	if len(exporters) > 0 {
		out["exporter"] = exporters
	}
	return []interface{}{out}
}

func addBatchingFields(
	entry map[string]interface{},
	sendBatchMaxSize, sendBatchSize, sendBatchTimeoutSeconds,
	memoryLimitMib, memoryLimitCheckIntervalSeconds *int32,
) {
	if sendBatchMaxSize != nil {
		entry["send_batch_max_size"] = int(*sendBatchMaxSize)
	}
	if sendBatchSize != nil {
		entry["send_batch_size"] = int(*sendBatchSize)
	}
	if sendBatchTimeoutSeconds != nil {
		entry["send_batch_timeout_seconds"] = int(*sendBatchTimeoutSeconds)
	}
	if memoryLimitMib != nil {
		entry["memory_limit_mib"] = int(*memoryLimitMib)
	}
	if memoryLimitCheckIntervalSeconds != nil {
		entry["memory_limit_check_interval_seconds"] = int(*memoryLimitCheckIntervalSeconds)
	}
}

func stringSliceToInterface(in []string) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

func tagsToInterface(in *map[string]string) map[string]interface{} {
	out := map[string]interface{}{}
	if in == nil {
		return out
	}
	for k, v := range *in {
		out[k] = v
	}
	return out
}
