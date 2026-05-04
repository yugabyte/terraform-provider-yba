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
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	client "github.com/yugabyte/platform-go-client"
	clientv2 "github.com/yugabyte/platform-go-client/v2"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// universeRef is a lightweight summary of a universe whose telemetry
// configuration references a given telemetry provider UUID.
type universeRef struct {
	UUID string
	Name string
}

// universeReferencesProvider returns true when any exporter in the
// universe's audit-log / query-log / metrics-export config matches the
// supplied provider UUID.
func universeReferencesProvider(u *client.UniverseResp, providerUUID string) bool {
	if u == nil {
		return false
	}
	details := u.GetUniverseDetails()
	for _, cluster := range details.Clusters {
		intent := cluster.UserIntent
		if a := intent.AuditLogConfig; a != nil {
			for _, e := range a.UniverseLogsExporterConfig {
				if e.ExporterUuid == providerUUID {
					return true
				}
			}
		}
		if q := intent.QueryLogConfig; q != nil {
			for _, e := range q.UniverseLogsExporterConfig {
				if e.ExporterUuid == providerUUID {
					return true
				}
			}
		}
		if m := intent.MetricsExportConfig; m != nil {
			for _, e := range m.UniverseMetricsExporterConfig {
				if e.ExporterUuid == providerUUID {
					return true
				}
			}
		}
	}
	return false
}

// detachTelemetryProviderFromUniverses rewrites the telemetry configuration
// of every universe that currently references `providerUUID`, submitting a
// new unified `export-telemetry-configs` spec with the provider filtered
// out of every exporter list. It blocks until every universe upgrade task
// reaches a terminal state.
//
// Invoked unconditionally during `yba_telemetry_provider` delete (before
// the YBA delete is attempted) so that the destroy step in a
// destroy-and-recreate plan does not race YBA's "as it is in use" check.
// When no universe references the provider the function is a no-op and
// returns an empty slice. The universes themselves are never destroyed —
// only their telemetry configuration is updated. Callers should log the
// list of affected universes prominently so the detach is auditable.
func detachTelemetryProviderFromUniverses(
	ctx context.Context, apiClient *api.APIClient, providerUUID string,
	timeout time.Duration,
) ([]universeRef, error) {
	universes, response, err := apiClient.YugawareClient.UniverseManagementAPI.
		ListUniverses(ctx, apiClient.CustomerID).Execute()
	if err != nil {
		return nil, utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Universe", "List")
	}

	var detached []universeRef
	for i := range universes {
		u := universes[i]
		if !universeReferencesProvider(&u, providerUUID) {
			continue
		}
		ref := universeRef{UUID: u.GetUniverseUUID(), Name: u.GetName()}
		tflog.Info(ctx, fmt.Sprintf(
			"Detaching telemetry provider %s from universe %s (%s)",
			providerUUID, ref.Name, ref.UUID))

		spec := buildDetachSpec(&u, providerUUID)

		// Wrap the dispatch in RetryOnUniverseTaskConflict so a YBA 409
		// "Task ConfigureExportTelemetryConfig cannot be queued on
		// existing task ConfigureExportTelemetryConfig" — typically left
		// behind by a previous interrupted apply that issued a rolling
		// upgrade still in flight — backs off and retries instead of
		// failing the destroy. Pre-existing telemetry tasks complete in
		// minutes; the retry helper keeps polling until our timeout
		// budget is exhausted.
		var (
			taskUUID string
			lastResp *http.Response
		)
		_, retryErr := utils.RetryOnUniverseTaskConflict(
			ctx,
			fmt.Sprintf("Detach telemetry provider %s from %s",
				providerUUID, ref.Name),
			timeout,
			func() (*http.Response, error) {
				task, resp, err := apiClient.YugawareClientV2.UniverseAPI.
					ConfigureExportTelemetryConfig(
						ctx, apiClient.CustomerID, ref.UUID).
					ExportTelemetryConfigSpec(spec).Execute()
				lastResp = resp
				if err != nil {
					return resp, err
				}
				if task != nil && task.TaskUuid != nil {
					taskUUID = *task.TaskUuid
				}
				return resp, nil
			},
		)
		if retryErr != nil {
			return detached, utils.ErrorFromHTTPResponse(lastResp, retryErr,
				utils.ResourceEntity,
				fmt.Sprintf("Universe Telemetry Config (%s)", ref.Name),
				"Detach")
		}
		if taskUUID != "" {
			if err := utils.WaitForTask(ctx, taskUUID, apiClient.CustomerID,
				apiClient.YugawareClient, timeout); err != nil {
				return detached, fmt.Errorf(
					"wait for detach task on universe %s (%s): %w",
					ref.Name, ref.UUID, err)
			}
		}
		detached = append(detached, ref)
	}
	return detached, nil
}

