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
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
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

// The following enum allow-lists mirror the YBA OpenAPI schemas
// (components/schemas/{YSQLAuditConfig,YCQLAuditConfig,YSQLQueryLogConfig}.yaml)
// so a typo is rejected at plan time with the valid set, instead of failing
// mid-apply when YBA rejects the upgrade.
var (
	allowedYSQLAuditClasses    = []string{"READ", "WRITE", "FUNCTION", "ROLE", "DDL", "MISC", "MISC_SET"}
	allowedYSQLAuditLogLevels  = []string{"DEBUG1", "DEBUG2", "DEBUG3", "DEBUG4", "DEBUG5", "INFO", "NOTICE", "WARNING", "LOG"}
	allowedYCQLAuditCategories = []string{"QUERY", "DML", "DDL", "DCL", "AUTH", "PREPARE", "ERROR", "OTHER"}
	allowedYCQLAuditLogLevels  = []string{"INFO", "WARNING", "ERROR"}
	allowedQueryLogStatements  = []string{"ALL", "NONE", "DDL", "MOD"}
	allowedQueryErrorVerbosity = []string{"VERBOSE", "TERSE", "DEFAULT"}
)

// Default snapshots sourced from the generated platform-go-client. Each
// `NewXxxWithDefaults` constructor encodes the YBA OpenAPI `default:` for its
// type, so wiring schema defaults to these values means bumping the client
// automatically picks up any default YBA changes — we never hand-copy a magic
// number that could silently drift from the server. The lone exception is the
// audit-log config: the YBA API marks every YSQLAuditConfig / YCQLAuditConfig
// field `required` with no `default:`, so the constructor leaves them at their
// Go zero value and the provider must choose sensible defaults itself (these
// mirror the YBA UI and are documented as provider defaults on each field).
var (
	queryLogDefaults       = clientv2.NewYSQLQueryLogConfigWithDefaults()
	metricsDefaults        = clientv2.NewMetricsTelemetrySpecWithDefaults()
	queryExporterDefaults  = clientv2.NewUniverseQueryLogsExporterConfigWithDefaults()
	metricExporterDefaults = clientv2.NewUniverseMetricsExporterConfigWithDefaults()
)

