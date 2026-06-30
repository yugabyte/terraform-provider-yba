# Look up an existing telemetry provider by name (e.g. one created outside
# this Terraform configuration) and reference its UUID without hard-coding it.
data "yba_telemetry_provider" "datadog" {
  name = "datadog"
}

# Wire the looked-up provider into a universe's audit-log export pipeline.
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
