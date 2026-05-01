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
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	clientv2 "github.com/yugabyte/platform-go-client/v2"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Allowed values for scrape_config_targets, mirrored from
// `ScrapeConfigTargetType` in the YBA OpenAPI spec.
var allowedScrapeTargets = []string{
	"MASTER_EXPORT",
	"TSERVER_EXPORT",
	"YSQL_EXPORT",
	"CQL_EXPORT",
	"NODE_EXPORT",
	"NODE_AGENT_EXPORT",
	"OTEL_EXPORT",
}

// allowedCollectionLevels mirrors the enum used by metrics export
// configuration (`ALL`, `NORMAL`, `TABLE_OFF`, `MINIMAL`, `OFF`).
var allowedCollectionLevels = []string{"ALL", "NORMAL", "TABLE_OFF", "MINIMAL", "OFF"}

// ResourceUniverseTelemetryConfig configures audit log, query log, and
// metric export pipelines for a single YBA universe through the unified
// `export-telemetry-configs` API.
//
// The lifecycle maps to the YBA API as follows:
//
//   - Create / Update: POST /api/v2/customers/{c}/universes/{u}/export-telemetry-configs
//     with a `telemetry_config` body that contains all configured exporters.
//   - Read: GET   /api/v1/customers/{c}/universes/{u} and inspect
//     `universeDetails.clusters[0].userIntent.{auditLogConfig,
//     queryLogConfig, metricsExportConfig}` (synced by YBA after the task
//     completes).
//   - Delete: POST the same unified endpoint with `telemetry_config: {}` to
//     disable all exporters on the universe.
//
// All write operations queue a universe upgrade task on YBA; the resource
// blocks until the task reaches a terminal state via `utils.WaitForTask`.
func ResourceUniverseTelemetryConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Universe Telemetry Config Resource. Attaches audit log, query log, " +
			"and metrics export pipelines to a YBA universe via the unified " +
			"`export-telemetry-configs` API. Each exporter references a " +
			"`yba_telemetry_provider` (or any pre-existing telemetry provider " +
			"UUID) and triggers a rolling/non-rolling restart of the universe to " +
			"install or update the OpenTelemetry collector.\n\n" +
			"~> **Note:** OTLP-based exporters require the global runtime config " +
			"`yb.telemetry.allow_otlp` to be set to `true`. Manage that with the " +
			"`yba_runtime_config` resource.\n\n" +
			"~> **Note:** This resource does not currently support importing an " +
			"existing universe-level configuration; recreate the resource by " +
			"applying the desired state.\n\n" +
			"~> **Dependency Note:** When `exporter_uuid` is wired through a " +
			"reference like `yba_telemetry_provider.x.id`, Terraform's dependency " +
			"graph automatically orders create / replace / destroy of the provider " +
			"before this resource — there is **no need to add an explicit " +
			"`depends_on`**. The provider's own destroy step also proactively " +
			"detaches itself from every referencing universe before deletion, so " +
			"a plan that destroys-and-recreates a provider in the same apply is " +
			"safe.",

		CreateContext: resourceUniverseTelemetryConfigCreate,
		ReadContext:   resourceUniverseTelemetryConfigRead,
		UpdateContext: resourceUniverseTelemetryConfigUpdate,
		DeleteContext: resourceUniverseTelemetryConfigDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(telemetryUpgradeTimeout),
			Update: schema.DefaultTimeout(telemetryUpgradeTimeout),
			Delete: schema.DefaultTimeout(telemetryUpgradeTimeout),
			Read:   schema.DefaultTimeout(15 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the universe whose telemetry pipelines are managed.",
			},
			"audit_logs": auditLogsSchema(),
			"query_logs": queryLogsSchema(),
			"metrics":    metricsSchema(),
			"upgrade_options": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Description: "Optional rolling-restart options applied while reconfiguring " +
					"the universe.\n\n" +
					"~> **Performance Note:** The `sleep_after_*_restart_millis` defaults " +
					"of 180000 (3 minutes) are applied per node. A 9-node universe " +
					"therefore spends ~27 minutes just sleeping between restarts on top " +
					"of the actual restart work. Lower these values for faster reconfigures " +
					"on healthy clusters, or raise them for clusters under heavy traffic.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"rolling_upgrade": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							Description: "Perform a rolling restart (default true). " +
								"Set to false to restart all nodes at once.",
						},
						"sleep_after_master_restart_millis": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  180000,
							Description: "Sleep between master restarts (ms). Defaults to " +
								"180000 (3 minutes).",
						},
						"sleep_after_tserver_restart_millis": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  180000,
							Description: "Sleep between tserver restarts (ms). Defaults to " +
								"180000 (3 minutes).",
						},
					},
				},
			},
		},
	}
}

func auditLogsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Description: "Audit log export configuration. Omit to disable audit log export.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ysql_audit_config": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"enabled": {Type: schema.TypeBool, Optional: true, Default: false},
							"classes": {
								Type:        schema.TypeList,
								Optional:    true,
								Elem:        &schema.Schema{Type: schema.TypeString},
								Description: "YSQL audit log classes (e.g. READ, WRITE, DDL, ROLE).",
							},
							"log_catalog":            {Type: schema.TypeBool, Optional: true, Default: true},
							"log_client":             {Type: schema.TypeBool, Optional: true, Default: true},
							"log_level":              {Type: schema.TypeString, Optional: true, Default: "LOG"},
							"log_parameter":          {Type: schema.TypeBool, Optional: true, Default: false},
							"log_parameter_max_size": {Type: schema.TypeInt, Optional: true, Default: 0},
							"log_relation":           {Type: schema.TypeBool, Optional: true, Default: false},
							"log_rows":               {Type: schema.TypeBool, Optional: true, Default: false},
							"log_statement":          {Type: schema.TypeBool, Optional: true, Default: true},
							"log_statement_once":     {Type: schema.TypeBool, Optional: true, Default: false},
						},
					},
				},
				"ycql_audit_config": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"enabled":             {Type: schema.TypeBool, Optional: true, Default: false},
							"log_level":           {Type: schema.TypeString, Optional: true, Default: "WARNING"},
							"included_categories": {Type: schema.TypeList, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
							"excluded_categories": {Type: schema.TypeList, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
							"included_keyspaces":  {Type: schema.TypeList, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
							"excluded_keyspaces":  {Type: schema.TypeList, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
							"included_users":      {Type: schema.TypeList, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
							"excluded_users":      {Type: schema.TypeList, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}},
						},
					},
				},
				"exporter": {
					Type:        schema.TypeList,
					Optional:    true,
					Description: "List of exporters that receive audit logs.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"exporter_uuid": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "UUID of the telemetry provider to send audit logs to.",
							},
							"additional_tags": {
								Type:        schema.TypeMap,
								Optional:    true,
								Description: "Additional string tags appended to each audit log record.",
								Elem:        &schema.Schema{Type: schema.TypeString},
							},
						},
					},
				},
			},
		},
	}
}

func queryLogsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Description: "Query log export configuration. Omit to disable query log export.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ysql_query_log_config": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"enabled":                    {Type: schema.TypeBool, Optional: true, Default: false},
							"log_statement":              {Type: schema.TypeString, Optional: true, Default: "NONE"},
							"log_min_error_statement":    {Type: schema.TypeString, Optional: true, Default: "ERROR"},
							"log_error_verbosity":        {Type: schema.TypeString, Optional: true, Default: "DEFAULT"},
							"log_duration":               {Type: schema.TypeBool, Optional: true, Default: false},
							"debug_print_plan":           {Type: schema.TypeBool, Optional: true, Default: false},
							"log_connections":            {Type: schema.TypeBool, Optional: true, Default: false},
							"log_disconnections":         {Type: schema.TypeBool, Optional: true, Default: false},
							"log_min_duration_statement": {Type: schema.TypeInt, Optional: true, Default: -1},
						},
					},
				},
				"exporter": exporterListSchema(true /* metrics */),
			},
		},
	}
}

func metricsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Description: "Metric export configuration. Omit to disable metric export.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"scrape_interval_seconds": {Type: schema.TypeInt, Optional: true, Default: 30},
				"scrape_timeout_seconds":  {Type: schema.TypeInt, Optional: true, Default: 20},
				"collection_level": {
					Type:         schema.TypeString,
					Optional:     true,
					Default:      "NORMAL",
					ValidateFunc: validation.StringInSlice(allowedCollectionLevels, false),
				},
				"scrape_config_targets": {
					Type:     schema.TypeList,
					Optional: true,
					Elem: &schema.Schema{
						Type:         schema.TypeString,
						ValidateFunc: validation.StringInSlice(allowedScrapeTargets, false),
					},
				},
				"exporter": metricsExporterSchema(),
			},
		},
	}
}