// derefInt32 returns the int value behind a client default pointer, or 0 if the
// client ever stops providing that default (defensive: a nil here would
// otherwise panic at provider init).
func derefInt32(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

// derefString returns the string value behind a client default pointer, or ""
// if absent.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ResourceUniverseTelemetryConfig configures audit log, query log, and
// metric export pipelines for a single YBA universe through the unified
// `export-telemetry-configs` API.
//
// The lifecycle maps to the YBA API as follows:
//
//   - Create / Update: POST /api/v2/customers/{c}/universes/{u}/export-telemetry-configs
//     with a `telemetry_config` body that contains all configured exporters.
//   - Read: GET  /api/v2/customers/{c}/universes/{u}/export-telemetry-configs,
//     which returns the same typed `telemetry_config` shape we POST (so read
//     and write are mirror images over the v2 SDK).
//   - Delete: POST the same unified endpoint with `telemetry_config: {}` to
//     disable all exporters on the universe.
//
// All write operations queue a universe upgrade task on YBA; the resource
// blocks until the task reaches a terminal state via `utils.WaitForTask`.
func ResourceUniverseTelemetryConfig() *schema.Resource {
	return &schema.Resource{
		Description: experimentalAdmonition +
			"Universe Telemetry Config Resource. Attaches audit log, query log, " +
			"and metrics export pipelines to a YBA universe via the unified " +
			"`export-telemetry-configs` API. Each exporter references a " +
			"`yba_telemetry_provider` (or any pre-existing telemetry provider " +
			"UUID) and triggers a rolling/non-rolling restart of the universe to " +
			"install or update the OpenTelemetry collector.\n\n" +
			"~> **Note:** OTLP-based exporters require the global runtime config " +
			"`yb.telemetry.allow_otlp` to be set to `true`. Manage that with the " +
			"`yba_runtime_config` resource.\n\n" +
			"~> **Note:** Import an existing universe-level configuration with the " +
			"universe UUID as the resource ID " +
			"(`terraform import yba_universe_telemetry_config.example <universe-uuid>`); " +
			"state is populated from the unified `export-telemetry-configs` GET API.\n\n" +
			"~> **One resource per universe:** YBA stores a single telemetry " +
			"configuration per universe and this resource owns it wholesale — " +
			"Terraform is the source of truth. On apply it **replaces** whatever the " +
			"universe currently has (including anything configured out-of-band in " +
			"the YBA UI), so manage all three pipelines (`audit_logs`, `query_logs`, " +
			"`metrics`) from a **single** `yba_universe_telemetry_config` block. " +
			"Declaring two resources for the same `universe_uuid` is rejected at " +
			"plan time (they would otherwise overwrite each other on every apply). " +
			"On destroy the resource disables every exporter on the universe, but " +
			"only if a configuration still exists server-side — an already-empty " +
			"universe is left untouched.\n\n" +
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

		// Plan-time validation: reject duplicate / empty exporters within a
		// pipeline and reject two resources claiming the same universe. See
		// customizeUniverseTelemetryDiff for the rationale of each check.
		CustomizeDiff: customizeUniverseTelemetryDiff,

		// Import by universe UUID; Read repopulates state from the unified
		// GetExportTelemetryConfig endpoint.
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

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
							Type:         schema.TypeInt,
							Optional:     true,
							Default:      180000,
							ValidateFunc: validation.IntBetween(0, math.MaxInt32),
							Description: "Sleep between master restarts (ms). Defaults to " +
								"180000 (3 minutes).",
						},
						"sleep_after_tserver_restart_millis": {
							Type:         schema.TypeInt,
							Optional:     true,
							Default:      180000,
							ValidateFunc: validation.IntBetween(0, math.MaxInt32),
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
					Description: "YSQL audit (pgaudit) logging configuration. Declaring this " +
						"block enables YSQL audit logging on the universe — YBA derives " +
						"`enabled` from the block's presence, so there is no `enabled` " +
						"field; omit the block to disable. The YBA API marks every field " +
						"below `required` with no server default, so the `Default` values " +
						"are **provider defaults** chosen to mirror the YBA UI.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"classes": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem: &schema.Schema{
									Type:         schema.TypeString,
									ValidateFunc: validation.StringInSlice(allowedYSQLAuditClasses, false),
								},
								Description: "YSQL audit log classes (e.g. READ, WRITE, DDL, ROLE).",
							},
							"log_catalog": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  true,
							},
							"log_client": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  true,
							},
							"log_level": {
								Type:         schema.TypeString,
								Optional:     true,
								Default:      "LOG",
								ValidateFunc: validation.StringInSlice(allowedYSQLAuditLogLevels, false),
							},
							"log_parameter": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
							},
							"log_parameter_max_size": {
								Type:         schema.TypeInt,
								Optional:     true,
								Default:      0,
								ValidateFunc: validation.IntBetween(0, math.MaxInt32),
							},
							"log_relation": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
							},
							"log_rows": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
							},
							"log_statement": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  true,
							},
							"log_statement_once": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
							},
						},
					},
				},
				"ycql_audit_config": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Description: "YCQL audit logging configuration. Declaring this block " +
						"enables YCQL audit logging — YBA derives `enabled` from the " +
						"block's presence, so there is no `enabled` field; omit the block " +
						"to disable. `log_level`'s `Default` is a **provider default** " +
						"(the YBA API requires the field but defines no default).",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"log_level": {
								Type:         schema.TypeString,
								Optional:     true,
								Default:      "WARNING",
								ValidateFunc: validation.StringInSlice(allowedYCQLAuditLogLevels, false),
							},
							"included_categories": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem: &schema.Schema{
									Type:         schema.TypeString,
									ValidateFunc: validation.StringInSlice(allowedYCQLAuditCategories, false),
								},
							},
							"excluded_categories": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem: &schema.Schema{
									Type:         schema.TypeString,
									ValidateFunc: validation.StringInSlice(allowedYCQLAuditCategories, false),
								},
							},
							"included_keyspaces": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
							},
							"excluded_keyspaces": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
							},
							"included_users": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
							},
							"excluded_users": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
							},
						},
					},
				},
				"exporter": {
					Type:     schema.TypeList,
					Optional: true,
					Description: "Exporter (telemetry destination) for audit logs. Repeat " +
						"this block to fan out to multiple destinations — each block " +
						"becomes one entry in the API's `exporters` array.",
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
					Description: "YSQL query logging configuration. Declaring this block " +
						"enables YSQL query logging — YBA derives `enabled` from the " +
						"block's presence, so there is no `enabled` field; omit the block " +
						"to disable. `Default` values are sourced from the YBA API's own " +
						"`default:` (via the generated client) so they track the server.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"log_statement": {
								Type:         schema.TypeString,
								Optional:     true,
								Default:      queryLogDefaults.LogStatement,
								ValidateFunc: validation.StringInSlice(allowedQueryLogStatements, false),
							},
							"log_min_error_statement": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  queryLogDefaults.LogMinErrorStatement,
							},
							"log_error_verbosity": {
								Type:         schema.TypeString,
								Optional:     true,
								Default:      queryLogDefaults.LogErrorVerbosity,
								ValidateFunc: validation.StringInSlice(allowedQueryErrorVerbosity, false),
							},
							"log_duration": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.LogDuration,
							},
							"debug_print_plan": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.DebugPrintPlan,
							},
							"log_connections": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.LogConnections,
							},
							"log_disconnections": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.LogDisconnections,
							},
							"log_min_duration_statement": {
								Type:     schema.TypeInt,
								Optional: true,
								Default:  int(queryLogDefaults.LogMinDurationStatement),
								// -1 disables duration logging and 0 logs every
								// statement (Postgres semantics); bound the top end so
								// the int32 conversion in buildYsqlQueryLogConfig
								// cannot silently wrap.
								ValidateFunc: validation.IntBetween(-1, math.MaxInt32),
							},
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
				"scrape_interval_seconds": {
					Type:         schema.TypeInt,
					Optional:     true,
					Default:      derefInt32(metricsDefaults.ScrapeIntervalSeconds),
					ValidateFunc: validation.IntBetween(1, math.MaxInt32),
				},
				"scrape_timeout_seconds": {
					Type:         schema.TypeInt,
					Optional:     true,
					Default:      derefInt32(metricsDefaults.ScrapeTimeoutSeconds),
					ValidateFunc: validation.IntBetween(1, math.MaxInt32),
				},
				"collection_level": {
					Type:         schema.TypeString,
					Optional:     true,
					Default:      derefString(metricsDefaults.CollectionLevel),
					ValidateFunc: validation.StringInSlice(allowedCollectionLevels, false),
				},
				"scrape_config_targets": {
					Type:     schema.TypeSet,
					Optional: true,
					// Computed: YBA fills an empty set with every supported
					// target, so an unset config must absorb the server-applied
					// set rather than perpetually diff against it.
					Computed: true,
					Elem: &schema.Schema{
						Type:         schema.TypeString,
						ValidateFunc: validation.StringInSlice(allowedScrapeTargets, false),
					},
					Description: "Scrape target types to include. Omit to let YBA " +
						"include all supported targets.",
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
		// Defaults are sourced from the generated client so they track the YBA
		// OpenAPI `default:` automatically (see queryExporterDefaults). The
		// IntBetween(1, MaxInt32) bound rejects two foot-guns at plan time: a
		// value above 2^31-1 that would silently wrap negative in the int32
		// conversion, and an explicit 0 that utils.GetInt32Pointer would drop
		// from the request, letting YBA substitute its own default and producing
		// a permanent plan diff.
		s["send_batch_max_size"] = &schema.Schema{
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(queryExporterDefaults.SendBatchMaxSize),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		}
		s["send_batch_size"] = &schema.Schema{
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(queryExporterDefaults.SendBatchSize),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		}
		s["send_batch_timeout_seconds"] = &schema.Schema{
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(queryExporterDefaults.SendBatchTimeoutSeconds),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		}
		s["memory_limit_mib"] = &schema.Schema{
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(queryExporterDefaults.MemoryLimitMib),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		}
		s["memory_limit_check_interval_seconds"] = &schema.Schema{
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(queryExporterDefaults.MemoryLimitCheckIntervalSeconds),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		}
	}
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Description: "Exporter (telemetry destination). Repeat this block to send to " +
			"multiple destinations — each becomes one entry in the API's " +
			"`exporters` array.",
		Elem: &schema.Resource{Schema: s},
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
		// Batching defaults are sourced from the generated client so they track
		// the YBA OpenAPI `default:` automatically (see metricExporterDefaults).
		// The IntBetween(1, MaxInt32) bound rejects both an overflowing value
		// (which would wrap negative in the int32 conversion) and an explicit 0
		// (which utils.GetInt32Pointer would drop, yielding a permanent plan
		// diff against YBA's substituted default).
		"send_batch_max_size": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(metricExporterDefaults.SendBatchMaxSize),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		},
		"send_batch_size": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(metricExporterDefaults.SendBatchSize),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		},
		"send_batch_timeout_seconds": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(metricExporterDefaults.SendBatchTimeoutSeconds),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		},
		"memory_limit_mib": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(metricExporterDefaults.MemoryLimitMib),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		},
		"memory_limit_check_interval_seconds": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      derefInt32(metricExporterDefaults.MemoryLimitCheckIntervalSeconds),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
		},
		"metrics_prefix": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Optional prefix prepended to every metric name.",
		},
	}
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Description: "Metric exporter (telemetry destination). Repeat this block to " +
			"send metrics to multiple destinations — each becomes one entry in the " +
			"API's `exporters` array.",
		Elem: &schema.Resource{Schema: s},
	}
}

