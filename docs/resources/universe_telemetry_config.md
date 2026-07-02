---
page_title: "yba_universe_telemetry_config Resource - YugabyteDB Anywhere"
description: |-
  ~> Experimental: This resource wraps a YugabyteDB Anywhere telemetry export API that is still experimental and may change in backward-incompatible ways across YBA releases. Pin your provider version and review release notes before upgrading.
  Universe Telemetry Config Resource. Attaches audit log, query log, and metrics export pipelines to a YBA universe via the unified export-telemetry-configs API. Each exporter references a yba_telemetry_provider (or any pre-existing telemetry provider UUID) and triggers a rolling/non-rolling restart of the universe to install or update the OpenTelemetry collector.
  ~> Note: OTLP-based exporters require the global runtime config yb.telemetry.allow_otlp to be set to true. Manage that with the yba_runtime_config resource.
  ~> Note: Import an existing universe-level configuration with the universe UUID as the resource ID (terraform import yba_universe_telemetry_config.example <universe-uuid>); state is populated from the unified export-telemetry-configs GET API.
  ~> One resource per universe: YBA stores a single telemetry configuration per universe and this resource owns it wholesale — Terraform is the source of truth. On apply it replaces whatever the universe currently has (including anything configured out-of-band in the YBA UI), so manage all three pipelines (audit_logs, query_logs, metrics) from a single yba_universe_telemetry_config block. Declaring two resources for the same universe_uuid is rejected at plan time (they would otherwise overwrite each other on every apply). On destroy the resource disables every exporter on the universe, but only if a configuration still exists server-side — an already-empty universe is left untouched.
  ~> Dependency Note: When exporter_uuid is wired through a reference like yba_telemetry_provider.x.id, Terraform's dependency graph automatically orders create / replace / destroy of the provider before this resource — there is no need to add an explicit depends_on. The provider's own destroy step also proactively detaches itself from every referencing universe before deletion, so a plan that destroys-and-recreates a provider in the same apply is safe.
---

# yba_universe_telemetry_config (Resource)

~> **Experimental:** This resource wraps a YugabyteDB Anywhere telemetry export API that is still experimental and may change in backward-incompatible ways across YBA releases. Pin your provider version and review release notes before upgrading.

Universe Telemetry Config Resource. Attaches audit log, query log, and metrics export pipelines to a YBA universe via the unified `export-telemetry-configs` API. Each exporter references a `yba_telemetry_provider` (or any pre-existing telemetry provider UUID) and triggers a rolling/non-rolling restart of the universe to install or update the OpenTelemetry collector.

~> **Note:** OTLP-based exporters require the global runtime config `yb.telemetry.allow_otlp` to be set to `true`. Manage that with the `yba_runtime_config` resource.

~> **Note:** Import an existing universe-level configuration with the universe UUID as the resource ID (`terraform import yba_universe_telemetry_config.example <universe-uuid>`); state is populated from the unified `export-telemetry-configs` GET API.

~> **One resource per universe:** YBA stores a single telemetry configuration per universe and this resource owns it wholesale — Terraform is the source of truth. On apply it **replaces** whatever the universe currently has (including anything configured out-of-band in the YBA UI), so manage all three pipelines (`audit_logs`, `query_logs`, `metrics`) from a **single** `yba_universe_telemetry_config` block. Declaring two resources for the same `universe_uuid` is rejected at plan time (they would otherwise overwrite each other on every apply). On destroy the resource disables every exporter on the universe, but only if a configuration still exists server-side — an already-empty universe is left untouched.

~> **Dependency Note:** When `exporter_uuid` is wired through a reference like `yba_telemetry_provider.x.id`, Terraform's dependency graph automatically orders create / replace / destroy of the provider before this resource — there is **no need to add an explicit `depends_on`**. The provider's own destroy step also proactively detaches itself from every referencing universe before deletion, so a plan that destroys-and-recreates a provider in the same apply is safe.

## Example Usage

YBA stores a single telemetry configuration per universe, so manage all three pipelines (`audit_logs`, `query_logs`, `metrics`) from **one** `yba_universe_telemetry_config` resource. Declaring a second resource for the same `universe_uuid` is rejected at plan time, because the two would overwrite each other on every apply.

