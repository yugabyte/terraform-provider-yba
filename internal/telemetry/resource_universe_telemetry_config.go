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
	"maps"
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
	allowedServerLogLevels     = []string{"INFO", "WARNING", "ERROR", "FATAL"}
)

// Schema defaults are wired to these client constructors so they track the YBA
// OpenAPI `default:` on a client bump, instead of hand-copied magic numbers.
// Audit-log config is the exception: those fields are required with no server
// default, so the provider picks its own (see the audit schema Defaults).
var (
	queryLogDefaults           = clientv2.NewYSQLQueryLogConfigWithDefaults()
	metricsDefaults            = clientv2.NewMetricsTelemetrySpecWithDefaults()
	queryExporterDefaults      = clientv2.NewUniverseQueryLogsExporterConfigWithDefaults()
	metricExporterDefaults     = clientv2.NewUniverseMetricsExporterConfigWithDefaults()
	masterLogsDefaults         = clientv2.NewMasterLogsTelemetrySpecWithDefaults()
	tserverLogsDefaults        = clientv2.NewTServerLogsTelemetrySpecWithDefaults()
	serverLogsExporterDefaults = clientv2.NewUniverseServerLogsExporterConfigWithDefaults()
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

func derefFloat64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// codeList renders allowed values for a Description: "`A`, `B` or `C`".
func codeList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "`" + v + "`"
	}
	if len(quoted) < 2 {
		return strings.Join(quoted, "")
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}

// defaultNote renders a schema Default for a Description; tfplugindocs does
// not print Default on its own.
func defaultNote(v interface{}) string {
	return fmt.Sprintf("Defaults to `%v`.", v)
}

