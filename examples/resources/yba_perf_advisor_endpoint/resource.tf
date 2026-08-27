# A customer-run Perf Advisor reached directly.
#
# YBA probes both endpoints from a Perf Advisor collector before storing
# anything, so a destination that is unreachable or rejects these credentials
# fails the apply rather than silently dropping data later.
resource "yba_perf_advisor_endpoint" "standalone" {
  name = "perf-advisor-prod"
  type = "BYOC"

  collection_endpoint = "https://perf-advisor.example.com:9443"
  collection_auth {
    type     = "BASIC"
    username = var.pa_username
    password = var.pa_password
  }

  metrics_endpoint = "https://perf-advisor.example.com:9443/api/v1/otlp/metrics"
  metrics_type     = "otlphttp"
  metrics_auth {
    type     = "BASIC"
    username = var.pa_username
    password = var.pa_password
  }
}

# The BYOC ingest gateway, which identifies the sending account and project by
# header rather than by credential. Both identifiers are required there and are
# sent on the metrics and the collection endpoint alike.
resource "yba_perf_advisor_endpoint" "byoc" {
  name = "byoc-prod"
  type = "BYOC"

  collection_endpoint = "https://byoc.cloud.yugabyte.com"
  collection_auth {
    type     = "BASIC"
    username = var.byoc_ingest_username
    password = var.byoc_ingest_password
  }

  metrics_endpoint = "https://byoc.cloud.yugabyte.com/api/v1/otlp/metrics"
  metrics_type     = "otlphttp"
  metrics_auth {
    type     = "BASIC"
    username = var.byoc_ingest_username
    password = var.byoc_ingest_password
  }

  ybm_account_id = var.ybm_account_id
  ybm_project_id = var.ybm_project_id
}
