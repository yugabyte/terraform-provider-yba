resource "yba_storage_config_resource" "storage_config" {
  name            = "<storage-config-code>"
  backup_location = "<storage-location/bucket-location>"
  config_name     = "<storage-config-name>"
}

resource "yba_storage_config_resource" "s3_storage_config" {
  name            = "S3"
  backup_location = "<storage-location/bucket-location>"
  config_name     = "<storage-config-name>"
  s3_credentials {
    access_key_id     = "<s3-access-key-id>"
    secret_access_key = "<s3-secret-access-key>"
  }
}

resource "yba_storage_config_resource" "gcs_storage_config" {
  name            = "GCS"
  backup_location = "<storage-location/bucket-location>"
  config_name     = "<storage-config-name>"
  gcs_credentials {
    application_credentials = <<EOT
    <gcs-service-account-credentials-json>
    EOT
  }
}

resource "yba_storage_config_resource" "az_storage_config" {
  name            = "AZ"
  backup_location = "<storage-location/bucket-location>"
  config_name     = "<storage-config-name>"
  azure_credentials {
    sas_token = "<azure-sas-token>"
  }
}