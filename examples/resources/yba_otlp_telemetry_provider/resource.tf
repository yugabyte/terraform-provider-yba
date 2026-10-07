# OTLP destination, for example Prometheus with its OTLP receiver turned on.
# With protocol = "HTTP", endpoint is a base URL: metrics go to
# <endpoint>/v1/metrics.
resource "yba_otlp_telemetry_provider" "prometheus" {
  name = "prometheus"

  endpoint        = "http://prometheus.example.com:9090/api/v1/otlp"
  auth_type       = "NoAuth"
  protocol        = "HTTP"
  compression     = "gzip"
  timeout_seconds = 5

  # Optional tags. YBA adds them as attributes to every exported record.
  tags = {
    env = "prod"
  }
}

# OTLP collector behind basic authentication, with full URLs for logs and
# metrics (HTTP protocol only) and extra headers.
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

# OTLP endpoint with bearer token authentication, over gRPC.
resource "yba_otlp_telemetry_provider" "bearer" {
  name = "otel-bearer"

  endpoint     = "https://otlp.example.com:4317"
  auth_type    = "BearerToken"
  bearer_token = var.otlp_bearer_token
}
