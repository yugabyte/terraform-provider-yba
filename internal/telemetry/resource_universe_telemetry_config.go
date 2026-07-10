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

var allowedScrapeTargets = []string{
	"MASTER_EXPORT",
	"TSERVER_EXPORT",
	"YSQL_EXPORT",
	"CQL_EXPORT",
	"NODE_EXPORT",
	"NODE_AGENT_EXPORT",
	"OTEL_EXPORT",
}

var allowedCollectionLevels = []string{"ALL", "NORMAL", "TABLE_OFF", "MINIMAL", "OFF"}

var (
	allowedYSQLAuditClasses = []string{
		"READ",
		"WRITE",
		"FUNCTION",
		"ROLE",
		"DDL",
		"MISC",
		"MISC_SET",
	}
	allowedYSQLAuditLogLevels = []string{
		"DEBUG1",
		"DEBUG2",
		"DEBUG3",
		"DEBUG4",
		"DEBUG5",
		"INFO",
		"NOTICE",
		"WARNING",
		"LOG",
	}
	allowedYCQLAuditCategories = []string{
		"QUERY",
		"DML",
		"DDL",
		"DCL",
		"AUTH",
		"PREPARE",
		"ERROR",
		"OTHER",
	}
	allowedYCQLAuditLogLevels  = []string{"INFO", "WARNING", "ERROR"}
	allowedQueryLogStatements  = []string{"ALL", "NONE", "DDL", "MOD"}
	allowedQueryErrorVerbosity = []string{"VERBOSE", "TERSE", "DEFAULT"}
)

// Schema defaults are wired to these client constructors so they track the YBA
// OpenAPI `default:` on a client bump, instead of hand-copied magic numbers.
// Audit-log config is the exception: those fields are required with no server
// default, so the provider picks its own (see the audit schema Defaults).
var (
	queryLogDefaults       = clientv2.NewYSQLQueryLogConfigWithDefaults()
	metricsDefaults        = clientv2.NewMetricsTelemetrySpecWithDefaults()
	queryExporterDefaults  = clientv2.NewUniverseQueryLogsExporterConfigWithDefaults()
	metricExporterDefaults = clientv2.NewUniverseMetricsExporterConfigWithDefaults()
)

func derefInt32(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ResourceUniverseTelemetryConfig configures audit-log, query-log, and metric
// export pipelines for a single universe via the unified export-telemetry-configs
// API. Every write queues a universe upgrade task the resource waits on.
func ResourceUniverseTelemetryConfig() *schema.Resource {
	return &schema.Resource{
		Description: experimentalAdmonition +
			"Universe Telemetry Config Resource. Attaches audit log, query log, " +
			"and metrics export pipelines to a YBA universe via the unified " +
			"`export-telemetry-configs` API. Each exporter references a " +
			"telemetry provider resource (`yba_datadog_telemetry_provider`, " +
			"`yba_otlp_telemetry_provider`, ... — or any pre-existing telemetry " +
			"provider UUID) and triggers a rolling/non-rolling restart of the " +
			"universe to install or update the OpenTelemetry collector.\n\n" +
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
			"reference like `yba_datadog_telemetry_provider.x.id`, Terraform's dependency " +
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

		CustomizeDiff: customizeUniverseTelemetryDiff,

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
									Type: schema.TypeString,
									ValidateFunc: validation.StringInSlice(
										allowedYSQLAuditClasses,
										false,
									),
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
								Type:     schema.TypeString,
								Optional: true,
								Default:  "LOG",
								ValidateFunc: validation.StringInSlice(
									allowedYSQLAuditLogLevels,
									false,
								),
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
								Type:     schema.TypeString,
								Optional: true,
								Default:  "WARNING",
								ValidateFunc: validation.StringInSlice(
									allowedYCQLAuditLogLevels,
									false,
								),
							},
							"included_categories": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem: &schema.Schema{
									Type: schema.TypeString,
									ValidateFunc: validation.StringInSlice(
										allowedYCQLAuditCategories,
										false,
									),
								},
							},
							"excluded_categories": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem: &schema.Schema{
									Type: schema.TypeString,
									ValidateFunc: validation.StringInSlice(
										allowedYCQLAuditCategories,
										false,
									),
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
								Type:     schema.TypeString,
								Optional: true,
								Default:  queryLogDefaults.LogStatement,
								ValidateFunc: validation.StringInSlice(
									allowedQueryLogStatements,
									false,
								),
							},
							"log_min_error_statement": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  queryLogDefaults.LogMinErrorStatement,
							},
							"log_error_verbosity": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  queryLogDefaults.LogErrorVerbosity,
								ValidateFunc: validation.StringInSlice(
									allowedQueryErrorVerbosity,
									false,
								),
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
								// Bound the top end so the int32 conversion can't wrap.
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
					// Computed: YBA fills an empty set with every target, so an
					// unset config must absorb it rather than diff forever.
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