// customizeUniverseTelemetryDiff is the resource's CustomizeDiff. It runs every
// plan-time guard in turn so a misconfiguration is surfaced before any
// (sensitive) value is written to state or the universe is reconfigured:
//
//   - validateExporters: rejects an empty or duplicated exporter_uuid within a
//     single pipeline.
//   - validateSingleManagerPerUniverse: rejects two yba_universe_telemetry_config
//     resources that claim the same universe in one Terraform run.
func customizeUniverseTelemetryDiff(
	ctx context.Context, d *schema.ResourceDiff, meta interface{},
) error {
	if err := validateExporters(ctx, d, meta); err != nil {
		return err
	}
	return validateSingleManagerPerUniverse(ctx, d, meta)
}

// validateExporters runs at plan time (CustomizeDiff) and rejects a
// configuration whose exporter list within a single pipeline lists the same
// telemetry provider (exporter_uuid) more than once, or lists an exporter with
// an empty exporter_uuid. A provider may legitimately appear across different
// pipelines (audit / query / metrics) of the same universe, and the same
// provider may be shared by many universes — only intra-list duplicates are a
// mistake.
//
// exporter_uuid is `Required`, so an empty value is a genuine misconfiguration
// rather than an unfinished plan; we surface it here (with the precise pipeline
// and position) instead of letting a malformed exporter block sail through plan
// and fail with a less useful error mid-apply. Values that are not yet known
// (e.g. `exporter_uuid = yba_telemetry_provider.x.id` for a provider being
// created in the same apply) are skipped — they resolve before apply.
func validateExporters(
	_ context.Context, d *schema.ResourceDiff, _ interface{},
) error {
	for _, section := range []struct {
		label string
		path  string
	}{
		{"audit_logs", "audit_logs.0.exporter"},
		{"query_logs", "query_logs.0.exporter"},
		{"metrics", "metrics.0.exporter"},
	} {
		list, ok := d.Get(section.path).([]interface{})
		if !ok {
			continue
		}
		seen := make(map[string]struct{}, len(list))
		for i, e := range list {
			m, _ := e.(map[string]interface{})
			if m == nil {
				continue
			}
			uuid := stringValue(m["exporter_uuid"])
			if uuid == "" {
				// An empty exporter_uuid that is still unknown is a computed
				// reference Terraform will resolve before apply — not an error.
				key := fmt.Sprintf("%s.%d.exporter_uuid", section.path, i)
				if !d.NewValueKnown(key) {
					continue
				}
				return fmt.Errorf(
					"%s: exporter #%d has an empty exporter_uuid; every "+
						"exporter must reference a telemetry provider UUID",
					section.label, i+1)
			}
			if _, dup := seen[uuid]; dup {
				return fmt.Errorf(
					"%s: exporter_uuid %q is listed more than once; each "+
						"telemetry provider may appear at most once per pipeline",
					section.label, uuid)
			}
			seen[uuid] = struct{}{}
		}
	}
	return nil
}

