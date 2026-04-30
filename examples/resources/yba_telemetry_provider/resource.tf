# Datadog destination
resource "yba_telemetry_provider" "datadog" {
  name = "kroger_datadog"

  data_dog {
    site    = "datadoghq.com"
    api_key = var.datadog_api_key
  }

  tags = {
    env = "prod"
  }
}

# Generic OTLP destination (e.g. Prometheus, Tempo, Loki w/ OTLP receiver)
resource "yba_telemetry_provider" "prometheus" {
  name = "kroger_prometheus"

  otlp {
    endpoint        = "http://10.242.32.5:9091/api/v1/otlp/v1/metrics"
    auth_type       = "NoAuth"
    protocol        = "HTTP"
    compression     = "gzip"
    timeout_seconds = 5
  }
}

# AWS CloudWatch destination
resource "yba_telemetry_provider" "cw" {
  name = "kroger_cloudwatch"

  aws_cloud_watch {
    log_group  = "yba/audit"
    log_stream = "primary"
    region     = "us-west-2"
    access_key = var.aws_access_key
    secret_key = var.aws_secret_key
  }
}