// exporterListSchema describes the per-exporter fields for log exporters.
// `withBatching=true` enables the same `send_batch_*` and `memory_limit_*`
// fields used by query logs and metrics, but not by audit logs.
func exporterListSchema(withBatching bool) *schema.Schema {
	s := map[string]*schema.Schema{
		"exporter_uuid": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "UUID of the telemetry provider that receives the data.",
		},
		"additional_tags": {
			Type:        schema.TypeMap,
			Optional:    true,
			Description: "Additional string tags appended to each record.",
			Elem:        &schema.Schema{Type: schema.TypeString},
		},
	}
	if withBatching {
		s["send_batch_max_size"] = &schema.Schema{Type: schema.TypeInt, Optional: true, Default: 1000}
		s["send_batch_size"] = &schema.Schema{Type: schema.TypeInt, Optional: true, Default: 100}
		s["send_batch_timeout_seconds"] = &schema.Schema{Type: schema.TypeInt, Optional: true, Default: 10}
		s["memory_limit_mib"] = &schema.Schema{Type: schema.TypeInt, Optional: true, Default: 2048}
		s["memory_limit_check_interval_seconds"] = &schema.Schema{Type: schema.TypeInt, Optional: true, Default: 10}
	}
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Elem:     &schema.Resource{Schema: s},
	}
}

func metricsExporterSchema() *schema.Schema {
	s := map[string]*schema.Schema{
		"exporter_uuid": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "UUID of the telemetry provider that receives the metric data.",
		},
		"additional_tags": {
			Type:        schema.TypeMap,
			Optional:    true,
			Description: "Additional string tags appended to each metric.",
			Elem:        &schema.Schema{Type: schema.TypeString},
		},
		"send_batch_max_size":                 {Type: schema.TypeInt, Optional: true, Default: 1000},
		"send_batch_size":                     {Type: schema.TypeInt, Optional: true, Default: 100},
		"send_batch_timeout_seconds":          {Type: schema.TypeInt, Optional: true, Default: 10},
		"memory_limit_mib":                    {Type: schema.TypeInt, Optional: true, Default: 2048},
		"memory_limit_check_interval_seconds": {Type: schema.TypeInt, Optional: true, Default: 10},
		"metrics_prefix": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Optional prefix prepended to every metric name.",
		},
	}
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Elem:     &schema.Resource{Schema: s},
	}
}

// buildExportTelemetryConfigSpec assembles the typed v2 SDK request body for
// the unified `export-telemetry-configs` endpoint based on the resource data.
func buildExportTelemetryConfigSpec(d *schema.ResourceData) clientv2.ExportTelemetryConfigSpec {
	tc := clientv2.TelemetryConfig{}
	if v, ok := d.GetOk("audit_logs"); ok {
		if a := buildAuditLogs(v); a != nil {
			tc.AuditLogs = a
		}
	}
	if v, ok := d.GetOk("query_logs"); ok {
		if q := buildQueryLogs(v); q != nil {
			tc.QueryLogs = q
		}
	}
	if v, ok := d.GetOk("metrics"); ok {
		if m := buildMetrics(v); m != nil {
			tc.Metrics = m
		}
	}
	upgrade := buildUpgradeOptions(d.Get("upgrade_options"))
	return clientv2.ExportTelemetryConfigSpec{
		TelemetryConfig: &tc,
		UpgradeOptions:  &upgrade,
	}
}

// buildDisableSpec builds an `export-telemetry-configs` body that instructs
// YBA to disable all exporters (per the API contract: empty telemetry_config
// disables every exporter).
func buildDisableSpec(d *schema.ResourceData) clientv2.ExportTelemetryConfigSpec {
	upgrade := buildUpgradeOptions(d.Get("upgrade_options"))
	return clientv2.ExportTelemetryConfigSpec{
		TelemetryConfig: &clientv2.TelemetryConfig{},
		UpgradeOptions:  &upgrade,
	}
}

func buildAuditLogs(in interface{}) *clientv2.AuditLogsTelemetrySpec {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	out := &clientv2.AuditLogsTelemetrySpec{
		YsqlAuditConfig: buildYsqlAuditConfig(m["ysql_audit_config"]),
		YcqlAuditConfig: buildYcqlAuditConfig(m["ycql_audit_config"]),
		Exporters:       buildAuditExporters(m["exporter"]),
	}
	return out
}

