# Splunk HTTP Event Collector destination for audit/query logs.
resource "yba_splunk_telemetry_provider" "splunk" {
  name = "splunk"

  endpoint = "https://splunk.example.com:8088"
  token    = var.splunk_hec_token

  # Optional Splunk event routing fields.
  source      = "yba"
  source_type = "_json"
  index       = "main"
}