// universeTelemetryClaims tracks which universe UUIDs have been claimed by a
// yba_universe_telemetry_config resource during the current Terraform run, so
// that two resources targeting the same universe are rejected at plan time
// rather than silently overwriting each other on every apply.
//
// It is keyed by the provider meta (*api.APIClient), which is created once per
// `terraform` invocation, so the registry is naturally scoped to a single
// plan/apply and to a single provider configuration (aliases get their own
// client, hence their own registry). The stored value is a fingerprint of the
// claiming resource's configuration: a re-invocation of the SAME resource's
// CustomizeDiff carries the same fingerprint and is a no-op, while a DIFFERENT
// resource claiming an already-claimed universe (different fingerprint) is the
// duplicate we reject. Two byte-identical resources converge to the same state
// and are therefore harmless, so they are intentionally not flagged.
//
// Limitation: this catches duplicates within a single Terraform configuration
// (the common "split across modules" foot-gun). Two *separate* configurations
// or state files pointing at the same universe run in different processes and
// cannot be cross-checked here — that case is documented as unsupported.
var (
	universeTelemetryClaimsMu sync.Mutex
	universeTelemetryClaims   = map[*api.APIClient]map[string]string{}
)

// claimUniverse records that universeUUID is managed by a resource with the
// given config fingerprint. It returns true when the universe was already
// claimed by a resource with a DIFFERENT fingerprint (i.e. a real duplicate).
func claimUniverse(client *api.APIClient, universeUUID, fingerprint string) bool {
	universeTelemetryClaimsMu.Lock()
	defer universeTelemetryClaimsMu.Unlock()
	byUniverse := universeTelemetryClaims[client]
	if byUniverse == nil {
		byUniverse = map[string]string{}
		universeTelemetryClaims[client] = byUniverse
	}
	if prev, ok := byUniverse[universeUUID]; ok {
		return prev != fingerprint
	}
	byUniverse[universeUUID] = fingerprint
	return false
}