```terraform
resource "yba_universe_telemetry_config" "main" {
  universe_uuid = yba_universe.main.id

  audit_logs {
    ysql_audit_config {
      classes                = ["READ", "WRITE", "FUNCTION", "ROLE", "DDL", "MISC", "MISC_SET"]
      log_catalog            = true
      log_client             = true
      log_level              = "WARNING"
      log_parameter          = true
      log_parameter_max_size = 4096
      log_relation           = true
      log_rows               = true
      log_statement          = true
      log_statement_once     = true
    }

    ycql_audit_config {
      log_level           = "WARNING"
      included_categories = ["QUERY", "DML", "DDL", "DCL", "AUTH", "PREPARE", "ERROR", "OTHER"]
    }

    exporter {
      exporter_uuid = yba_telemetry_provider.datadog.id
      additional_tags = {
        query_logs_key = yba_universe.main.name
      }
    }
  }

  query_logs {
    ysql_query_log_config {
      log_statement              = "ALL"
      log_min_error_statement    = "ERROR"
      log_error_verbosity        = "VERBOSE"
      log_duration               = false
      debug_print_plan           = false
      log_connections            = true
      log_disconnections         = true
      log_min_duration_statement = -1
    }

    exporter {
      exporter_uuid              = yba_telemetry_provider.datadog.id
      send_batch_max_size        = 1000
      send_batch_size            = 100
      send_batch_timeout_seconds = 10
      memory_limit_mib           = 2048
    }
  }

  metrics {
    scrape_interval_seconds = 301
    scrape_timeout_seconds  = 60
    collection_level        = "NORMAL"
    scrape_config_targets = [
      "MASTER_EXPORT",
      "TSERVER_EXPORT",
      "YSQL_EXPORT",
      "CQL_EXPORT",
      "NODE_EXPORT",
      "NODE_AGENT_EXPORT",
      "OTEL_EXPORT",
    ]

    # Repeat the exporter block per destination — each becomes one entry in the
    # API's exporters array (metrics here fan out to both Prometheus and Datadog).
    exporter {
      exporter_uuid = yba_telemetry_provider.prometheus.id
      additional_tags = {
        metrics_key = "muthu"
      }
      send_batch_max_size        = 1000
      send_batch_size            = 100
      send_batch_timeout_seconds = 60
      memory_limit_mib           = 2048
      metrics_prefix             = "ybdb."
    }
    exporter {
      exporter_uuid  = yba_telemetry_provider.datadog.id
      metrics_prefix = "yba."
    }
  }

  upgrade_options {
    rolling_upgrade = false
  }
}
```

### Multiple exporters per pipeline

To fan a pipeline out to multiple telemetry destinations, repeat its `exporter` block **within that same resource** — each block becomes one entry in the API's `exporters` array. The `metrics` pipeline in the example above does exactly this, shipping to both Prometheus and Datadog from the single resource:

```terraform
resource "yba_universe_telemetry_config" "main" {
  universe_uuid = yba_universe.main.id

  metrics {
    # Each exporter block adds one destination; do NOT add a second
    # yba_universe_telemetry_config resource for the same universe.
    exporter {
      exporter_uuid  = yba_telemetry_provider.prometheus.id
      metrics_prefix = "ybdb."
    }
    exporter {
      exporter_uuid  = yba_telemetry_provider.datadog.id
      metrics_prefix = "yba."
    }
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `universe_uuid` (String) UUID of the universe whose telemetry pipelines are managed.

### Optional

- `audit_logs` (Block List, Max: 1) Audit log export configuration. Omit to disable audit log export. (see [below for nested schema](#nestedblock--audit_logs))
- `metrics` (Block List, Max: 1) Metric export configuration. Omit to disable metric export. (see [below for nested schema](#nestedblock--metrics))
- `query_logs` (Block List, Max: 1) Query log export configuration. Omit to disable query log export. (see [below for nested schema](#nestedblock--query_logs))
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `upgrade_options` (Block List, Max: 1) Optional rolling-restart options applied while reconfiguring the universe.

~> **Performance Note:** The `sleep_after_*_restart_millis` defaults of 180000 (3 minutes) are applied per node. A 9-node universe therefore spends ~27 minutes just sleeping between restarts on top of the actual restart work. Lower these values for faster reconfigures on healthy clusters, or raise them for clusters under heavy traffic. (see [below for nested schema](#nestedblock--upgrade_options))

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--audit_logs"></a>

### Nested Schema for `audit_logs`

Optional:

- `exporter` (Block List) Exporter (telemetry destination) for audit logs. Repeat this block to fan out to multiple destinations — each block becomes one entry in the API's `exporters` array. (see [below for nested schema](#nestedblock--audit_logs--exporter))
- `ycql_audit_config` (Block List, Max: 1) YCQL audit logging configuration. Declaring this block enables YCQL audit logging — YBA derives `enabled` from the block's presence, so there is no `enabled` field; omit the block to disable. `log_level`'s `Default` is a **provider default** (the YBA API requires the field but defines no default). (see [below for nested schema](#nestedblock--audit_logs--ycql_audit_config))
- `ysql_audit_config` (Block List, Max: 1) YSQL audit (pgaudit) logging configuration. Declaring this block enables YSQL audit logging on the universe — YBA derives `enabled` from the block's presence, so there is no `enabled` field; omit the block to disable. The YBA API marks every field below `required` with no server default, so the `Default` values are **provider defaults** chosen to mirror the YBA UI. (see [below for nested schema](#nestedblock--audit_logs--ysql_audit_config))

<a id="nestedblock--audit_logs--exporter"></a>

### Nested Schema for `audit_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider to send audit logs to.

