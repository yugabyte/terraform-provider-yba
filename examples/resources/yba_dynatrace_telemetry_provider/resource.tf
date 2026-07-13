# Dynatrace OTLP ingest destination. Metrics only: YBA does not allow
# Dynatrace in a universe's audit log or query log exporter lists.
resource "yba_dynatrace_telemetry_provider" "dynatrace" {
  name = "dynatrace"

  endpoint  = "https://abc12345.live.dynatrace.com/api/v2/otlp"
  api_token = var.dynatrace_api_token

  # Optional tags, upserted as attributes onto every exported record.
  tags = {
    env = "prod"
  }
}