// ResourceUniverseTelemetryConfig configures audit-log, query-log, server-log,
// and metric export pipelines for a single universe via the unified
// export-telemetry-configs API. Every write queues a universe upgrade task the
// resource waits on.
func ResourceUniverseTelemetryConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the telemetry export configuration of a universe in " +
			"YugabyteDB Anywhere: the audit log, query log, server log and metrics " +
			"pipelines, and the telemetry providers that each pipeline sends data " +
			"to.\n\n" +
			previewAdmonition + "\n\n" +
			"YBA runs an OpenTelemetry Collector on the universe nodes to export " +
			"the data.\n\n" +
			"~> **Note:** " + versionNote("This resource requires", unifiedTelemetryAPIMin) +
			"\n\n" +
			"~> **Note:** Each create and each update restarts the yb-master and " +
			"yb-tserver processes on every node of the universe. By default, YBA " +
			"restarts one server at a time and waits 3 minutes after each restart, " +
			"so a universe with 3 masters and 9 tservers waits 36 minutes in total. " +
			"Use `upgrade_options` to change the wait or to restart all nodes at " +
			"once. Destroy turns off every pipeline on the universe, which also " +
			"restarts it. If the universe has no telemetry configuration at that " +
			"time, destroy only removes the resource from the state.\n\n" +
			"~> **Note:** YBA stores one telemetry configuration per universe, and " +
			"this resource replaces all of it on each apply. A pipeline that the " +
			"configuration does not declare is turned off, also when it was set up " +
			"in the YBA UI. Manage all pipelines of a universe in one " +
			"`yba_universe_telemetry_config` resource. The provider rejects a " +
			"second resource for the same `universe_uuid` in one configuration at " +
			"plan time. It cannot detect a second resource in another Terraform " +
			"state.\n\n" +
			"~> **Note:** Datadog and OTLP telemetry providers accept logs and " +
			"metrics. Dynatrace accepts only metrics. AWS CloudWatch, GCP Cloud " +
			"Monitoring, Splunk and S3 accept only logs. All AWS CloudWatch and S3 " +
			"telemetry providers that one universe uses must have the same access " +
			"key and secret key. All GCP Cloud Monitoring telemetry providers that " +
			"one universe uses must have the same credentials.\n\n" +
			"~> **Note:** To export from a Kubernetes universe, install the " +
			"OpenTelemetry Operator in its Kubernetes cluster first. On a " +
			"Kubernetes universe, `node_agent_logs` and `ynp_logs` are not " +
			"available. Audit log export needs YugabyteDB " +
			"2025.1.0.0 or later on the universe, and the other pipelines need " +
			"YugabyteDB 2026.1.2.0 or later. `metrics` must set " +
			"`scrape_config_targets` to a subset of `MASTER_EXPORT`, " +
			"`TSERVER_EXPORT`, `YSQL_EXPORT`, `CQL_EXPORT` and `OTEL_EXPORT`.\n\n" +
			"~> **Note:** When `exporter_uuid` refers to a telemetry provider " +
			"resource, Terraform orders the work itself, and `depends_on` is not " +
			"needed. When an apply replaces a telemetry provider that this " +
			"universe uses, the universe restarts twice. The first restart removes " +
			"the old telemetry provider before Terraform deletes it. A pipeline " +
			"whose only exporter was the old telemetry provider stays off until " +
			"the second restart, which adds the new telemetry provider.",

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
				Description: "UUID of the universe. A change replaces the resource.",
			},
			"audit_logs":         auditLogsSchema(),
			"query_logs":         queryLogsSchema(),
			"metrics":            metricsSchema(),
			"master_logs":        masterLogsSchema(),
			"tserver_logs":       tserverLogsSchema(),
			"ysql_conn_mgr_logs": serverLogsSchema("YSQL Connection Manager", kubernetesLogsNote),
			"node_agent_logs":    serverLogsSchema("Node agent", vmOnlyLogsNote),
			"ynp_logs":           serverLogsSchema("YNP (node provisioning)", vmOnlyLogsNote),
			"controller_logs":    serverLogsSchema("YB-Controller", kubernetesLogsNote),
			"upgrade_options": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Description: "Options for the restart that applies each change. YBA " +
					"uses them only with a change to a pipeline, and rejects an apply " +
					"that changes only `upgrade_options`. Lower the wait times to " +
					"apply a change faster on a universe with a light load, and raise " +
					"them for a universe under heavy load.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"rolling_upgrade": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							Description: "Restart one server at a time. Set to `false` " +
								"to restart all nodes at once, which makes the universe " +
								"unavailable during the restart. Defaults to `true`.",
						},
						"sleep_after_master_restart_millis": {
							Type:         schema.TypeInt,
							Optional:     true,
							Default:      180000,
							ValidateFunc: validation.IntBetween(0, math.MaxInt32),
							Description: "Time, in milliseconds, that YBA waits after it " +
								"restarts each yb-master. Defaults to `180000` (3 minutes). " +
								"`0` uses the YBA runtime config " +
								"`yb.upgrade.sleep_after_master_restart_ms`.",
						},
						"sleep_after_tserver_restart_millis": {
							Type:         schema.TypeInt,
							Optional:     true,
							Default:      180000,
							ValidateFunc: validation.IntBetween(0, math.MaxInt32),
							Description: "Time, in milliseconds, that YBA waits after it " +
								"restarts each yb-tserver. Defaults to `180000` (3 minutes). " +
								"`0` uses the YBA runtime config " +
								"`yb.upgrade.sleep_after_tserver_restart_ms`.",
						},
					},
				},
			},
		},
	}
}

func auditLogsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Description: "Audit logging and audit log export. Omit the block to turn " +
			"off audit logging and its export.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ysql_audit_config": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Description: "YSQL audit logging, through the pgaudit extension. " +
						"The block turns on YSQL audit logging. Omit it to turn YSQL " +
						"audit logging off.",
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
								Description: "Sets `pgaudit.log`: the classes of statements " +
									"to log. Allowed values: " +
									codeList(allowedYSQLAuditClasses) + ". When not set, YBA " +
									"does not set `pgaudit.log`, whose default, `none`, logs " +
									"no statements.",
							},
							"log_catalog": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  true,
								Description: "Sets `pgaudit.log_catalog`: also log statements " +
									"whose relations are all in `pg_catalog`. Set to `false` to " +
									"drop the catalog lookups that tools make. Defaults to `true`.",
							},
							"log_client": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  true,
								Description: "Sets `pgaudit.log_client`: also send audit " +
									"messages to the client, such as ysqlsh. Defaults to `true`.",
							},
							"log_level": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  "LOG",
								ValidateFunc: validation.StringInSlice(
									allowedYSQLAuditLogLevels,
									false,
								),
								Description: "Sets `pgaudit.log_level`: the severity of the " +
									"audit log entries when `log_client` is `true`. When " +
									"`log_client` is `false`, pgaudit uses `LOG`. " +
									"Allowed values: " +
									codeList(allowedYSQLAuditLogLevels) + ". Defaults to `LOG`. " +
									"A level below `WARNING` (`DEBUG1` to `DEBUG5`, `INFO` or " +
									"`NOTICE`) also needs the yb-tserver flag " +
									"`ysql_log_min_messages` set to that level or a more " +
									"verbose one.",
							},
							"log_parameter": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
								Description: "Sets `pgaudit.log_parameter`: include the " +
									"statement parameters in the audit log. Defaults to `false`.",
							},
							"log_parameter_max_size": {
								Type:         schema.TypeInt,
								Optional:     true,
								Default:      0,
								ValidateFunc: validation.IntBetween(0, math.MaxInt32),
								Description: "Sets `pgaudit.log_parameter_max_size`: the " +
									"largest parameter, in bytes, to log when `log_parameter` " +
									"is `true`. A longer parameter is replaced with " +
									"`<long param suppressed>`. `0` logs every parameter. " +
									"Defaults to `0`.",
							},
							"log_relation": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
								Description: "Sets `pgaudit.log_relation`: write a separate " +
									"entry for each relation that a SELECT or DML statement " +
									"references. Defaults to `false`.",
							},
							"log_rows": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
								Description: "Sets `pgaudit.log_rows`: include the number of " +
									"rows that the statement retrieved or changed. Defaults to " +
									"`false`.",
							},
							"log_statement": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  true,
								Description: "Sets `pgaudit.log_statement`: include the " +
									"statement text and parameters. Defaults to `true`.",
							},
							"log_statement_once": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  false,
								Description: "Sets `pgaudit.log_statement_once`: include the " +
									"statement text and parameters only in the first entry " +
									"for a statement or sub-statement. Defaults to `false`.",
							},
						},
					},
				},
				"ycql_audit_config": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Description: "YCQL audit logging. The block turns on YCQL audit " +
						"logging. Omit it to turn YCQL audit logging off.",
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
								Description: "Sets `ycql_audit_log_level`: the severity of " +
									"audit records, which selects the yb-tserver log file " +
									"they go to. Allowed values: " +
									codeList(allowedYCQLAuditLogLevels) +
									". Defaults to `WARNING`.",
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
								Description: "Sets `ycql_audit_included_categories`: the " +
									"statement categories to audit. Allowed values: " +
									codeList(allowedYCQLAuditCategories) + ".",
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
								Description: "Sets `ycql_audit_excluded_categories`: the " +
									"statement categories not to audit. Allowed values: " +
									codeList(allowedYCQLAuditCategories) + ".",
							},
							"included_keyspaces": {
								Type:        schema.TypeSet,
								Optional:    true,
								Elem:        &schema.Schema{Type: schema.TypeString},
								Description: "Sets `ycql_audit_included_keyspaces`: the keyspaces to audit.",
							},
							"excluded_keyspaces": {
								Type:     schema.TypeSet,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
								Description: "Sets `ycql_audit_excluded_keyspaces`: the " +
									"keyspaces not to audit.",
							},
							"included_users": {
								Type:        schema.TypeSet,
								Optional:    true,
								Elem:        &schema.Schema{Type: schema.TypeString},
								Description: "Sets `ycql_audit_included_users`: the users to audit.",
							},
							"excluded_users": {
								Type:        schema.TypeSet,
								Optional:    true,
								Elem:        &schema.Schema{Type: schema.TypeString},
								Description: "Sets `ycql_audit_excluded_users`: the users not to audit.",
							},
						},
					},
				},
				"exporter": {
					Type:     schema.TypeList,
					Optional: true,
					Description: "Telemetry provider that receives the audit logs. " +
						"Repeat the block to send them to more than one telemetry " +
						"provider. A telemetry provider can appear only once in a pipeline.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"exporter_uuid": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "UUID of the telemetry provider.",
							},
							"additional_tags": {
								Type:     schema.TypeMap,
								Optional: true,
								Description: "Tags that YBA adds as attributes to each audit " +
									"log record. A tag here overrides a telemetry provider tag " +
									"with the same key.",
								Elem: &schema.Schema{Type: schema.TypeString},
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
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Description: "Query logging and query log export. Omit the block to turn " +
			"off query logging and its export.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ysql_query_log_config": {
					Type:     schema.TypeList,
					Optional: true,
					MaxItems: 1,
					Description: "YSQL query logging. The block turns on YSQL query " +
						"logging. Omit it to turn YSQL query logging off.",
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
								Description: "Sets `log_statement`: which SQL statements to " +
									"log. Allowed values: " + codeList(allowedQueryLogStatements) +
									". `MOD` logs DDL and data-changing statements. " +
									defaultNote(queryLogDefaults.LogStatement),
							},
							"log_min_error_statement": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  queryLogDefaults.LogMinErrorStatement,
								Description: "Sets `log_min_error_statement`: the lowest " +
									"error severity that logs the statement that caused it. " +
									"The only allowed value is `ERROR`. " +
									defaultNote(queryLogDefaults.LogMinErrorStatement),
							},
							"log_error_verbosity": {
								Type:     schema.TypeString,
								Optional: true,
								Default:  queryLogDefaults.LogErrorVerbosity,
								ValidateFunc: validation.StringInSlice(
									allowedQueryErrorVerbosity,
									false,
								),
								Description: "Sets `log_error_verbosity`: how much detail " +
									"each logged message carries. Allowed values: " +
									codeList(allowedQueryErrorVerbosity) + ". " +
									defaultNote(queryLogDefaults.LogErrorVerbosity),
							},
							"log_duration": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.LogDuration,
								Description: "Sets `log_duration`: log the duration of every " +
									"completed statement. " + defaultNote(queryLogDefaults.LogDuration),
							},
							"debug_print_plan": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.DebugPrintPlan,
								Description: "Sets `debug_print_plan`: log the execution " +
									"plan of every query. " + defaultNote(queryLogDefaults.DebugPrintPlan),
							},
							"log_connections": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.LogConnections,
								Description: "Sets `log_connections`: log each connection " +
									"attempt and each completed client authentication. " +
									defaultNote(queryLogDefaults.LogConnections),
							},
							"log_disconnections": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  queryLogDefaults.LogDisconnections,
								Description: "Sets `log_disconnections`: log each session " +
									"end, with the session duration. " +
									defaultNote(queryLogDefaults.LogDisconnections),
							},
							"log_min_duration_statement": {
								Type:     schema.TypeInt,
								Optional: true,
								Default:  int(queryLogDefaults.LogMinDurationStatement),
								// Bound the top end so the int32 conversion can't wrap.
								ValidateFunc: validation.IntBetween(-1, math.MaxInt32),
								Description: "Sets `log_min_duration_statement`: log each " +
									"statement that runs for at least this many " +
									"milliseconds. `-1` turns this off, and `0` logs every " +
									"statement. " + defaultNote(queryLogDefaults.LogMinDurationStatement),
							},
						},
					},
				},
				"exporter": queryLogsExporterSchema(),
			},
		},
	}
}

func metricsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Description: "Metric export. Omit the block to turn off metric export.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"scrape_interval_seconds": {
					Type:         schema.TypeInt,
					Optional:     true,
					Default:      derefInt32(metricsDefaults.ScrapeIntervalSeconds),
					ValidateFunc: validation.IntBetween(1, math.MaxInt32),
					Description: "Seconds between two scrapes of each target. " +
						defaultNote(derefInt32(metricsDefaults.ScrapeIntervalSeconds)),
				},
				"scrape_timeout_seconds": {
					Type:         schema.TypeInt,
					Optional:     true,
					Default:      derefInt32(metricsDefaults.ScrapeTimeoutSeconds),
					ValidateFunc: validation.IntBetween(1, math.MaxInt32),
					Description: "Timeout of each scrape, in seconds. " +
						defaultNote(derefInt32(metricsDefaults.ScrapeTimeoutSeconds)),
				},
				"collection_level": {
					Type:         schema.TypeString,
					Optional:     true,
					Default:      derefString(metricsDefaults.CollectionLevel),
					ValidateFunc: validation.StringInSlice(allowedCollectionLevels, false),
					Description: "Which metrics to collect: `ALL`, `NORMAL`, `TABLE_OFF` " +
						"(no table-level metrics), `MINIMAL` or `OFF`. " +
						defaultNote(derefString(metricsDefaults.CollectionLevel)),
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
					Description: "Targets to scrape. Allowed values: " +
						codeList(allowedScrapeTargets) + ". When not set, YBA scrapes " +
						"all targets. After you set it, removing the argument keeps the " +
						"current targets. To scrape all targets again, list them all. " +
						"A Kubernetes universe requires this argument.",
				},
				"exporter": metricsExporterSchema(),
			},
		},
	}
}

