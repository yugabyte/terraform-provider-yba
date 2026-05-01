# Datadog destination
resource "yba_telemetry_provider" "datadog" {
  name = "datadog"

  data_dog {
    site    = "datadoghq.com"
    api_key = var.datadog_api_key
  }

  tags = {
    env = "prod"
  }
}

# Generic OTLP destination (e.g. Prometheus, Tempo, Loki w/ OTLP receiver).
#
# When this resource is replaced (any config change forces a recreate),
# Terraform first rewrites every universe whose telemetry config
# references this provider to drop the exporter (rolling upgrade), then
# deletes the old provider and creates the replacement. The universe
# itself is never destroyed.
resource "yba_telemetry_provider" "prometheus" {
  name = "prometheus"

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
  name = "cloudwatch"

  aws_cloud_watch {
    log_group  = "yba/audit"
    log_stream = "primary"
    region     = "us-west-2"
    access_key = var.aws_access_key
    secret_key = var.aws_secret_key
  }
}

# S3 archival destination (long-term log storage)
resource "yba_telemetry_provider" "audit_archive" {
  name = "audit-archive"

  s3 {
    bucket           = "yba-audit-logs"
    region           = "us-west-2"
    access_key       = var.aws_access_key
    secret_key       = var.aws_secret_key
    directory_prefix = "yb-logs"

    include_universe_and_node_in_prefix = true
  }
}
