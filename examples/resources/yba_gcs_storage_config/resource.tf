// GCS storage configuration with service account credentials.
resource "yba_gcs_storage_config" "gcs" {
  name            = "my-gcs-config"
  backup_location = "gs://my-bucket/yugabyte-backups"
  credentials     = file("~/.gcp/service-account.json")
}

// GCS storage configuration using GKE workload identity.
resource "yba_gcs_storage_config" "gcs_iam" {
  name            = "gcs-workload-identity"
  backup_location = "gs://my-bucket/yugabyte-backups"
  use_gcp_iam     = true
}
