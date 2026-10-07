# Dynatrace destination for metrics. YBA does not accept a Dynatrace
# telemetry provider in a log pipeline.
resource "yba_dynatrace_telemetry_provider" "dynatrace" {
  name = "dynatrace"

  # The URL of the Dynatrace environment. YBA appends /api/v2/otlp.
  endpoint  = "https://abc12345.live.dynatrace.com"
  api_token = var.dynatrace_api_token

  # Optional tags. YBA adds them as attributes to every exported record.
  tags = {
    env = "prod"
  }
}