Optional:

- `additional_tags` (Map of String) Additional string tags appended to each audit log record.

<a id="nestedblock--audit_logs--ycql_audit_config"></a>

### Nested Schema for `audit_logs.ycql_audit_config`

Optional:

- `excluded_categories` (Set of String)
- `excluded_keyspaces` (Set of String)
- `excluded_users` (Set of String)
- `included_categories` (Set of String)
- `included_keyspaces` (Set of String)
- `included_users` (Set of String)
- `log_level` (String)

<a id="nestedblock--audit_logs--ysql_audit_config"></a>

### Nested Schema for `audit_logs.ysql_audit_config`

Optional:

- `classes` (Set of String) YSQL audit log classes (e.g. READ, WRITE, DDL, ROLE).
- `log_catalog` (Boolean)
- `log_client` (Boolean)
- `log_level` (String)
- `log_parameter` (Boolean)
- `log_parameter_max_size` (Number)
- `log_relation` (Boolean)
- `log_rows` (Boolean)
- `log_statement` (Boolean)
- `log_statement_once` (Boolean)

<a id="nestedblock--metrics"></a>

### Nested Schema for `metrics`

Optional:

- `collection_level` (String)
- `exporter` (Block List) Metric exporter (telemetry destination). Repeat this block to send metrics to multiple destinations — each becomes one entry in the API's `exporters` array. (see [below for nested schema](#nestedblock--metrics--exporter))
- `scrape_config_targets` (Set of String) Scrape target types to include. Omit to let YBA include all supported targets.
- `scrape_interval_seconds` (Number)
- `scrape_timeout_seconds` (Number)

<a id="nestedblock--metrics--exporter"></a>

### Nested Schema for `metrics.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider that receives the metric data.

Optional:

- `additional_tags` (Map of String) Additional string tags appended to each metric.
- `memory_limit_check_interval_seconds` (Number)
- `memory_limit_mib` (Number)
- `metrics_prefix` (String) Optional prefix prepended to every metric name.
- `send_batch_max_size` (Number)
- `send_batch_size` (Number)
- `send_batch_timeout_seconds` (Number)

<a id="nestedblock--query_logs"></a>

### Nested Schema for `query_logs`

Optional:

- `exporter` (Block List) Exporter (telemetry destination). Repeat this block to send to multiple destinations — each becomes one entry in the API's `exporters` array. (see [below for nested schema](#nestedblock--query_logs--exporter))
- `ysql_query_log_config` (Block List, Max: 1) YSQL query logging configuration. Declaring this block enables YSQL query logging — YBA derives `enabled` from the block's presence, so there is no `enabled` field; omit the block to disable. `Default` values are sourced from the YBA API's own `default:` (via the generated client) so they track the server. (see [below for nested schema](#nestedblock--query_logs--ysql_query_log_config))

<a id="nestedblock--query_logs--exporter"></a>

### Nested Schema for `query_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider that receives the data.

Optional:

- `additional_tags` (Map of String) Additional string tags appended to each record.
- `memory_limit_check_interval_seconds` (Number)
- `memory_limit_mib` (Number)
- `send_batch_max_size` (Number)
- `send_batch_size` (Number)
- `send_batch_timeout_seconds` (Number)

<a id="nestedblock--query_logs--ysql_query_log_config"></a>

### Nested Schema for `query_logs.ysql_query_log_config`

Optional:

- `debug_print_plan` (Boolean)
- `log_connections` (Boolean)
- `log_disconnections` (Boolean)
- `log_duration` (Boolean)
- `log_error_verbosity` (String)
- `log_min_duration_statement` (Number)
- `log_min_error_statement` (String)
- `log_statement` (String)

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `read` (String)
- `update` (String)

<a id="nestedblock--upgrade_options"></a>

### Nested Schema for `upgrade_options`

Optional:

- `rolling_upgrade` (Boolean) Perform a rolling restart (default true). Set to false to restart all nodes at once.
- `sleep_after_master_restart_millis` (Number) Sleep between master restarts (ms). Defaults to 180000 (3 minutes).
- `sleep_after_tserver_restart_millis` (Number) Sleep between tserver restarts (ms). Defaults to 180000 (3 minutes).