// batchingDefaults is the shape every batched exporter config in the
// generated client shares (query-log, metric and server-log exporters).
type batchingDefaults interface {
	GetSendBatchMaxSize() int32
	GetSendBatchSize() int32
	GetSendBatchTimeoutSeconds() int32
	GetMemoryLimitMib() int32
	GetMemoryLimitCheckIntervalSeconds() int32
}

// batchingSchema returns the batch processor and memory limiter fields of the
// OpenTelemetry collector pipeline that YBA builds for each exporter, with
// defaults from that exporter's generated client constructor.
func batchingSchema(defaults batchingDefaults) map[string]*schema.Schema {
	// IntBetween(1, MaxInt32) rejects two footguns: an overflowing value
	// (wraps negative in the int32 conversion) and an explicit 0
	// (GetInt32Pointer drops it, so YBA substitutes its default and diffs forever).
	field := func(def int32, description string) *schema.Schema {
		return &schema.Schema{
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      int(def),
			ValidateFunc: validation.IntBetween(1, math.MaxInt32),
			Description:  description + " " + defaultNote(def),
		}
	}
	return map[string]*schema.Schema{
		"send_batch_size": field(defaults.GetSendBatchSize(),
			"Number of records after which the collector sends a batch to this "+
				"exporter, before `send_batch_timeout_seconds` passes."),
		"send_batch_max_size": field(defaults.GetSendBatchMaxSize(),
			"Largest batch, in records, that the collector sends to this exporter. "+
				"The collector splits a larger batch."),
		"send_batch_timeout_seconds": field(defaults.GetSendBatchTimeoutSeconds(),
			"Seconds after which the collector sends a batch, whatever its size."),
		"memory_limit_mib": field(defaults.GetMemoryLimitMib(),
			"Memory limit, in MiB, of the collector's memory limiter for this "+
				"exporter. When memory use comes close to the limit, the collector "+
				"refuses new data."),
		"memory_limit_check_interval_seconds": field(
			defaults.GetMemoryLimitCheckIntervalSeconds(),
			"Seconds between memory-use checks by the memory limiter."),
	}
}

func queryLogsExporterSchema() *schema.Schema {
	s := map[string]*schema.Schema{
		"exporter_uuid": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "UUID of the telemetry provider.",
		},
		"additional_tags": {
			Type:     schema.TypeMap,
			Optional: true,
			Description: "Tags that YBA adds as attributes to each query log " +
				"record. A tag here overrides a telemetry provider tag with the " +
				"same key.",
			Elem: &schema.Schema{Type: schema.TypeString},
		},
	}
	maps.Copy(s, batchingSchema(queryExporterDefaults))
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Description: "Telemetry provider that receives the query logs. Repeat " +
			"the block to send them to more than one telemetry provider. A " +
			"telemetry provider can appear only once in a pipeline.",
		Elem: &schema.Resource{Schema: s},
	}
}

func metricsExporterSchema() *schema.Schema {
	s := map[string]*schema.Schema{
		"exporter_uuid": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "UUID of the telemetry provider.",
		},
		"additional_tags": {
			Type:     schema.TypeMap,
			Optional: true,
			Description: "Tags that YBA adds as attributes to each metric. A tag " +
				"here overrides a telemetry provider tag with the same key.",
			Elem: &schema.Schema{Type: schema.TypeString},
		},
		"metrics_prefix": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Prefix that YBA adds to the name of each metric.",
		},
	}
	maps.Copy(s, batchingSchema(metricExporterDefaults))
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Description: "Telemetry provider that receives the metrics. Repeat the " +
			"block to send them to more than one telemetry provider. A telemetry " +
			"provider can appear only once in a pipeline.",
		Elem: &schema.Resource{Schema: s},
	}
}

// serverLogsElem is the shared Elem of the six server-log pipeline blocks
// (master/tserver/ysql_conn_mgr/node_agent/ynp/controller): a repeatable
// exporter list plus any per-pipeline extra fields.
func serverLogsElem(extra map[string]*schema.Schema) *schema.Resource {
	s := map[string]*schema.Schema{"exporter": serverLogsExporterSchema()}
	for k, v := range extra {
		s[k] = v
	}
	return &schema.Resource{Schema: s}
}

