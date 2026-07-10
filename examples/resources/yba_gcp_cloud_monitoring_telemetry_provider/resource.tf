# Google Cloud Monitoring/Logging destination for audit/query logs.
resource "yba_gcp_cloud_monitoring_telemetry_provider" "gcm" {
  name = "gcp-cloud-monitoring"

  # Optional: defaults to the project_id inside the credentials JSON.
  project          = "my-gcp-project"
  credentials_json = file("service-account.json")
}
