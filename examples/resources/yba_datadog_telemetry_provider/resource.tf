# Datadog destination for logs and metrics.
resource "yba_datadog_telemetry_provider" "datadog" {
  name = "datadog"

  site    = "datadoghq.com"
  api_key = var.datadog_api_key

  # Optional tags. YBA adds them as attributes to every exported record.
  tags = {
    env = "prod"
  }
}
