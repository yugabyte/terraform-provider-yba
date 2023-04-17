resource "yb_storage_config_resource" "gcs_storage" {
    name            = "<storage-config-code>"
    backup_location = "<storage-location/bucket-location>"
    config_name     = "<storage-config-name>"
}