# Perf Advisor online mode is off by default. YBA rejects every endpoint
# request until it is on.
resource "yba_runtime_config" "pa_online_mode" {
  key   = "yb.ui.feature_flags.enable_pa_online_mode"
  value = "true"
}

# A Perf Advisor that you run, with basic authentication on both URLs.
# YBA tests both URLs and their credentials before it saves the endpoint.
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

  depends_on = [yba_runtime_config.pa_online_mode]
}

# A BYOC ingest gateway in front of a Perf Advisor. The gateway identifies the
# sender by the account and project IDs, which YBA sends in headers to both
# URLs.
resource "yba_perf_advisor_endpoint" "byoc" {
  name = "byoc-prod"
  type = "BYOC"

  collection_endpoint = "https://pa-ingest.example.com"
  collection_auth {
    type     = "BASIC"
    username = var.byoc_ingest_username
    password = var.byoc_ingest_password
  }

  metrics_endpoint = "https://pa-ingest.example.com/api/v1/otlp/metrics"
  metrics_type     = "otlphttp"
  metrics_auth {
    type     = "BASIC"
    username = var.byoc_ingest_username
    password = var.byoc_ingest_password
  }

  ybm_account_id = var.ybm_account_id
  ybm_project_id = var.ybm_project_id

  depends_on = [yba_runtime_config.pa_online_mode]
}
