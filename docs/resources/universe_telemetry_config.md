---
page_title: "yba_universe_telemetry_config Resource - YugabyteDB Anywhere"
description: |-
  Manages the telemetry export configuration of a universe in YugabyteDB Anywhere: the audit log, query log, server log and metrics pipelines, and the telemetry providers that each pipeline sends data to.
---

# yba_universe_telemetry_config (Resource)

Manages the telemetry export configuration of a universe in YugabyteDB Anywhere: the audit log, query log, server log and metrics pipelines, and the telemetry providers that each pipeline sends data to.

~> **Experimental:** Telemetry export is an experimental feature of YugabyteDB Anywhere. A later YBA release can change it in ways that are not backward compatible. Read the release notes before you upgrade YBA or the provider.

YBA runs an OpenTelemetry Collector on the universe nodes to export the data.

~> **Note:** This resource requires YugabyteDB Anywhere 2026.1.0.0 or later. `terraform plan` fails on an older YBA. The server-log pipelines (`master_logs`, `tserver_logs`, `ysql_conn_mgr_logs`, `node_agent_logs`, `ynp_logs` and `controller_logs`) require YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA.

~> **Note:** Each create and each update restarts the yb-master and yb-tserver processes on every node of the universe. By default, YBA restarts one server at a time and waits 3 minutes after each restart, so a universe with 3 masters and 9 tservers waits 36 minutes in total. Use `upgrade_options` to change the wait or to restart all nodes at once. Destroy turns off every pipeline on the universe, which also restarts it. If the universe has no telemetry configuration at that time, destroy only removes the resource from the state.

~> **Note:** YBA stores one telemetry configuration per universe, and this resource replaces all of it on each apply. A pipeline that the configuration does not declare is turned off, also when it was set up in the YBA UI. Manage all pipelines of a universe in one `yba_universe_telemetry_config` resource. The provider rejects a second resource for the same `universe_uuid` in one configuration at plan time. It cannot detect a second resource in another Terraform state.

~> **Note:** Datadog and OTLP telemetry providers accept logs and metrics. Dynatrace accepts only metrics. AWS CloudWatch, GCP Cloud Monitoring, Splunk and S3 accept only logs. All AWS CloudWatch and S3 telemetry providers that one universe uses must have the same access key and secret key. All GCP Cloud Monitoring telemetry providers that one universe uses must have the same credentials.

~> **Note:** On a Kubernetes universe, `node_agent_logs` and `ynp_logs` are not available. Audit log export needs YugabyteDB 2025.1.0.0 or later on the universe, and the other pipelines need YugabyteDB 2026.1.2.0 or later. `metrics` must set `scrape_config_targets` to a subset of `MASTER_EXPORT`, `TSERVER_EXPORT`, `YSQL_EXPORT`, `CQL_EXPORT` and `OTEL_EXPORT`.

~> **Note:** When `exporter_uuid` refers to a telemetry provider resource, Terraform orders the work itself, and `depends_on` is not needed. When an apply replaces a telemetry provider that this universe uses, the universe restarts twice. The first restart removes the old telemetry provider before Terraform deletes it. A pipeline whose only exporter was the old telemetry provider stays off until the second restart, which adds the new telemetry provider.

