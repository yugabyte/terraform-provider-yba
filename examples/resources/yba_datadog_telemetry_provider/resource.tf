# Datadog destination for audit/query logs and metrics.
resource "yba_datadog_telemetry_provider" "datadog" {
  name = "datadog"

  site    = "datadoghq.com"
  api_key = var.datadog_api_key

  tags = {
    env = "prod"
  }
}
