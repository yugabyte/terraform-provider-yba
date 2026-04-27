resource "yba_universe" "universe_name" {
  clusters {
    cluster_type = "<cluster-type>"
    user_intent {
      universe_name      = "<universe-name>"
      provider_type      = "<yba_cloud_provider.cloud_provider.code>"
      provider           = "<yba_cloud_provider.cloud_provider.id>"
      region_list        = "<yba_cloud_provider.cloud_provider.regions[*].uuid>"
      num_nodes          = 3
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

# Universe with dedicated master nodes: masters run on separate nodes from TServers.
# Omit instance_type inside dedicated_masters block to use the
# same instance type as the TServer nodes.
resource "yba_universe" "dedicated_masters" {
  clusters {
    cluster_type = "<cluster-type>"
    user_intent {
      universe_name      = "<universe-name>"
      provider_type      = "<yba_cloud_provider.cloud_provider.code>"
      provider           = "<yba_cloud_provider.cloud_provider.id>"
      region_list        = "<yba_cloud_provider.cloud_provider.regions[*].uuid>"
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "<instance-type>"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "%s"
      }
      dedicated_masters {
        instance_type = "<master-instance-type>"
      }
      use_time_sync       = true
      enable_ysql         = true
      yb_software_version = "<YBDB-version - data.yba_release_version.release_version.id>"
      access_key_code     = "<access-key - data.yba_provider_key.cloud_key.id>"
    }
  }
  communication_ports {}
}

resource "yba_universe" "dedicated_masters" {
  clusters {
    cluster_type = "<cluster-type>"
    user_intent {
      universe_name      = "<universe-name>"
      provider_type      = "<yba_cloud_provider.cloud_provider.code>"
      provider           = "<yba_cloud_provider.cloud_provider.id>"
      region_list        = "<yba_cloud_provider.cloud_provider.regions[*].uuid>"
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "<instance-type>"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "%s"
      }
      dedicated_masters {} # Use the tserver instance type and device info for dedicated masters
      use_time_sync       = true
      enable_ysql         = true
      yb_software_version = "<YBDB-version - data.yba_release_version.release_version.id>"
      access_key_code     = "<access-key - data.yba_provider_key.cloud_key.id>"
    }
  }
  communication_ports {}
}