// buildDetachSpec rebuilds a universe's current telemetry configuration as
// a v2 ExportTelemetryConfigSpec, omitting any exporter entry whose
// `exporter_uuid` matches `providerUUID`. A section (audit_logs /
// query_logs / metrics) that has no other content is dropped entirely so
// the unified endpoint disables it — matching YBA's "empty or missing
// section == disable" semantics.
func buildDetachSpec(u *client.UniverseResp, providerUUID string) clientv2.ExportTelemetryConfigSpec {
	tc := clientv2.TelemetryConfig{}
	details := u.GetUniverseDetails()
	if len(details.Clusters) > 0 {
		intent := details.Clusters[0].UserIntent
		if a := intent.AuditLogConfig; a != nil {
			if spec := auditConfigToSpec(a, providerUUID); spec != nil {
				tc.AuditLogs = spec
			}
		}
		if q := intent.QueryLogConfig; q != nil {
			if spec := queryConfigToSpec(q, providerUUID); spec != nil {
				tc.QueryLogs = spec
			}
		}
		if m := intent.MetricsExportConfig; m != nil {
			if spec := metricsConfigToSpec(m, providerUUID); spec != nil {
				tc.Metrics = spec
			}
		}
	}
	// Drive the detach as a rolling upgrade and let YBA pick its own
	// per-restart sleep defaults — there is no user-facing
	// `upgrade_options` block on `yba_telemetry_provider`, so we
	// deliberately do not hard-code a sleep here.
	upgrade := clientv2.ExportTelemetryUpgradeOptions{
		RollingUpgrade: utils.GetBoolPointer(true),
	}
	return clientv2.ExportTelemetryConfigSpec{
		TelemetryConfig: &tc,
		UpgradeOptions:  &upgrade,
	}
}

// auditConfigToSpec converts an in-flight v1 AuditLogConfig into a v2
// AuditLogsTelemetrySpec, filtering out any exporter that references
// `skipUUID`. Returns nil when there are no remaining exporters and no
// enabled YSQL/YCQL audit config (so the caller can drop the section and
// disable audit log export entirely).
func auditConfigToSpec(a *client.AuditLogConfig, skipUUID string) *clientv2.AuditLogsTelemetrySpec {
	exporters := make([]clientv2.TelemetryExporterEntry, 0, len(a.UniverseLogsExporterConfig))
	for _, e := range a.UniverseLogsExporterConfig {
		if e.ExporterUuid == skipUUID {
			continue
		}
		entry := clientv2.TelemetryExporterEntry{ExporterUuid: e.ExporterUuid}
		if len(e.AdditionalTags) > 0 {
			tags := map[string]string{}
			for k, v := range e.AdditionalTags {
				tags[k] = v
			}
			entry.AdditionalTags = &tags
		}
		exporters = append(exporters, entry)
	}
	if len(exporters) == 0 {
		return nil
	}
	out := &clientv2.AuditLogsTelemetrySpec{Exporters: exporters}
	if y := a.YsqlAuditConfig; y != nil {
		out.YsqlAuditConfig = &clientv2.YSQLAuditConfig{
			Enabled:             y.Enabled,
			Classes:             append([]string{}, y.Classes...),
			LogCatalog:          y.LogCatalog,
			LogClient:           y.LogClient,
			LogLevel:            y.LogLevel,
			LogParameter:        y.LogParameter,
			LogParameterMaxSize: y.LogParameterMaxSize,
			LogRelation:         y.LogRelation,
			LogRows:             y.LogRows,
			LogStatement:        y.LogStatement,
			LogStatementOnce:    y.LogStatementOnce,
		}
	}
	if y := a.YcqlAuditConfig; y != nil {
		out.YcqlAuditConfig = &clientv2.YCQLAuditConfig{
			Enabled:            y.Enabled,
			LogLevel:           y.LogLevel,
			IncludedCategories: append([]string{}, y.IncludedCategories...),
			ExcludedCategories: append([]string{}, y.ExcludedCategories...),
			IncludedKeyspaces:  append([]string{}, y.IncludedKeyspaces...),
			ExcludedKeyspaces:  append([]string{}, y.ExcludedKeyspaces...),
			IncludedUsers:      append([]string{}, y.IncludedUsers...),
			ExcludedUsers:      append([]string{}, y.ExcludedUsers...),
		}
	}
	return out
}

