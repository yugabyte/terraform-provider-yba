# GCP Cloud Monitoring destination for logs, in Google Cloud Logging.
resource "yba_gcp_cloud_monitoring_telemetry_provider" "gcm" {
  name = "gcp-cloud-monitoring"

  # Optional. Defaults to the project_id in the credentials JSON.
  project          = "my-gcp-project"
  credentials_json = file("service-account.json")

  # Optional tags. YBA adds them as attributes to every exported record.
  tags = {
    env = "prod"
  }
}