## Example Usage

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
      included_categories = ["DDL", "DCL", "AUTH", "ERROR"]
      excluded_categories = ["QUERY", "DML"]
      included_keyspaces  = ["app"]
      excluded_keyspaces  = ["app_staging"]
      included_users      = ["app_admin"]
      excluded_users      = ["app_monitor"]
    }

    exporter {
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
      additional_tags = {
        universe = yba_universe.main.name
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
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
      additional_tags = {
        universe = yba_universe.main.name
      }
      send_batch_max_size                 = 1000
      send_batch_size                     = 100
      send_batch_timeout_seconds          = 10
      memory_limit_mib                    = 2048
      memory_limit_check_interval_seconds = 10
    }
  }

  metrics {
    scrape_interval_seconds = 60
    scrape_timeout_seconds  = 30
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

    # Repeat the exporter block for each telemetry provider. Here, metrics go
    # to both Prometheus and Datadog.
    exporter {
      exporter_uuid = yba_otlp_telemetry_provider.prometheus.id
      additional_tags = {
        env = "prod"
      }
      send_batch_max_size                 = 1000
      send_batch_size                     = 100
      send_batch_timeout_seconds          = 60
      memory_limit_mib                    = 2048
      memory_limit_check_interval_seconds = 10
      metrics_prefix                      = "ybdb."
    }
    exporter {
      exporter_uuid  = yba_datadog_telemetry_provider.datadog.id
      metrics_prefix = "yba."
    }
  }

  # Server-log pipelines export the logs of the database processes. They
  # accept the same telemetry providers as audit_logs and query_logs.
  # min_level sets the lowest severity to export. master_logs can also drop
  # a fraction of high-volume, low-value lines.
  master_logs {
    min_level               = "INFO"
    noise_sample_drop_ratio = 0.99

    exporter {
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
      additional_tags = {
        log_type = "yb-master"
      }
      send_batch_max_size                 = 1000
      send_batch_size                     = 100
      send_batch_timeout_seconds          = 10
      memory_limit_mib                    = 2048
      memory_limit_check_interval_seconds = 10
    }
  }

  tserver_logs {
    # The default is WARNING, because yb-tserver writes many INFO lines.
    min_level = "WARNING"

    exporter {
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
    }
  }

  # The other server-log pipelines have only exporter blocks.
  ysql_conn_mgr_logs {
    exporter {
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
    }
  }

  # node_agent_logs and ynp_logs are for VM universes only.
  node_agent_logs {
    exporter {
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
    }
  }

  ynp_logs {
    exporter {
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
    }
  }

  controller_logs {
    exporter {
      exporter_uuid = yba_datadog_telemetry_provider.datadog.id
    }
  }

  # Restart one server at a time, and wait 1 minute after each restart.
  upgrade_options {
    rolling_upgrade                    = true
    sleep_after_master_restart_millis  = 60000
    sleep_after_tserver_restart_millis = 60000
  }
}
```

### Multiple exporters per pipeline

To send a pipeline to more than one telemetry provider, repeat its `exporter`
block in the same resource. Do not add a second `yba_universe_telemetry_config`
resource for the universe. The `metrics` block of the example above sends
metrics to Prometheus and to Datadog:

```terraform
resource "yba_universe_telemetry_config" "main" {
  universe_uuid = yba_universe.main.id

  metrics {
    # Each exporter block adds one telemetry provider.
    exporter {
      exporter_uuid  = yba_otlp_telemetry_provider.prometheus.id
      metrics_prefix = "ybdb."
    }
    exporter {
      exporter_uuid  = yba_datadog_telemetry_provider.datadog.id
      metrics_prefix = "yba."
    }
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `universe_uuid` (String) UUID of the universe. A change replaces the resource.

### Optional

- `audit_logs` (Block List, Max: 1) Audit logging and audit log export. Omit the block to turn off audit logging and its export. (see [below for nested schema](#nestedblock--audit_logs))
- `controller_logs` (Block List, Max: 1) YB-Controller log export. Omit the block to turn it off. Requires YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA. On a Kubernetes universe, YBA also requires YugabyteDB 2026.1.2.0 or later on the universe. (see [below for nested schema](#nestedblock--controller_logs))
- `master_logs` (Block List, Max: 1) yb-master log export. Omit the block to turn it off. Requires YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA. On a Kubernetes universe, YBA also requires YugabyteDB 2026.1.2.0 or later on the universe. (see [below for nested schema](#nestedblock--master_logs))
- `metrics` (Block List, Max: 1) Metric export. Omit the block to turn off metric export. (see [below for nested schema](#nestedblock--metrics))
- `node_agent_logs` (Block List, Max: 1) Node agent log export. Omit the block to turn it off. Requires YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA. Not available on a Kubernetes universe. (see [below for nested schema](#nestedblock--node_agent_logs))
- `query_logs` (Block List, Max: 1) Query logging and query log export. Omit the block to turn off query logging and its export. (see [below for nested schema](#nestedblock--query_logs))
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `tserver_logs` (Block List, Max: 1) yb-tserver log export. Omit the block to turn it off. Requires YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA. On a Kubernetes universe, YBA also requires YugabyteDB 2026.1.2.0 or later on the universe. (see [below for nested schema](#nestedblock--tserver_logs))
- `upgrade_options` (Block List, Max: 1) Options for the restart that applies each change. YBA uses them only with a change to a pipeline, and rejects an apply that changes only `upgrade_options`. Lower the wait times to apply a change faster on a universe with a light load, and raise them for a universe under heavy load. (see [below for nested schema](#nestedblock--upgrade_options))
- `ynp_logs` (Block List, Max: 1) YNP (node provisioning) log export. Omit the block to turn it off. Requires YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA. Not available on a Kubernetes universe. (see [below for nested schema](#nestedblock--ynp_logs))
- `ysql_conn_mgr_logs` (Block List, Max: 1) YSQL Connection Manager log export. Omit the block to turn it off. Requires YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA. On a Kubernetes universe, YBA also requires YugabyteDB 2026.1.2.0 or later on the universe. (see [below for nested schema](#nestedblock--ysql_conn_mgr_logs))

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--audit_logs"></a>

### Nested Schema for `audit_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the audit logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--audit_logs--exporter))
- `ycql_audit_config` (Block List, Max: 1) YCQL audit logging. The block turns on YCQL audit logging. Omit it to turn YCQL audit logging off. (see [below for nested schema](#nestedblock--audit_logs--ycql_audit_config))
- `ysql_audit_config` (Block List, Max: 1) YSQL audit logging, through the pgaudit extension. The block turns on YSQL audit logging. Omit it to turn YSQL audit logging off. (see [below for nested schema](#nestedblock--audit_logs--ysql_audit_config))

<a id="nestedblock--audit_logs--exporter"></a>

### Nested Schema for `audit_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each audit log record. A tag here overrides a telemetry provider tag with the same key.

<a id="nestedblock--audit_logs--ycql_audit_config"></a>

### Nested Schema for `audit_logs.ycql_audit_config`

Optional:

- `excluded_categories` (Set of String) Sets `ycql_audit_excluded_categories`: the statement categories not to audit. Allowed values: `QUERY`, `DML`, `DDL`, `DCL`, `AUTH`, `PREPARE`, `ERROR` or `OTHER`.
- `excluded_keyspaces` (Set of String) Sets `ycql_audit_excluded_keyspaces`: the keyspaces not to audit.
- `excluded_users` (Set of String) Sets `ycql_audit_excluded_users`: the users not to audit.
- `included_categories` (Set of String) Sets `ycql_audit_included_categories`: the statement categories to audit. Allowed values: `QUERY`, `DML`, `DDL`, `DCL`, `AUTH`, `PREPARE`, `ERROR` or `OTHER`.
- `included_keyspaces` (Set of String) Sets `ycql_audit_included_keyspaces`: the keyspaces to audit.
- `included_users` (Set of String) Sets `ycql_audit_included_users`: the users to audit.
- `log_level` (String) Sets `ycql_audit_log_level`: the severity of audit records, which selects the yb-tserver log file they go to. Allowed values: `INFO`, `WARNING` or `ERROR`. Defaults to `WARNING`.

<a id="nestedblock--audit_logs--ysql_audit_config"></a>

### Nested Schema for `audit_logs.ysql_audit_config`

Optional:

- `classes` (Set of String) Sets `pgaudit.log`: the classes of statements to log. Allowed values: `READ`, `WRITE`, `FUNCTION`, `ROLE`, `DDL`, `MISC` or `MISC_SET`.
- `log_catalog` (Boolean) Sets `pgaudit.log_catalog`: also log statements whose relations are all in `pg_catalog`. Set to `false` to drop the catalog lookups that tools make. Defaults to `true`.
- `log_client` (Boolean) Sets `pgaudit.log_client`: also send audit messages to the client, such as ysqlsh. Defaults to `true`.
- `log_level` (String) Sets `pgaudit.log_level`: the severity of the audit messages sent to the client. Applies only when `log_client` is `true`. Allowed values: `DEBUG1`, `DEBUG2`, `DEBUG3`, `DEBUG4`, `DEBUG5`, `INFO`, `NOTICE`, `WARNING` or `LOG`. Defaults to `LOG`.
- `log_parameter` (Boolean) Sets `pgaudit.log_parameter`: include the statement parameters in the audit log. Defaults to `false`.
- `log_parameter_max_size` (Number) Sets `pgaudit.log_parameter_max_size`: the largest parameter, in bytes, to log when `log_parameter` is `true`. A longer parameter is replaced with `<long param suppressed>`. `0` logs every parameter. Defaults to `0`.
- `log_relation` (Boolean) Sets `pgaudit.log_relation`: write a separate entry for each relation that a SELECT or DML statement references. Defaults to `false`.
- `log_rows` (Boolean) Sets `pgaudit.log_rows`: include the number of rows that the statement retrieved or changed. Defaults to `false`.
- `log_statement` (Boolean) Sets `pgaudit.log_statement`: include the statement text and parameters. Defaults to `true`.
- `log_statement_once` (Boolean) Sets `pgaudit.log_statement_once`: include the statement text and parameters only in the first entry for a statement or sub-statement. Defaults to `false`.

<a id="nestedblock--controller_logs"></a>

### Nested Schema for `controller_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--controller_logs--exporter))

<a id="nestedblock--controller_logs--exporter"></a>

### Nested Schema for `controller_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each log record. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

<a id="nestedblock--master_logs"></a>

### Nested Schema for `master_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--master_logs--exporter))
- `min_level` (String) Lowest yb-master log severity to export. YBA drops lines below this severity. Allowed values: `INFO`, `WARNING`, `ERROR` or `FATAL`. Defaults to `INFO`.
- `noise_sample_drop_ratio` (Number) Fraction, from `0.0` to `1.0`, of the high-volume, low-value log lines to drop. `0.0` keeps every line. Defaults to `0.99`.

<a id="nestedblock--master_logs--exporter"></a>

### Nested Schema for `master_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each log record. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

<a id="nestedblock--metrics"></a>

### Nested Schema for `metrics`

Optional:

- `collection_level` (String) Which metrics to collect: `ALL`, `NORMAL`, `TABLE_OFF` (no table-level metrics), `MINIMAL` or `OFF`. Defaults to `NORMAL`.
- `exporter` (Block List) Telemetry provider that receives the metrics. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--metrics--exporter))
- `scrape_config_targets` (Set of String) Targets to scrape. Allowed values: `MASTER_EXPORT`, `TSERVER_EXPORT`, `YSQL_EXPORT`, `CQL_EXPORT`, `NODE_EXPORT`, `NODE_AGENT_EXPORT` or `OTEL_EXPORT`. When not set, YBA scrapes all targets. A Kubernetes universe requires this argument.
- `scrape_interval_seconds` (Number) Seconds between two scrapes of each target. Defaults to `30`.
- `scrape_timeout_seconds` (Number) Timeout of each scrape, in seconds. Defaults to `20`.

<a id="nestedblock--metrics--exporter"></a>

### Nested Schema for `metrics.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each metric. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `metrics_prefix` (String) Prefix that YBA adds to the name of each metric.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

<a id="nestedblock--node_agent_logs"></a>

### Nested Schema for `node_agent_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--node_agent_logs--exporter))

<a id="nestedblock--node_agent_logs--exporter"></a>

### Nested Schema for `node_agent_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each log record. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

<a id="nestedblock--query_logs"></a>

### Nested Schema for `query_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the query logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--query_logs--exporter))
- `ysql_query_log_config` (Block List, Max: 1) YSQL query logging. The block turns on YSQL query logging. Omit it to turn YSQL query logging off. (see [below for nested schema](#nestedblock--query_logs--ysql_query_log_config))

<a id="nestedblock--query_logs--exporter"></a>

### Nested Schema for `query_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each query log record. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

<a id="nestedblock--query_logs--ysql_query_log_config"></a>

### Nested Schema for `query_logs.ysql_query_log_config`

Optional:

- `debug_print_plan` (Boolean) Sets `debug_print_plan`: log the execution plan of every query. Defaults to `false`.
- `log_connections` (Boolean) Sets `log_connections`: log each connection attempt and each completed client authentication. Defaults to `false`.
- `log_disconnections` (Boolean) Sets `log_disconnections`: log each session end, with the session duration. Defaults to `false`.
- `log_duration` (Boolean) Sets `log_duration`: log the duration of every completed statement. Defaults to `false`.
- `log_error_verbosity` (String) Sets `log_error_verbosity`: how much detail each logged message carries. Allowed values: `VERBOSE`, `TERSE` or `DEFAULT`. Defaults to `DEFAULT`.
- `log_min_duration_statement` (Number) Sets `log_min_duration_statement`: log each statement that runs for at least this many milliseconds. `-1` turns this off, and `0` logs every statement. Defaults to `-1`.
- `log_min_error_statement` (String) Sets `log_min_error_statement`: the lowest error severity that logs the statement that caused it. Defaults to `ERROR`.
- `log_statement` (String) Sets `log_statement`: which SQL statements to log. Allowed values: `ALL`, `NONE`, `DDL` or `MOD`. `MOD` logs DDL and data-changing statements. Defaults to `NONE`.

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `read` (String)
- `update` (String)

<a id="nestedblock--tserver_logs"></a>

### Nested Schema for `tserver_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--tserver_logs--exporter))
- `min_level` (String) Lowest yb-tserver log severity to export. YBA drops lines below this severity. Allowed values: `INFO`, `WARNING`, `ERROR` or `FATAL`. Defaults to `WARNING`. The default is higher than for yb-master, because yb-tserver writes many INFO lines.

<a id="nestedblock--tserver_logs--exporter"></a>

### Nested Schema for `tserver_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each log record. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

<a id="nestedblock--upgrade_options"></a>

### Nested Schema for `upgrade_options`

Optional:

- `rolling_upgrade` (Boolean) Restart one server at a time. Set to `false` to restart all nodes at once, which makes the universe unavailable during the restart. Defaults to `true`.
- `sleep_after_master_restart_millis` (Number) Time, in milliseconds, that YBA waits after it restarts each yb-master. Defaults to `180000` (3 minutes). `0` uses the YBA runtime config `yb.upgrade.sleep_after_master_restart_ms`.
- `sleep_after_tserver_restart_millis` (Number) Time, in milliseconds, that YBA waits after it restarts each yb-tserver. Defaults to `180000` (3 minutes). `0` uses the YBA runtime config `yb.upgrade.sleep_after_tserver_restart_ms`.

<a id="nestedblock--ynp_logs"></a>

### Nested Schema for `ynp_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--ynp_logs--exporter))

<a id="nestedblock--ynp_logs--exporter"></a>

### Nested Schema for `ynp_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each log record. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

<a id="nestedblock--ysql_conn_mgr_logs"></a>

### Nested Schema for `ysql_conn_mgr_logs`

Optional:

- `exporter` (Block List) Telemetry provider that receives the logs. Repeat the block to send them to more than one telemetry provider. A telemetry provider can appear only once in a pipeline. (see [below for nested schema](#nestedblock--ysql_conn_mgr_logs--exporter))

<a id="nestedblock--ysql_conn_mgr_logs--exporter"></a>

### Nested Schema for `ysql_conn_mgr_logs.exporter`

Required:

- `exporter_uuid` (String) UUID of the telemetry provider.

Optional:

- `additional_tags` (Map of String) Tags that YBA adds as attributes to each log record. A tag here overrides a telemetry provider tag with the same key.
- `memory_limit_check_interval_seconds` (Number) Seconds between memory-use checks by the memory limiter. Defaults to `10`.
- `memory_limit_mib` (Number) Memory limit, in MiB, of the collector's memory limiter for this exporter. When memory use comes close to the limit, the collector refuses new data. Defaults to `2048`.
- `send_batch_max_size` (Number) Largest batch, in records, that the collector sends to this exporter. The collector splits a larger batch. Defaults to `1000`.
- `send_batch_size` (Number) Number of records after which the collector sends a batch to this exporter, before `send_batch_timeout_seconds` passes. Defaults to `100`.
- `send_batch_timeout_seconds` (Number) Seconds after which the collector sends a batch, whatever its size. Defaults to `10`.

## Import

Import the telemetry configuration of a universe with the universe UUID:

```sh
terraform import yba_universe_telemetry_config.main <universe-uuid>
```

Import reads every pipeline. It does not read `upgrade_options`, because YBA
does not return them. If the configuration sets `upgrade_options`, the first
plan after import shows an update that adds them, and YBA rejects that update
because no pipeline changes. To get a clean plan after import, leave
`upgrade_options` out of the configuration until the next change to a pipeline.
