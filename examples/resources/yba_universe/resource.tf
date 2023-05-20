resource "yba_universe" "universe_name" {
  clusters {
    cluster_type = "<cluster-type>"
    user_intent {
      universe_name      = "%s"
      provider_type      = "<yb_cloud_proivder.cloud_provider.code>"
      provider           = "<yba_cloud_provider.cloud_provider.id>"
      region_list        = "<yba_cloud_provider.cloud_provider.regions[*].uuid>"
      num_nodes          = 1
      replication_factor = 3
      instance_type      = "<instance-type>"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "%s"
      }
      use_time_sync       = true
      enable_ysql         = true
      yb_software_version = "<YBDB-version - data.yba_release_version.release_version.id>"
      access_key_code     = "<access-key - data.yba_provider_key.cloud_key.id>"
    }
  }
  communication_ports {}
}
