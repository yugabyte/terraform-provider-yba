# Splunk HTTP Event Collector (HEC) destination for logs.
resource "yba_splunk_telemetry_provider" "splunk" {
  name = "splunk"

  endpoint = "https://splunk.example.com:8088"
  token    = var.splunk_hec_token

  # Optional Splunk fields for the events.
  source      = "yba"
  source_type = "_json"
  index       = "main"

  # Optional tags. YBA adds them as attributes to every exported record.
  tags = {
    env = "prod"
  }
}
