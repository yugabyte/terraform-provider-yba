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

// Package telemetry provides the per-sink telemetry provider resources
// (yba_datadog_telemetry_provider, yba_otlp_telemetry_provider, ...) and the
// yba_universe_telemetry_config resource for managing YugabyteDB Anywhere
// observability exports.
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

type universeRef struct {
	UUID string
	Name string
}

const clusterTypePrimary = "PRIMARY"

// primaryUserIntent returns the primary cluster's UserIntent. Selected by
// clusterType, not index — YBA doesn't guarantee the primary is clusters[0].
func primaryUserIntent(u *client.UniverseResp) *client.UserIntent {
	if u == nil {
		return nil
	}
	details := u.GetUniverseDetails()
	if len(details.Clusters) == 0 {
		return nil
	}
	for i := range details.Clusters {
		if details.Clusters[i].ClusterType == clusterTypePrimary {
			ui := details.Clusters[i].UserIntent
			return &ui
		}
	}
	ui := details.Clusters[0].UserIntent
	return &ui
}

// universeReferencesProvider reports whether any exporter in the primary cluster's
// audit/query/metrics config uses providerUUID. Scoped to the primary cluster to
// match YBA's isProviderInUse (the "as it is in use" delete gate).
func universeReferencesProvider(u *client.UniverseResp, providerUUID string) bool {
	intent := primaryUserIntent(u)
	if intent == nil {
		return false
	}
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
	return false
}

// detachTelemetryProviderFromUniverses rewrites every universe that references
// providerUUID, resubmitting its telemetry spec with the provider filtered out,
// and blocks until each upgrade task finishes. Called before the YBA provider
// delete so a destroy-and-recreate doesn't race YBA's "as it is in use" check.
// No-op (empty slice) when nothing references it; universes are never destroyed.
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

		// Retry on a YBA 409 (a ConfigureExportTelemetryConfig task left in
		// flight by a prior interrupted apply) instead of failing the destroy;
		// such tasks finish in minutes, and the helper polls until the timeout
		// budget is spent.
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

// buildDetachSpec rebuilds a universe's telemetry config as a v2 spec, omitting
// any exporter matching providerUUID. A section with no remaining content is
// dropped so the endpoint disables it (YBA's "empty/missing section == disable").
func buildDetachSpec(
	u *client.UniverseResp,
	providerUUID string,
) clientv2.ExportTelemetryConfigSpec {
	tc := clientv2.TelemetryConfig{}
	if intent := primaryUserIntent(u); intent != nil {
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
	// Rolling upgrade with YBA's default sleeps — the telemetry provider
	// resources have no upgrade_options block, so we don't hard-code a sleep.
	upgrade := clientv2.ExportTelemetryUpgradeOptions{
		RollingUpgrade: utils.GetBoolPointer(true),
	}
	return clientv2.ExportTelemetryConfigSpec{
		TelemetryConfig: &tc,
		UpgradeOptions:  &upgrade,
	}
}

// auditConfigToSpec converts a v1 AuditLogConfig to a v2 spec, dropping exporters
// that reference skipUUID. Returns nil when no exporters remain, so the caller
// drops the section and disables audit export.
func auditConfigToSpec(a *client.AuditLogConfig, skipUUID string) *clientv2.AuditLogsTelemetrySpec {
	exporters := make([]clientv2.UniverseLogsExporterConfig, 0, len(a.UniverseLogsExporterConfig))
	for _, e := range a.UniverseLogsExporterConfig {
		if e.ExporterUuid == skipUUID {
			continue
		}
		entry := clientv2.UniverseLogsExporterConfig{ExporterUuid: e.ExporterUuid}
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

func queryConfigToSpec(q *client.QueryLogConfig, skipUUID string) *clientv2.QueryLogsTelemetrySpec {
	exporters := make(
		[]clientv2.UniverseQueryLogsExporterConfig,
		0,
		len(q.UniverseLogsExporterConfig),
	)
	for _, e := range q.UniverseLogsExporterConfig {
		if e.ExporterUuid == skipUUID {
			continue
		}
		entry := clientv2.UniverseQueryLogsExporterConfig{
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

func metricsConfigToSpec(
	m *client.MetricsExportConfig,
	skipUUID string,
) *clientv2.MetricsTelemetrySpec {
	exporters := make(
		[]clientv2.UniverseMetricsExporterConfig,
		0,
		len(m.UniverseMetricsExporterConfig),
	)
	for _, e := range m.UniverseMetricsExporterConfig {
		if e.ExporterUuid == skipUUID {
			continue
		}
		entry := clientv2.UniverseMetricsExporterConfig{
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
		out.ScrapeConfigTargets = append(
			out.ScrapeConfigTargets,
			clientv2.ScrapeConfigTargetType(t),
		)
	}
	return out
}