// validateSingleManagerPerUniverse rejects a plan that declares more than one
// yba_universe_telemetry_config for the same universe. Because YBA stores a
// single telemetry config per universe and this resource replaces it wholesale,
// two resources for one universe would overwrite each other on every apply and
// oscillate forever. See universeTelemetryClaims for the mechanism and its
// scope.
func validateSingleManagerPerUniverse(
	_ context.Context, d *schema.ResourceDiff, meta interface{},
) error {
	client, ok := meta.(*api.APIClient)
	if !ok || client == nil {
		// No provider meta (unit tests exercising the other diff rules in
		// isolation); cross-resource claim tracking only matters during a real
		// plan, where meta is always the configured *api.APIClient.
		return nil
	}
	universeUUID := stringValue(d.Get("universe_uuid"))
	if universeUUID == "" {
		return nil // not yet known; nothing to claim
	}
	if claimUniverse(client, universeUUID, universeConfigFingerprint(d)) {
		return fmt.Errorf(
			"universe %s is already managed by another "+
				"yba_universe_telemetry_config resource in this configuration; "+
				"declare exactly one per universe (a single resource's "+
				"audit_logs / query_logs / metrics blocks manage all three "+
				"pipelines together)", universeUUID)
	}
	return nil
}

// universeConfigFingerprint renders a stable identity for the claiming
// resource: the sorted set of exporter UUIDs declared in each pipeline.
//
// It must be byte-identical across the (up to two) CustomizeDiff invocations of
// the SAME resource and yet differ between two distinct resources, so the claim
// registry can tell a re-invocation apart from a genuine duplicate. SDKv2 runs
// CustomizeDiff a second time whenever the diff RequiresNew (always true here,
// because universe_uuid is ForceNew), and the diff-processed scalar defaults /
// TypeSet internals of a block are NOT stable between those two passes — so we
// deliberately fingerprint only the user-supplied exporter_uuid strings, which
// are stable. Two resources whose exporter sets are byte-identical converge to
// the same server state and are therefore harmless to leave unflagged.
func universeConfigFingerprint(d *schema.ResourceDiff) string {
	var b strings.Builder
	for _, path := range []string{
		"audit_logs.0.exporter",
		"query_logs.0.exporter",
		"metrics.0.exporter",
	} {
		uuids := []string{}
		if list, ok := d.Get(path).([]interface{}); ok {
			for _, e := range list {
				if m, _ := e.(map[string]interface{}); m != nil {
					uuids = append(uuids, stringValue(m["exporter_uuid"]))
				}
			}
		}
		sort.Strings(uuids)
		b.WriteString(strings.Join(uuids, ","))
		b.WriteString("|")
	}
	return b.String()
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
		Exporters:          buildQueryExporters(m["exporter"]),
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
		Exporters:             buildMetricsExporters(m["exporter"]),
	}
	for _, t := range stringList(m["scrape_config_targets"]) {
		out.ScrapeConfigTargets = append(
			out.ScrapeConfigTargets,
			clientv2.ScrapeConfigTargetType(t),
		)
	}
	return out
}