// queryConfigToSpec mirrors auditConfigToSpec for QueryLogConfig,
// preserving per-exporter batching/memory fields.
func queryConfigToSpec(q *client.QueryLogConfig, skipUUID string) *clientv2.QueryLogsTelemetrySpec {
	exporters := make([]clientv2.TelemetryExporterEntry, 0, len(q.UniverseLogsExporterConfig))
	for _, e := range q.UniverseLogsExporterConfig {
		if e.ExporterUuid == skipUUID {
			continue
		}
		entry := clientv2.TelemetryExporterEntry{
			ExporterUuid:                    e.ExporterUuid,
			SendBatchMaxSize:                e.SendBatchMaxSize,
			SendBatchSize:                   e.SendBatchSize,
			SendBatchTimeoutSeconds:         e.SendBatchTimeoutSeconds,
			MemoryLimitMib:                  e.MemoryLimitMib,
			MemoryLimitCheckIntervalSeconds: e.MemoryLimitCheckIntervalSeconds,
		}
		if len(e.AdditionalTags) > 0 {
			tags := map[string]string{}
			for k, v := range e.AdditionalTags {
				tags[k] = v
			}
			entry.AdditionalTags = &tags
		}
		exporters = append(exporters, entry)
	}
	if len(exporters) == 0 {
		return nil
	}
	out := &clientv2.QueryLogsTelemetrySpec{Exporters: exporters}
	if y := q.YsqlQueryLogConfig; y != nil {
		out.YsqlQueryLogConfig = &clientv2.YSQLQueryLogConfig{
			Enabled:                 y.Enabled,
			LogStatement:            y.LogStatement,
			LogMinErrorStatement:    y.LogMinErrorStatement,
			LogErrorVerbosity:       y.LogErrorVerbosity,
			LogDuration:             y.LogDuration,
			DebugPrintPlan:          y.DebugPrintPlan,
			LogConnections:          y.LogConnections,
			LogDisconnections:       y.LogDisconnections,
			LogMinDurationStatement: y.LogMinDurationStatement,
		}
	}
	return out
}

// metricsConfigToSpec mirrors auditConfigToSpec for MetricsExportConfig,
// preserving scrape and per-exporter settings.
func metricsConfigToSpec(m *client.MetricsExportConfig, skipUUID string) *clientv2.MetricsTelemetrySpec {
	exporters := make([]clientv2.TelemetryExporterEntry, 0, len(m.UniverseMetricsExporterConfig))
	for _, e := range m.UniverseMetricsExporterConfig {
		if e.ExporterUuid == skipUUID {
			continue
		}
		entry := clientv2.TelemetryExporterEntry{
			ExporterUuid:                    e.ExporterUuid,
			SendBatchMaxSize:                e.SendBatchMaxSize,
			SendBatchSize:                   e.SendBatchSize,
			SendBatchTimeoutSeconds:         e.SendBatchTimeoutSeconds,
			MemoryLimitMib:                  e.MemoryLimitMib,
			MemoryLimitCheckIntervalSeconds: e.MemoryLimitCheckIntervalSeconds,
			MetricsPrefix:                   e.MetricsPrefix,
		}
		if len(e.AdditionalTags) > 0 {
			tags := map[string]string{}
			for k, v := range e.AdditionalTags {
				tags[k] = v
			}
			entry.AdditionalTags = &tags
		}
		exporters = append(exporters, entry)
	}
	if len(exporters) == 0 {
		return nil
	}
	out := &clientv2.MetricsTelemetrySpec{
		Exporters:             exporters,
		ScrapeIntervalSeconds: m.ScrapeIntervalSeconds,
		ScrapeTimeoutSeconds:  m.ScrapeTimeoutSeconds,
		CollectionLevel:       m.CollectionLevel,
	}
	for _, t := range m.ScrapeConfigTargets {
		out.ScrapeConfigTargets = append(out.ScrapeConfigTargets, clientv2.ScrapeConfigTargetType(t))
	}
	return out
}