// YBA's Kubernetes rules for the server-log pipelines (ExportType,
// OtelCollectorUtil.supportsOtelConfigPassthrough,
// ExportTelemetryConfigParams.verifyParams). The provider does not manage
// Kubernetes universes, so the docs state these rules and the server enforces
// them.
const (
	vmOnlyLogsNote     = "Not available on a Kubernetes universe."
	kubernetesLogsNote = "On a Kubernetes universe, YBA also requires YugabyteDB " +
		"2026.1.2.0 or later on the universe."
)

func serverLogsSchema(display, platformNote string) *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Description: display + " log export. Omit the block to turn it off. " +
			platformNote,
		Elem: serverLogsElem(nil),
	}
}

func masterLogsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Description: "yb-master log export. Omit the block to turn it off. " +
			kubernetesLogsNote,
		Elem: serverLogsElem(map[string]*schema.Schema{
			"min_level": serverLogMinLevelSchema(
				"yb-master", derefString(masterLogsDefaults.MinLevel), ""),
			"noise_sample_drop_ratio": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      derefFloat64(masterLogsDefaults.NoiseSampleDropRatio),
				ValidateFunc: validation.FloatBetween(0, 1),
				Description: "Fraction, from `0.0` to `1.0`, of the high-volume, " +
					"low-value log lines to drop. `0.0` keeps every line. " +
					defaultNote(derefFloat64(masterLogsDefaults.NoiseSampleDropRatio)),
			},
		}),
	}
}

func tserverLogsSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Description: "yb-tserver log export. Omit the block to turn it off. " +
			kubernetesLogsNote,
		Elem: serverLogsElem(map[string]*schema.Schema{
			"min_level": serverLogMinLevelSchema(
				"yb-tserver", derefString(tserverLogsDefaults.MinLevel),
				" The default is higher than for yb-master, because yb-tserver "+
					"writes many INFO lines."),
		}),
	}
}

func serverLogMinLevelSchema(process, defaultLevel, note string) *schema.Schema {
	return &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		Default:      defaultLevel,
		ValidateFunc: validation.StringInSlice(allowedServerLogLevels, false),
		Description: "Lowest " + process + " log severity to export. YBA drops " +
			"lines below this severity. Allowed values: " +
			codeList(allowedServerLogLevels) + ". " + defaultNote(defaultLevel) + note,
	}
}

func serverLogsExporterSchema() *schema.Schema {
	s := map[string]*schema.Schema{
		"exporter_uuid": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "UUID of the telemetry provider.",
		},
		"additional_tags": {
			Type:     schema.TypeMap,
			Optional: true,
			Description: "Tags that YBA adds as attributes to each log record. A " +
				"tag here overrides a telemetry provider tag with the same key.",
			Elem: &schema.Schema{Type: schema.TypeString},
		},
	}
	maps.Copy(s, batchingSchema(serverLogsExporterDefaults))
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Description: "Telemetry provider that receives the logs. Repeat the " +
			"block to send them to more than one telemetry provider. A telemetry " +
			"provider can appear only once in a pipeline.",
		Elem: &schema.Resource{Schema: s},
	}
}

func customizeUniverseTelemetryDiff(
	ctx context.Context, d *schema.ResourceDiff, meta interface{},
) error {
	if err := validateYBAVersion(ctx, d, meta); err != nil {
		return err
	}
	if err := validateExporters(ctx, d, meta); err != nil {
		return err
	}
	return validateSingleManagerPerUniverse(ctx, d, meta)
}

// telemetryPipelines lists every pipeline block, its exporter path, and the
// YBA build that ships it. The exporter guardrails, the version gate, and the
// claim fingerprint iterate it, so a new pipeline needs exactly one entry here.
var telemetryPipelines = []struct {
	label string
	path  string
	// min is the first YBA build whose API accepts the block; nil means the
	// resource floor (unifiedTelemetryAPIMin) is the only requirement.
	min *utils.YBAMinimumVersion
}{
	{label: "audit_logs", path: "audit_logs.0.exporter"},
	{label: "query_logs", path: "query_logs.0.exporter"},
	{label: "metrics", path: "metrics.0.exporter"},
	{label: "master_logs", path: "master_logs.0.exporter", min: &serverLogPipelinesMin},
	{label: "tserver_logs", path: "tserver_logs.0.exporter", min: &serverLogPipelinesMin},
	{
		label: "ysql_conn_mgr_logs",
		path:  "ysql_conn_mgr_logs.0.exporter",
		min:   &serverLogPipelinesMin,
	},
	{label: "node_agent_logs", path: "node_agent_logs.0.exporter", min: &serverLogPipelinesMin},
	{label: "ynp_logs", path: "ynp_logs.0.exporter", min: &serverLogPipelinesMin},
	{label: "controller_logs", path: "controller_logs.0.exporter", min: &serverLogPipelinesMin},
}

