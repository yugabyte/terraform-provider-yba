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
	client "github.com/yugabyte/platform-go-client"
)

// flattenAuditLogConfig converts a YBA AuditLogConfig (camelCase) into the
// nested-map shape expected by Terraform's `audit_logs` schema.
func flattenAuditLogConfig(c *client.AuditLogConfig) []interface{} {
	if c == nil {
		return nil
	}
	out := map[string]interface{}{}
	if y := c.YsqlAuditConfig; y != nil {
		out["ysql_audit_config"] = []interface{}{flattenYsqlAuditConfig(y)}
	}
	if y := c.YcqlAuditConfig; y != nil {
		out["ycql_audit_config"] = []interface{}{flattenYcqlAuditConfig(y)}
	}
	if exporters := flattenLogExporters(c.UniverseLogsExporterConfig); len(exporters) > 0 {
		out["exporter"] = exporters
	}
	return []interface{}{out}
}

func flattenYsqlAuditConfig(y *client.YSQLAuditConfig) map[string]interface{} {
	return map[string]interface{}{
		"enabled":                y.Enabled,
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
	}
}

func flattenYcqlAuditConfig(y *client.YCQLAuditConfig) map[string]interface{} {
	return map[string]interface{}{
		"enabled":             y.Enabled,
		"log_level":           y.LogLevel,
		"included_categories": stringSliceToInterface(y.IncludedCategories),
		"excluded_categories": stringSliceToInterface(y.ExcludedCategories),
		"included_keyspaces":  stringSliceToInterface(y.IncludedKeyspaces),
		"excluded_keyspaces":  stringSliceToInterface(y.ExcludedKeyspaces),
		"included_users":      stringSliceToInterface(y.IncludedUsers),
		"excluded_users":      stringSliceToInterface(y.ExcludedUsers),
	}
}

func flattenLogExporters(in []client.UniverseLogsExporterConfig) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, e := range in {
		out = append(out, map[string]interface{}{
			"exporter_uuid":   e.ExporterUuid,
			"additional_tags": stringStringMapToInterface(e.AdditionalTags),
		})
	}
	return out
}

func flattenQueryLogConfig(c *client.QueryLogConfig) []interface{} {
	if c == nil {
		return nil
	}
	out := map[string]interface{}{}
	if y := c.YsqlQueryLogConfig; y != nil {
		out["ysql_query_log_config"] = []interface{}{flattenYsqlQueryLogConfig(y)}
	}
	if exporters := flattenQueryLogExporters(c.UniverseLogsExporterConfig); len(exporters) > 0 {
		out["exporter"] = exporters
	}
	return []interface{}{out}
}

func flattenYsqlQueryLogConfig(y *client.YSQLQueryLogConfig) map[string]interface{} {
	return map[string]interface{}{
		"enabled":                    y.Enabled,
		"log_statement":              y.LogStatement,
		"log_min_error_statement":    y.LogMinErrorStatement,
		"log_error_verbosity":        y.LogErrorVerbosity,
		"log_duration":               y.LogDuration,
		"debug_print_plan":           y.DebugPrintPlan,
		"log_connections":            y.LogConnections,
		"log_disconnections":         y.LogDisconnections,
		"log_min_duration_statement": int(y.LogMinDurationStatement),
	}
}

func flattenQueryLogExporters(in []client.UniverseQueryLogsExporterConfig) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, e := range in {
		entry := map[string]interface{}{
			"exporter_uuid":   e.ExporterUuid,
			"additional_tags": stringStringMapToInterface(e.AdditionalTags),
		}
		if e.SendBatchMaxSize != nil {
			entry["send_batch_max_size"] = int(*e.SendBatchMaxSize)
		}
		if e.SendBatchSize != nil {
			entry["send_batch_size"] = int(*e.SendBatchSize)
		}
		if e.SendBatchTimeoutSeconds != nil {
			entry["send_batch_timeout_seconds"] = int(*e.SendBatchTimeoutSeconds)
		}
		if e.MemoryLimitMib != nil {
			entry["memory_limit_mib"] = int(*e.MemoryLimitMib)
		}
		if e.MemoryLimitCheckIntervalSeconds != nil {
			entry["memory_limit_check_interval_seconds"] = int(*e.MemoryLimitCheckIntervalSeconds)
		}
		out = append(out, entry)
	}
	return out
}

func flattenMetricsExportConfig(c *client.MetricsExportConfig) []interface{} {
	if c == nil {
		return nil
	}
	out := map[string]interface{}{}
	if c.ScrapeIntervalSeconds != nil {
		out["scrape_interval_seconds"] = int(*c.ScrapeIntervalSeconds)
	}
	if c.ScrapeTimeoutSeconds != nil {
		out["scrape_timeout_seconds"] = int(*c.ScrapeTimeoutSeconds)
	}
	if c.CollectionLevel != nil {
		out["collection_level"] = *c.CollectionLevel
	}
	if len(c.ScrapeConfigTargets) > 0 {
		out["scrape_config_targets"] = stringSliceToInterface(c.ScrapeConfigTargets)
	}
	if exporters := flattenMetricsExporters(c.UniverseMetricsExporterConfig); len(exporters) > 0 {
		out["exporter"] = exporters
	}
	return []interface{}{out}
}

func flattenMetricsExporters(in []client.UniverseMetricsExporterConfig) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, e := range in {
		entry := map[string]interface{}{
			"exporter_uuid":   e.ExporterUuid,
			"additional_tags": stringStringMapToInterface(e.AdditionalTags),
		}
		if e.SendBatchMaxSize != nil {
			entry["send_batch_max_size"] = int(*e.SendBatchMaxSize)
		}
		if e.SendBatchSize != nil {
			entry["send_batch_size"] = int(*e.SendBatchSize)
		}
		if e.SendBatchTimeoutSeconds != nil {
			entry["send_batch_timeout_seconds"] = int(*e.SendBatchTimeoutSeconds)
		}
		if e.MemoryLimitMib != nil {
			entry["memory_limit_mib"] = int(*e.MemoryLimitMib)
		}
		if e.MemoryLimitCheckIntervalSeconds != nil {
			entry["memory_limit_check_interval_seconds"] = int(*e.MemoryLimitCheckIntervalSeconds)
		}
		if e.MetricsPrefix != nil {
			entry["metrics_prefix"] = *e.MetricsPrefix
		}
		out = append(out, entry)
	}
	return out
}

func stringSliceToInterface(in []string) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

func stringStringMapToInterface(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