func buildQueryLogs(in interface{}) *clientv2.QueryLogsTelemetrySpec {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	return &clientv2.QueryLogsTelemetrySpec{
		YsqlQueryLogConfig: buildYsqlQueryLogConfig(m["ysql_query_log_config"]),
		Exporters:          buildBatchedExporters(m["exporter"], false /* withMetricsPrefix */),
	}
}

func buildMetrics(in interface{}) *clientv2.MetricsTelemetrySpec {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	out := &clientv2.MetricsTelemetrySpec{
		ScrapeIntervalSeconds: utils.GetInt32Pointer(int32(intValue(m["scrape_interval_seconds"]))),
		ScrapeTimeoutSeconds:  utils.GetInt32Pointer(int32(intValue(m["scrape_timeout_seconds"]))),
		CollectionLevel:       utils.GetStringPointer(stringValue(m["collection_level"])),
		Exporters:             buildBatchedExporters(m["exporter"], true /* withMetricsPrefix */),
	}
	for _, t := range stringList(m["scrape_config_targets"]) {
		out.ScrapeConfigTargets = append(out.ScrapeConfigTargets, clientv2.ScrapeConfigTargetType(t))
	}
	return out
}

func buildYsqlAuditConfig(in interface{}) *clientv2.YSQLAuditConfig {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	return &clientv2.YSQLAuditConfig{
		Enabled:             boolValue(m["enabled"]),
		Classes:             stringList(m["classes"]),
		LogCatalog:          boolValue(m["log_catalog"]),
		LogClient:           boolValue(m["log_client"]),
		LogLevel:            stringValue(m["log_level"]),
		LogParameter:        boolValue(m["log_parameter"]),
		LogParameterMaxSize: int32Value(m["log_parameter_max_size"]),
		LogRelation:         boolValue(m["log_relation"]),
		LogRows:             boolValue(m["log_rows"]),
		LogStatement:        boolValue(m["log_statement"]),
		LogStatementOnce:    boolValue(m["log_statement_once"]),
	}
}

func buildYcqlAuditConfig(in interface{}) *clientv2.YCQLAuditConfig {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	return &clientv2.YCQLAuditConfig{
		Enabled:            boolValue(m["enabled"]),
		LogLevel:           stringValue(m["log_level"]),
		IncludedCategories: stringList(m["included_categories"]),
		ExcludedCategories: stringList(m["excluded_categories"]),
		IncludedKeyspaces:  stringList(m["included_keyspaces"]),
		ExcludedKeyspaces:  stringList(m["excluded_keyspaces"]),
		IncludedUsers:      stringList(m["included_users"]),
		ExcludedUsers:      stringList(m["excluded_users"]),
	}
}

func buildYsqlQueryLogConfig(in interface{}) *clientv2.YSQLQueryLogConfig {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	return &clientv2.YSQLQueryLogConfig{
		Enabled:                 boolValue(m["enabled"]),
		LogStatement:            stringValue(m["log_statement"]),
		LogMinErrorStatement:    stringValue(m["log_min_error_statement"]),
		LogErrorVerbosity:       stringValue(m["log_error_verbosity"]),
		LogDuration:             boolValue(m["log_duration"]),
		DebugPrintPlan:          boolValue(m["debug_print_plan"]),
		LogConnections:          boolValue(m["log_connections"]),
		LogDisconnections:       boolValue(m["log_disconnections"]),
		LogMinDurationStatement: int32Value(m["log_min_duration_statement"]),
	}
}

// buildAuditExporters builds exporter entries for audit logs. The audit log
// pipeline does not honour the batching/memory fields, so we only emit
// exporter_uuid and additional_tags.
func buildAuditExporters(in interface{}) []clientv2.TelemetryExporterEntry {
	list, ok := in.([]interface{})
	if !ok {
		return nil
	}
	out := make([]clientv2.TelemetryExporterEntry, 0, len(list))
	for _, e := range list {
		m, _ := e.(map[string]interface{})
		if m == nil {
			continue
		}
		entry := clientv2.TelemetryExporterEntry{
			ExporterUuid: stringValue(m["exporter_uuid"]),
		}
		if tags := stringMap(m["additional_tags"]); len(tags) > 0 {
			entry.AdditionalTags = &tags
		}
		out = append(out, entry)
	}
	return out
}

