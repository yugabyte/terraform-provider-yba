resource "yba_universe_telemetry_config" "main" {
  universe_uuid = yba_universe.main.id

  audit_logs {
    ysql_audit_config {
      enabled                = true
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
      enabled             = true
      log_level           = "WARNING"
      included_categories = ["QUERY", "DML", "DDL", "DCL", "AUTH", "PREPARE", "ERROR", "OTHER"]
    }

    exporter {
      exporter_uuid = yba_telemetry_provider.datadog.id
      additional_tags = {
        query_logs_key = "mchidambaram"
      }
    }
  }

  query_logs {
    ysql_query_log_config {
      enabled                    = true
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
  }

  upgrade_options {
    rolling_upgrade = false
  }
}
