# Look up a telemetry provider by name, for example one created outside this
# Terraform configuration.
data "yba_telemetry_provider" "datadog" {
  name = "datadog"
}

# Send the audit logs of a universe to the telemetry provider.
resource "yba_universe_telemetry_config" "example" {
  universe_uuid = var.universe_uuid

  audit_logs {
    ysql_audit_config {
      classes = ["READ", "WRITE", "DDL"]
    }

    exporter {
      exporter_uuid = data.yba_telemetry_provider.datadog.id
    }
  }
}
