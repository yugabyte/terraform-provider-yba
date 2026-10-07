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