// exporterListSchema builds the per-exporter fields; withBatching adds the
// send_batch_*/memory_limit_* fields (query logs + metrics, not audit logs).
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
		// IntBetween(1, MaxInt32) rejects two footguns: an overflowing value
		// (wraps negative in the int32 conversion) and an explicit 0
		// (GetInt32Pointer drops it, so YBA substitutes its default and diffs forever).
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
		// See exporterListSchema: IntBetween(1, MaxInt32) guards int32 wrap and
		// the 0-dropped-diff footgun.
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

func customizeUniverseTelemetryDiff(
	ctx context.Context, d *schema.ResourceDiff, meta interface{},
) error {
	if err := validateExporters(ctx, d, meta); err != nil {
		return err
	}
	return validateSingleManagerPerUniverse(ctx, d, meta)
}

// validateExporters rejects a duplicate or empty exporter_uuid within one
// pipeline. A provider may repeat across pipelines or universes — only
// intra-pipeline duplicates are the mistake. Unknown values are skipped.
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
				// Empty but unknown = a computed reference resolved before apply.
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

// universeTelemetryClaims records which universes a config resource has claimed
// this run, to reject two resources targeting one universe (they'd overwrite each
// other every apply). Keyed by provider meta (*api.APIClient), which is per
// terraform invocation, so it's scoped to one plan/apply. The value fingerprints
// the resource's config: a re-invocation of the same resource matches and is a
// no-op; a different resource on the same universe is the duplicate we reject.
// Only catches duplicates within one configuration — separate state files run in
// separate processes and can't be cross-checked.
var (
	universeTelemetryClaimsMu sync.Mutex
	universeTelemetryClaims   = map[*api.APIClient]map[string]string{}
)

// claimUniverse records universeUUID's fingerprint and returns true if it was
// already claimed with a different fingerprint (a real duplicate).
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

// validateSingleManagerPerUniverse rejects more than one resource per universe:
// YBA stores one config per universe and this resource replaces it wholesale, so
// two would oscillate forever. See universeTelemetryClaims.
func validateSingleManagerPerUniverse(
	_ context.Context, d *schema.ResourceDiff, meta interface{},
) error {
	client, ok := meta.(*api.APIClient)
	if !ok || client == nil {
		// No provider meta: unit tests exercising the other diff rules alone.
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

// universeConfigFingerprint identifies the claiming resource by its sorted
// exporter UUIDs. It must be stable across the two CustomizeDiff passes SDKv2 runs
// for a ForceNew diff, so we fingerprint only the user-supplied exporter_uuids —
// scalar defaults and TypeSet internals aren't stable between those passes.
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

// buildDisableSpec builds an empty telemetry_config body, which YBA treats as
// "disable every exporter".
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
		// enabled is derived server-side from block presence; required field, so set it.
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
		// enabled is derived server-side from block presence; required field, so set it.
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
		// enabled is derived server-side from block presence; required field, so set it.
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

// exporterRows normalizes a TypeList of exporter blocks to a slice of maps. The
// three pipeline builders below are separate because the v2 SDK types differ.
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

// buildAuditExporters builds audit-log exporters — no batching fields, just
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

// buildUpgradeOptions translates the optional upgrade_options block. When absent,
// only RollingUpgrade is sent (true) and YBA picks its own restart sleeps.
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

// resourceUniverseTelemetryConfigDelete disables every exporter by POSTing an
// empty telemetry_config. It GETs first: a missing universe just drops from state,
// and an already-empty config is left alone (no pointless restart, and no
// clobbering a config set up out-of-band to take over). Other errors surface.
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

func telemetryConfigIsEmpty(c *clientv2.TelemetryConfig) bool {
	return c == nil ||
		(c.AuditLogs == nil && c.QueryLogs == nil && c.Metrics == nil)
}

// dispatchExportTelemetryConfig runs the request through utils.DispatchAndWait so
// Create/Update/Delete share the same conflict-retry, error-formatting, and task-wait.
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

// resourceUniverseTelemetryConfigRead populates state from the v2
// GetExportTelemetryConfig endpoint (a mirror of the write path, which is what
// makes import work). Sections are set unconditionally, so an out-of-band disable
// surfaces as drift rather than lingering.
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

// errUniverseMissing collapses YBA's ways of reporting a gone universe (a 404, or
// a non-404 with a universeMissingMarkers body) so CRUD uses errors.Is, not
// substring matching. See AGENTS.md, Error & Task Handling.
var errUniverseMissing = errors.New("universe does not exist")

var universeMissingMarkers = []string{
	"Cannot find universe",
	"does not exist",
}

// getExportTelemetryConfig fetches the v2 telemetry config, mapping a gone
// universe to errUniverseMissing and other errors to a formatted error.
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

func bodyHasMissingMarker(err error) bool {
	body := utils.OpenAPIErrorBody(err)
	for _, m := range universeMissingMarkers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
}

func intValue(in interface{}) int {
	if in == nil {
		return 0
	}
	if v, ok := in.(int); ok {
		return v
	}
	return 0
}

// stringList converts a Terraform string collection to []string, accepting either
// a TypeList ([]interface{}) or a TypeSet (*schema.Set).
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

func boolValue(in interface{}) bool {
	if v, ok := in.(bool); ok {
		return v
	}
	return false
}

func int32Value(in interface{}) int32 {
	return int32(intValue(in))
}