// validateExporters rejects a duplicate or empty exporter_uuid within one
// pipeline. A provider may repeat across pipelines or universes — only
// intra-pipeline duplicates are the mistake. Unknown values are skipped.
func validateExporters(
	_ context.Context, d *schema.ResourceDiff, _ interface{},
) error {
	for _, section := range telemetryPipelines {
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
				"declare exactly one per universe (a single resource's pipeline "+
				"blocks manage every pipeline together)", universeUUID)
	}
	return nil
}

// universeConfigFingerprint identifies the claiming resource by its sorted
// exporter UUIDs. It must be stable across the two CustomizeDiff passes SDKv2 runs
// for a ForceNew diff, so we fingerprint only the user-supplied exporter_uuids —
// scalar defaults and TypeSet internals aren't stable between those passes.
func universeConfigFingerprint(d *schema.ResourceDiff) string {
	var b strings.Builder
	for _, section := range telemetryPipelines {
		uuids := []string{}
		if list, ok := d.Get(section.path).([]interface{}); ok {
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
	if v, ok := d.GetOk("master_logs"); ok {
		if s := buildMasterLogs(v); s != nil {
			tc.MasterLogs = s
		}
	}
	if v, ok := d.GetOk("tserver_logs"); ok {
		if s := buildTserverLogs(v); s != nil {
			tc.TserverLogs = s
		}
	}
	if v, ok := d.GetOk("ysql_conn_mgr_logs"); ok {
		if m := firstMap(v); len(m) > 0 {
			tc.YsqlConnMgrLogs = &clientv2.YsqlConnMgrLogsTelemetrySpec{
				Exporters: buildServerLogsExporters(m["exporter"]),
			}
		}
	}
	if v, ok := d.GetOk("node_agent_logs"); ok {
		if m := firstMap(v); len(m) > 0 {
			tc.NodeAgentLogs = &clientv2.NodeAgentLogsTelemetrySpec{
				Exporters: buildServerLogsExporters(m["exporter"]),
			}
		}
	}
	if v, ok := d.GetOk("ynp_logs"); ok {
		if m := firstMap(v); len(m) > 0 {
			tc.YnpLogs = &clientv2.YnpLogsTelemetrySpec{
				Exporters: buildServerLogsExporters(m["exporter"]),
			}
		}
	}
	if v, ok := d.GetOk("controller_logs"); ok {
		if m := firstMap(v); len(m) > 0 {
			tc.ControllerLogs = &clientv2.ControllerLogsTelemetrySpec{
				Exporters: buildServerLogsExporters(m["exporter"]),
			}
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
	// log_level / log_parameter_max_size became optional in the YBA API. Send
	// them unconditionally anyway: the schema always has a value (Default), and
	// 0 / "" are not "unset" here (utils.GetInt32Pointer would drop a
	// deliberate log_parameter_max_size = 0).
	logLevel := stringValue(m["log_level"])
	logParameterMaxSize := int32Value(m["log_parameter_max_size"])
	return &clientv2.YSQLAuditConfig{
		// enabled is derived server-side from block presence; required field, so set it.
		Enabled:             true,
		Classes:             stringList(m["classes"]),
		LogCatalog:          boolValue(m["log_catalog"]),
		LogClient:           boolValue(m["log_client"]),
		LogLevel:            &logLevel,
		LogParameter:        boolValue(m["log_parameter"]),
		LogParameterMaxSize: &logParameterMaxSize,
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
	// log_level became optional in the YBA API; the schema always has a value
	// (Default), so send it unconditionally to keep the previous behavior.
	logLevel := stringValue(m["log_level"])
	return &clientv2.YCQLAuditConfig{
		// enabled is derived server-side from block presence; required field, so set it.
		Enabled:            true,
		LogLevel:           &logLevel,
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

func buildMasterLogs(in interface{}) *clientv2.MasterLogsTelemetrySpec {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	// Always send both scalars: the schema fills them with defaults, and 0.0 is
	// a deliberate noise_sample_drop_ratio ("keep every line"), so a
	// drop-zero-values pointer helper would silently flip it to the server
	// default of 0.99.
	minLevel := stringValue(m["min_level"])
	noiseRatio := floatValue(m["noise_sample_drop_ratio"])
	return &clientv2.MasterLogsTelemetrySpec{
		Exporters:            buildServerLogsExporters(m["exporter"]),
		MinLevel:             &minLevel,
		NoiseSampleDropRatio: &noiseRatio,
	}
}

func buildTserverLogs(in interface{}) *clientv2.TServerLogsTelemetrySpec {
	m := firstMap(in)
	if len(m) == 0 {
		return nil
	}
	minLevel := stringValue(m["min_level"])
	return &clientv2.TServerLogsTelemetrySpec{
		Exporters: buildServerLogsExporters(m["exporter"]),
		MinLevel:  &minLevel,
	}
}

// buildServerLogsExporters builds the exporter list shared by all six server-log
// pipelines (UniverseServerLogsExporterConfig carries the same batching fields
// as the query-log exporter).
func buildServerLogsExporters(in interface{}) []clientv2.UniverseServerLogsExporterConfig {
	rows := exporterRows(in)
	out := make([]clientv2.UniverseServerLogsExporterConfig, 0, len(rows))
	for _, m := range rows {
		entry := clientv2.UniverseServerLogsExporterConfig{
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
		diag.Diagnostics{previewWarning("yba_universe_telemetry_config")},
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
		diag.Diagnostics{previewWarning("yba_universe_telemetry_config")},
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
		if errors.Is(err, utils.ErrUniverseMissing) {
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
		(c.AuditLogs == nil && c.QueryLogs == nil && c.Metrics == nil &&
			c.MasterLogs == nil && c.TserverLogs == nil &&
			c.YsqlConnMgrLogs == nil && c.NodeAgentLogs == nil &&
			c.YnpLogs == nil && c.ControllerLogs == nil)
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
		if errors.Is(err, utils.ErrUniverseMissing) {
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
	if err := d.Set("master_logs", flattenMasterLogsSpec(config.MasterLogs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("tserver_logs", flattenTserverLogsSpec(config.TserverLogs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ysql_conn_mgr_logs",
		flattenYsqlConnMgrLogsSpec(config.YsqlConnMgrLogs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("node_agent_logs",
		flattenNodeAgentLogsSpec(config.NodeAgentLogs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ynp_logs", flattenYnpLogsSpec(config.YnpLogs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("controller_logs",
		flattenControllerLogsSpec(config.ControllerLogs)); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

// errExportTelemetryRouteMissing marks a 404 from the v2 telemetry config GET.
// YBA reports a gone universe on this route as a 400 ("Cannot find universe"),
// so a 404 means the YBA lacks the route.
var errExportTelemetryRouteMissing = errors.New(
	"YBA does not have the export-telemetry-configs API")

// getExportTelemetryConfig fetches the v2 telemetry config, mapping a gone
// universe to utils.ErrUniverseMissing, a missing route to
// errExportTelemetryRouteMissing, and other errors to a formatted error. The
// config resource surfaces a missing route as an error rather than dropping a
// live config from state.
func getExportTelemetryConfig(
	ctx context.Context, apiClient *api.APIClient, universeUUID, operation string,
) (*clientv2.TelemetryConfig, error) {
	config, response, err := apiClient.YugawareClientV2.UniverseAPI.
		GetExportTelemetryConfig(ctx, apiClient.CustomerID, universeUUID).Execute()
	if err != nil {
		httpErr := utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Universe Telemetry Config", operation)
		if utils.IsHTTPNotFound(response) {
			return nil, fmt.Errorf("%w: %w", errExportTelemetryRouteMissing, httpErr)
		}
		if utils.IsUniverseMissing(response, err) {
			return nil, utils.ErrUniverseMissing
		}
		return nil, httpErr
	}
	return config, nil
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

// boolValue reads a JSON boolean and nothing else: telemetry configs never
// carry booleans as strings, and TestHelpersTolerateWeirdInput pins that.
// (utils.BoolValue also accepts "true", for YBA settings the UI stores as
// strings.)
func boolValue(in interface{}) bool {
	if v, ok := in.(bool); ok {
		return v
	}
	return false
}

func floatValue(in interface{}) float64 {
	if v, ok := in.(float64); ok {
		return v
	}
	return 0
}

func int32Value(in interface{}) int32 {
	return int32(intValue(in))
}
