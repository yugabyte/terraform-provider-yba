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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

type universeRef struct {
	UUID string
	Name string
}

// detachTelemetryProviderFromUniverses rewrites every universe whose telemetry
// config references providerUUID, resubmitting the config with that provider's
// exporters filtered out, and blocks until each upgrade task finishes. Called
// before the YBA provider delete so a destroy-and-recreate doesn't race YBA's
// "as it is in use" check. No-op (empty slice) when nothing references it;
// universes are never destroyed.
//
// Reference detection and the rewrite both go through the v2
// export-telemetry-configs GET, which covers every pipeline (audit, query,
// metrics, and the server-log pipelines) and reads the primary cluster's
// config — the same scope as YBA's isProviderInUse delete gate. The v1
// UserIntent no longer carries the telemetry sections.
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
		ref := universeRef{UUID: u.GetUniverseUUID(), Name: u.GetName()}

		config, err := getExportTelemetryConfig(
			ctx, apiClient, ref.UUID, "Detach - Get Config")
		if err != nil {
			if errors.Is(err, utils.ErrUniverseMissing) {
				// The universe disappeared between the list and this read.
				continue
			}
			return detached, err
		}

		filtered, changed := filterTelemetryConfig(config, providerUUID)
		if !changed {
			continue
		}
		tflog.Info(ctx, fmt.Sprintf(
			"Detaching telemetry provider %s from universe %s (%s)",
			providerUUID, ref.Name, ref.UUID))

		// Rolling upgrade with YBA's default sleeps — the telemetry provider
		// resources have no upgrade_options block, so we don't hard-code a sleep.
		spec := clientv2.ExportTelemetryConfigSpec{
			TelemetryConfig: &filtered,
			UpgradeOptions: &clientv2.ExportTelemetryUpgradeOptions{
				RollingUpgrade: utils.GetBoolPointer(true),
			},
		}

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

// filterExporters returns in minus the entries whose UUID (per the accessor)
// matches skip, and whether any entry was removed.
func filterExporters[T any](in []T, uuid func(T) string, skip string) ([]T, bool) {
	kept := make([]T, 0, len(in))
	for _, e := range in {
		if uuid(e) == skip {
			continue
		}
		kept = append(kept, e)
	}
	return kept, len(kept) != len(in)
}

// filterTelemetryConfig returns a copy of tc with every exporter referencing
// skipUUID removed, and reports whether anything was removed. A pipeline left
// with no exporters is dropped so the endpoint disables it (YBA's
// "empty/missing section == disable"); untouched pipelines are carried over
// verbatim so a detach never alters unrelated export config.
func filterTelemetryConfig(
	tc *clientv2.TelemetryConfig, skipUUID string,
) (clientv2.TelemetryConfig, bool) {
	out := clientv2.TelemetryConfig{}
	if tc == nil {
		return out, false
	}
	changed := false

	logsUUID := func(e clientv2.UniverseLogsExporterConfig) string { return e.ExporterUuid }
	queryUUID := func(e clientv2.UniverseQueryLogsExporterConfig) string { return e.ExporterUuid }
	metricsUUID := func(e clientv2.UniverseMetricsExporterConfig) string { return e.ExporterUuid }
	serverUUID := func(e clientv2.UniverseServerLogsExporterConfig) string { return e.ExporterUuid }

	if a := tc.AuditLogs; a != nil {
		kept, removed := filterExporters(a.Exporters, logsUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *a
			spec.Exporters = kept
			out.AuditLogs = &spec
		}
	}
	if q := tc.QueryLogs; q != nil {
		kept, removed := filterExporters(q.Exporters, queryUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *q
			spec.Exporters = kept
			out.QueryLogs = &spec
		}
	}
	if m := tc.Metrics; m != nil {
		kept, removed := filterExporters(m.Exporters, metricsUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *m
			spec.Exporters = kept
			out.Metrics = &spec
		}
	}
	if s := tc.MasterLogs; s != nil {
		kept, removed := filterExporters(s.Exporters, serverUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *s
			spec.Exporters = kept
			out.MasterLogs = &spec
		}
	}
	if s := tc.TserverLogs; s != nil {
		kept, removed := filterExporters(s.Exporters, serverUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *s
			spec.Exporters = kept
			out.TserverLogs = &spec
		}
	}
	if s := tc.YsqlConnMgrLogs; s != nil {
		kept, removed := filterExporters(s.Exporters, serverUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *s
			spec.Exporters = kept
			out.YsqlConnMgrLogs = &spec
		}
	}
	if s := tc.NodeAgentLogs; s != nil {
		kept, removed := filterExporters(s.Exporters, serverUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *s
			spec.Exporters = kept
			out.NodeAgentLogs = &spec
		}
	}
	if s := tc.YnpLogs; s != nil {
		kept, removed := filterExporters(s.Exporters, serverUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *s
			spec.Exporters = kept
			out.YnpLogs = &spec
		}
	}
	if s := tc.ControllerLogs; s != nil {
		kept, removed := filterExporters(s.Exporters, serverUUID, skipUUID)
		changed = changed || removed
		if len(kept) > 0 {
			spec := *s
			spec.Exporters = kept
			out.ControllerLogs = &spec
		}
	}
	return out, changed
}
