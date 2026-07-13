# Generic OTLP destination (e.g. Prometheus with the OTLP receiver).
#
# When this resource is replaced (any field change forces a recreate),
# Terraform first rewrites every universe whose telemetry config
# references this provider to drop the exporter (rolling upgrade), then
# deletes the old provider and creates the replacement. The universe
# itself is never destroyed.
resource "yba_otlp_telemetry_provider" "prometheus" {
  name = "prometheus"

  endpoint        = "http://10.242.32.5:9091/api/v1/otlp/v1/metrics"
  auth_type       = "NoAuth"
  protocol        = "HTTP"
  compression     = "gzip"
  timeout_seconds = 5

  # Optional tags, upserted as attributes onto every exported record.
  tags = {
    env = "prod"
  }
}

# OTLP collector behind basic auth, with per-signal endpoint overrides
# (HTTP protocol only) and extra headers.
resource "yba_otlp_telemetry_provider" "collector" {
  name = "otel-collector"

  endpoint            = "https://collector.example.com:4318"
  protocol            = "HTTP"
  auth_type           = "BasicAuth"
  basic_auth_username = var.otlp_username
  basic_auth_password = var.otlp_password

  logs_endpoint    = "https://collector.example.com:4318/v1/logs"
  metrics_endpoint = "https://collector.example.com:4318/v1/metrics"

  headers = {
    "X-Scope-OrgID" = "yba"
  }
}

# OTLP endpoint authenticated with a bearer token (gRPC transport).
resource "yba_otlp_telemetry_provider" "bearer" {
  name = "otel-bearer"

  endpoint     = "https://otlp.example.com:4317"
  auth_type    = "BearerToken"
  bearer_token = var.otlp_bearer_token
}