func buildYsqlAuditConfig(in interface{}) *clientv2.YSQLAuditConfig {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	return &clientv2.YSQLAuditConfig{
		// enabled is readOnly in the YBA API and derived server-side from this
		// block's presence (the v2 field is required, so we must still set it).
		Enabled:             true,
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
		// enabled is derived server-side from this block's presence; the v2
		// field is required so we always set it true.
		Enabled:            true,
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
		// enabled is readOnly in the YBA API and derived server-side from this
		// block's presence (the v2 field is required, so we must still set it).
		Enabled:                 true,
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

// exporterRows normalizes a TypeList of exporter blocks to a slice of maps,
// dropping nil entries. Shared by the three pipeline-specific exporter
// builders below, which exist as separate functions because the v2 SDK models
// each pipeline's exporter with a distinct type (audit logs carry no
// batching fields; metrics additionally carry metrics_prefix).
func exporterRows(in interface{}) []map[string]interface{} {
	list, ok := in.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(list))
	for _, e := range list {
		if m, _ := e.(map[string]interface{}); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// buildAuditExporters builds exporter entries for audit logs. The audit log
// pipeline does not honour the batching/memory fields, so we only emit
// exporter_uuid and additional_tags.
func buildAuditExporters(in interface{}) []clientv2.UniverseLogsExporterConfig {
	rows := exporterRows(in)
	out := make([]clientv2.UniverseLogsExporterConfig, 0, len(rows))
	for _, m := range rows {
		entry := clientv2.UniverseLogsExporterConfig{
			ExporterUuid: stringValue(m["exporter_uuid"]),
		}
		if tags := stringMap(m["additional_tags"]); len(tags) > 0 {
			entry.AdditionalTags = &tags
		}
		out = append(out, entry)
	}
	return out
}

// buildQueryExporters builds query-log exporter entries, which carry the OTel
// batching/memory fields but not metrics_prefix.
func buildQueryExporters(in interface{}) []clientv2.UniverseQueryLogsExporterConfig {
	rows := exporterRows(in)
	out := make([]clientv2.UniverseQueryLogsExporterConfig, 0, len(rows))
	for _, m := range rows {
		entry := clientv2.UniverseQueryLogsExporterConfig{
			ExporterUuid: stringValue(m["exporter_uuid"]),
			SendBatchMaxSize: utils.GetInt32Pointer(
				int32(intValue(m["send_batch_max_size"])),
			),
			SendBatchSize: utils.GetInt32Pointer(
				int32(intValue(m["send_batch_size"])),
			),
			SendBatchTimeoutSeconds: utils.GetInt32Pointer(
				int32(intValue(m["send_batch_timeout_seconds"])),
			),
			MemoryLimitMib: utils.GetInt32Pointer(
				int32(intValue(m["memory_limit_mib"])),
			),
			MemoryLimitCheckIntervalSeconds: utils.GetInt32Pointer(
				int32(intValue(m["memory_limit_check_interval_seconds"])),
			),
		}
		if tags := stringMap(m["additional_tags"]); len(tags) > 0 {
			entry.AdditionalTags = &tags
		}
		out = append(out, entry)
	}
	return out
}

// buildMetricsExporters builds metric exporter entries, which carry the OTel
// batching/memory fields plus the optional metrics_prefix.
func buildMetricsExporters(in interface{}) []clientv2.UniverseMetricsExporterConfig {
	rows := exporterRows(in)
	out := make([]clientv2.UniverseMetricsExporterConfig, 0, len(rows))
	for _, m := range rows {
		entry := clientv2.UniverseMetricsExporterConfig{
			ExporterUuid: stringValue(m["exporter_uuid"]),
			SendBatchMaxSize: utils.GetInt32Pointer(
				int32(intValue(m["send_batch_max_size"])),
			),
			SendBatchSize: utils.GetInt32Pointer(
				int32(intValue(m["send_batch_size"])),
			),
			SendBatchTimeoutSeconds: utils.GetInt32Pointer(
				int32(intValue(m["send_batch_timeout_seconds"])),
			),
			MemoryLimitMib: utils.GetInt32Pointer(
				int32(intValue(m["memory_limit_mib"])),
			),
			MemoryLimitCheckIntervalSeconds: utils.GetInt32Pointer(
				int32(intValue(m["memory_limit_check_interval_seconds"])),
			),
			MetricsPrefix: utils.GetStringPointer(
				stringValue(m["metrics_prefix"]),
			),
		}
		if tags := stringMap(m["additional_tags"]); len(tags) > 0 {
			entry.AdditionalTags = &tags
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
	return append(
		diag.Diagnostics{experimentalWarning("yba_universe_telemetry_config")},
		resourceUniverseTelemetryConfigRead(ctx, d, meta)...)
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
	return append(
		diag.Diagnostics{experimentalWarning("yba_universe_telemetry_config")},
		resourceUniverseTelemetryConfigRead(ctx, d, meta)...)
}

// resourceUniverseTelemetryConfigDelete tears down the universe's telemetry
// configuration by POSTing an empty `telemetry_config: {}` body, which disables
// every exporter on the universe.
//
// It first GETs the current config via the v2 endpoint, which serves two
// purposes:
//
//   - If the universe has been deleted out-of-band (404, or YBA's non-404
//     "Cannot find universe" body) there is nothing to disable — remove the
//     resource from state cleanly.
//   - If the universe has no telemetry config at all (already disabled, or
//     cleared out-of-band) we do NOT greedily POST a disable spec. Doing so
//     would queue a pointless rolling restart and could clobber a config that
//     someone has since set up to take over once Terraform stops managing the
//     universe. We simply drop the resource from state.
//
// Only when a configuration actually exists do we disable it (Terraform owns
// the universe's telemetry config, so a full disable on destroy is correct).
// Every other error is surfaced verbatim so genuine failures (permission
// revoked, task failures, transient outages) cannot silently corrupt state.
func resourceUniverseTelemetryConfigDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	universeUUID := d.Id()

	config, err := getExportTelemetryConfig(
		ctx, apiClient, universeUUID, "Delete - Get Config")
	if err != nil {
		if errors.Is(err, errUniverseMissing) {
			tflog.Warn(ctx, fmt.Sprintf(
				"universe %s not found during telemetry disable; "+
					"removing from state", universeUUID))
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	if telemetryConfigIsEmpty(config) {
		tflog.Info(ctx, fmt.Sprintf(
			"universe %s has no telemetry config to disable; removing from "+
				"state without reconfiguring the universe", universeUUID))
		d.SetId("")
		return nil
	}

	spec := buildDisableSpec(d)
	if diags := dispatchExportTelemetryConfig(
		ctx, apiClient, universeUUID, spec,
		d.Timeout(schema.TimeoutDelete), "Delete"); diags != nil {
		return diags
	}
	d.SetId("")
	return nil
}

// telemetryConfigIsEmpty reports whether a universe's telemetry config has no
// exporters of any kind. YBA returns an empty (or nil) TelemetryConfig when
// nothing is configured, in which case there is nothing for delete to disable.
func telemetryConfigIsEmpty(c *clientv2.TelemetryConfig) bool {
	return c == nil ||
		(c.AuditLogs == nil && c.QueryLogs == nil && c.Metrics == nil)
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

// resourceUniverseTelemetryConfigRead reads the universe's current export
// telemetry config via the dedicated v2 GetExportTelemetryConfig endpoint and
// populates state from it. Using the typed v2 endpoint (rather than
// spelunking universeDetails.clusters[0].userIntent over the v1 client) keeps
// the read path a mirror of the write path — both speak the same
// clientv2.TelemetryConfig — and is what makes import possible.
//
// Each section is set unconditionally (the flatten helpers return nil for an
// absent section), so an exporter disabled out-of-band surfaces as drift
// instead of lingering in state.
func resourceUniverseTelemetryConfigRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	universeUUID := d.Id()
	config, err := getExportTelemetryConfig(ctx, apiClient, universeUUID, "Read")
	if err != nil {
		if errors.Is(err, errUniverseMissing) {
			tflog.Warn(ctx, fmt.Sprintf(
				"universe %s not found, removing telemetry config from state", universeUUID))
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}
	if config == nil {
		config = &clientv2.TelemetryConfig{}
	}
	if err := d.Set("universe_uuid", universeUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("audit_logs", flattenAuditLogsSpec(config.AuditLogs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("query_logs", flattenQueryLogsSpec(config.QueryLogs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("metrics", flattenMetricsSpec(config.Metrics)); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

// errUniverseMissing is the typed sentinel returned by getExportTelemetryConfig
// when YBA reports the universe behind a telemetry config as already gone. YBA
// surfaces a deleted universe through more than one HTTP shape — a 404, or a
// non-404 whose body carries one of universeMissingMarkers (its
// Universe.getOrBadRequest returns a 400/500 "Cannot find universe ..." rather
// than a 404) — all of which collapse into this sentinel so CRUD code detects a
// missing universe with errors.Is instead of substring-matching the response
// body itself (see AGENTS.md, Error & Task Handling).
var errUniverseMissing = errors.New("universe does not exist")

// universeMissingMarkers lists the substrings YBA returns in the response body
// when the universe no longer exists. Kept next to the sentinel so the CRUD
// functions never reason about the wire format.
var universeMissingMarkers = []string{
	"Cannot find universe",
	"does not exist",
}

// getExportTelemetryConfig fetches the universe's unified telemetry config via
// the v2 endpoint, translating an out-of-band universe deletion (a 404, or a
// non-404 body marker) into the typed errUniverseMissing sentinel. Any other
// HTTP error is returned pre-formatted via utils.ErrorFromHTTPResponse, so the
// caller can surface it verbatim with diag.FromErr.
func getExportTelemetryConfig(
	ctx context.Context, apiClient *api.APIClient, universeUUID, operation string,
) (*clientv2.TelemetryConfig, error) {
	config, response, err := apiClient.YugawareClientV2.UniverseAPI.
		GetExportTelemetryConfig(ctx, apiClient.CustomerID, universeUUID).Execute()
	if err != nil {
		if utils.IsHTTPNotFound(response) || bodyHasMissingMarker(err) {
			return nil, errUniverseMissing
		}
		return nil, utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Universe Telemetry Config", operation)
	}
	return config, nil
}

// bodyHasMissingMarker reports whether err carries one of the YBA response
// bodies that mean "the universe no longer exists" — see universeMissingMarkers.
func bodyHasMissingMarker(err error) bool {
	body := utils.OpenAPIErrorBody(err)
	for _, m := range universeMissingMarkers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
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

// stringList converts a Terraform collection of strings to a concrete
// []string. TypeList fields decode to []interface{}; TypeSet fields decode
// to *schema.Set, so callers can hand either shape to this helper.
func stringList(in interface{}) []string {
	out := []string{}
	switch v := in.(type) {
	case []interface{}:
		for _, item := range v {
			out = append(out, stringValue(item))
		}
	case *schema.Set:
		for _, item := range v.List() {
			out = append(out, stringValue(item))
		}
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