// buildBatchedExporters builds exporter entries that include the OTel
// batching/memory fields used by query logs and metrics. When
// `withMetricsPrefix` is true the optional `metrics_prefix` field is also
// emitted.
func buildBatchedExporters(in interface{}, withMetricsPrefix bool) []clientv2.TelemetryExporterEntry {
	list, ok := in.([]interface{})
	if !ok {
		return nil
	}
	out := make([]clientv2.TelemetryExporterEntry, 0, len(list))
	for _, e := range list {
		m, _ := e.(map[string]interface{})
		if m == nil {
			continue
		}
		entry := clientv2.TelemetryExporterEntry{
			ExporterUuid:                    stringValue(m["exporter_uuid"]),
			SendBatchMaxSize:                utils.GetInt32Pointer(int32(intValue(m["send_batch_max_size"]))),
			SendBatchSize:                   utils.GetInt32Pointer(int32(intValue(m["send_batch_size"]))),
			SendBatchTimeoutSeconds:         utils.GetInt32Pointer(int32(intValue(m["send_batch_timeout_seconds"]))),
			MemoryLimitMib:                  utils.GetInt32Pointer(int32(intValue(m["memory_limit_mib"]))),
			MemoryLimitCheckIntervalSeconds: utils.GetInt32Pointer(int32(intValue(m["memory_limit_check_interval_seconds"]))),
		}
		if tags := stringMap(m["additional_tags"]); len(tags) > 0 {
			entry.AdditionalTags = &tags
		}
		if withMetricsPrefix {
			entry.MetricsPrefix = utils.GetStringPointer(stringValue(m["metrics_prefix"]))
		}
		out = append(out, entry)
	}
	return out
}

// buildUpgradeOptions translates the optional `upgrade_options` block into
// the clientv2 type. When the block is absent we send only RollingUpgrade
// (defaulting to true) and let YBA pick its own restart-sleep defaults
// rather than hard-coding values here. The schema also exposes Default
// values for the sleep fields, so when the user does specify
// `upgrade_options { ... }` the schema layer fills any omitted fields with
// the documented defaults before this function runs.
func buildUpgradeOptions(in interface{}) clientv2.ExportTelemetryUpgradeOptions {
	out := clientv2.ExportTelemetryUpgradeOptions{
		RollingUpgrade: utils.GetBoolPointer(true),
	}
	m := firstMap(in)
	if len(m) == 0 {
		return out
	}
	if v, ok := m["rolling_upgrade"].(bool); ok {
		out.RollingUpgrade = utils.GetBoolPointer(v)
	}
	if v, ok := m["sleep_after_master_restart_millis"].(int); ok && v > 0 {
		out.SleepAfterMasterRestartMillis = utils.GetInt32Pointer(int32(v))
	}
	if v, ok := m["sleep_after_tserver_restart_millis"].(int); ok && v > 0 {
		out.SleepAfterTserverRestartMillis = utils.GetInt32Pointer(int32(v))
	}
	return out
}

func resourceUniverseTelemetryConfigCreate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	universeUUID := d.Get("universe_uuid").(string)
	spec := buildExportTelemetryConfigSpec(d)
	tflog.Info(ctx, fmt.Sprintf(
		"Configuring universe export telemetry config for universe %s", universeUUID))

	if diags := dispatchExportTelemetryConfig(
		ctx, apiClient, universeUUID, spec,
		d.Timeout(schema.TimeoutCreate), "Create"); diags != nil {
		return diags
	}
	d.SetId(universeUUID)
	return resourceUniverseTelemetryConfigRead(ctx, d, meta)
}

func resourceUniverseTelemetryConfigUpdate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	spec := buildExportTelemetryConfigSpec(d)
	if diags := dispatchExportTelemetryConfig(
		ctx, apiClient, d.Id(), spec,
		d.Timeout(schema.TimeoutUpdate), "Update"); diags != nil {
		return diags
	}
	return resourceUniverseTelemetryConfigRead(ctx, d, meta)
}

// resourceUniverseTelemetryConfigDelete asks YBA to disable every exporter
// on the universe by POSTing an empty `telemetry_config: {}` body. If the
// universe has already been deleted out-of-band the API returns a 404; we
// preflight a GetUniverse call to detect that case and remove the resource
// from state cleanly. Every other error is surfaced verbatim so genuine
// failures (permission revoked, task failures, transient outages) cannot
// silently corrupt state.
func resourceUniverseTelemetryConfigDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)

	// Preflight: if the universe is already gone, there is nothing to
	// disable — return cleanly.
	_, response, err := apiClient.YugawareClient.UniverseManagementAPI.
		GetUniverse(ctx, apiClient.CustomerID, d.Id()).Execute()
	if err != nil {
		if utils.IsHTTPNotFound(response) {
			tflog.Warn(ctx, fmt.Sprintf(
				"universe %s not found during telemetry disable; "+
					"removing from state", d.Id()))
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Universe Telemetry Config",
			"Delete - Get Universe"))
	}

	spec := buildDisableSpec(d)
	if diags := dispatchExportTelemetryConfig(
		ctx, apiClient, d.Id(), spec,
		d.Timeout(schema.TimeoutDelete), "Delete"); diags != nil {
		return diags
	}
	d.SetId("")
	return nil
}

