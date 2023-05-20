data "yba_storage_configs" "configs" {
  // To fetch any storage config
}

data "yba_storage_configs" "configs_gcs" {
  // To fetch id of a particular storage config
  config_name = "<storage-config-name>"
}