// dispatchExportTelemetryConfig submits the unified export-telemetry-configs
// request through utils.DispatchAndWait so all three (Create / Update /
// Delete) call sites share identical conflict-retry, error-formatting, and
// task-waiting behaviour. The closure captures the latest HTTP response so
// the Delete caller can recognise an out-of-band universe deletion (HTTP
// 404) and remove the resource from state instead of erroring.
func dispatchExportTelemetryConfig(
	ctx context.Context,
	apiClient *api.APIClient,
	universeUUID string,
	spec clientv2.ExportTelemetryConfigSpec,
	timeout time.Duration,
	operation string,
) diag.Diagnostics {
	label := fmt.Sprintf("Configure Telemetry on Universe %s (%s)",
		universeUUID, operation)
	return utils.DispatchAndWait(ctx, label,
		apiClient.CustomerID, apiClient.YugawareClient, timeout,
		utils.ResourceEntity, "Universe Telemetry Config", operation,
		func() (string, *http.Response, error) {
			task, resp, err := apiClient.YugawareClientV2.UniverseAPI.
				ConfigureExportTelemetryConfig(
					ctx, apiClient.CustomerID, universeUUID).
				ExportTelemetryConfigSpec(spec).Execute()
			if err != nil {
				return "", resp, err
			}
			if task != nil && task.TaskUuid != nil {
				return *task.TaskUuid, resp, nil
			}
			return "", resp, nil
		})
}

// resourceUniverseTelemetryConfigRead reads the universe details and
// populates the state with the audit/query/metrics export configs YBA
// synced after the unified export task completes.
func resourceUniverseTelemetryConfigRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	c := apiClient.YugawareClient
	uni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, apiClient.CustomerID, d.Id()).
		Execute()
	if err != nil {
		if utils.IsHTTPNotFound(response) {
			tflog.Warn(ctx, fmt.Sprintf(
				"universe %s not found, removing telemetry config from state", d.Id()))
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Universe Telemetry Config", "Read"))
	}
	if err := d.Set("universe_uuid", uni.GetUniverseUUID()); err != nil {
		return diag.FromErr(err)
	}
	clusters := uni.GetUniverseDetails().Clusters
	if len(clusters) == 0 {
		return nil
	}
	intent := clusters[0].UserIntent
	if a := intent.AuditLogConfig; a != nil {
		_ = d.Set("audit_logs", flattenAuditLogConfig(a))
	}
	if q := intent.QueryLogConfig; q != nil {
		_ = d.Set("query_logs", flattenQueryLogConfig(q))
	}
	if m := intent.MetricsExportConfig; m != nil {
		_ = d.Set("metrics", flattenMetricsExportConfig(m))
	}
	return nil
}

// intValue converts a Terraform int (which decodes to int) to a JSON-friendly
// int. Resource defaults and zero are preserved as zero.
func intValue(in interface{}) int {
	if in == nil {
		return 0
	}
	if v, ok := in.(int); ok {
		return v
	}
	return 0
}

// stringList converts a Terraform list of strings (always []interface{}) to a
// concrete []string.
func stringList(in interface{}) []string {
	out := []string{}
	list, ok := in.([]interface{})
	if !ok {
		return out
	}
	for _, item := range list {
		out = append(out, stringValue(item))
	}
	return out
}

// stringMap converts a Terraform string map (always map[string]interface{})
// to a concrete map[string]string.
func stringMap(in interface{}) map[string]string {
	out := map[string]string{}
	m, ok := in.(map[string]interface{})
	if !ok {
		return out
	}
	for k, v := range m {
		out[k] = stringValue(v)
	}
	return out
}

// boolValue extracts a bool from a Terraform value, defaulting to false.
func boolValue(in interface{}) bool {
	if v, ok := in.(bool); ok {
		return v
	}
	return false
}

// int32Value converts a Terraform int (decoded as Go `int`) to int32.
func int32Value(in interface{}) int32 {
	return int32(intValue(in))
}
