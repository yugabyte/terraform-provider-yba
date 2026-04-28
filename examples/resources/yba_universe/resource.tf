data "yba_provider_key" "cloud_key" {
  provider_id = yba_aws_provider.aws.id
}

data "yba_release_version" "release_version" {
  depends_on = [yba_aws_provider.aws]
}

resource "yba_universe" "universe_name" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "<universe-name>"
      provider           = yba_aws_provider.aws.id
      region_list        = yba_aws_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "<instance-type>"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "<storage-type>"
      }
      use_time_sync       = true
      enable_ysql         = true
      yb_software_version = data.yba_release_version.release_version.id
      access_key_code     = data.yba_provider_key.cloud_key.id
    }
  }
  communication_ports {}
}

# Universe with dedicated master nodes: masters run on separate nodes from TServers.
# This variant pins a distinct instance_type and device_info for the master nodes.
resource "yba_universe" "dedicated_masters" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "<universe-name>"
      provider           = yba_aws_provider.aws.id
      region_list        = yba_aws_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "<instance-type>"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "<storage-type>"
      }
      dedicated_masters {
        instance_type = "<master-instance-type>"
        device_info {
          num_volumes  = 1
          volume_size  = 100
          storage_type = "<storage-type>"
        }
      }
      use_time_sync       = true
      enable_ysql         = true
      yb_software_version = data.yba_release_version.release_version.id
      access_key_code     = data.yba_provider_key.cloud_key.id
    }
  }
  communication_ports {}
}

# Same as above but omits instance_type / device_info inside dedicated_masters,
# so masters inherit the TServer instance type and device info.
resource "yba_universe" "dedicated_masters_inherit" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "<universe-name>"
      provider           = yba_aws_provider.aws.id
      region_list        = yba_aws_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "<instance-type>"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "<storage-type>"
      }
      dedicated_masters {}
      use_time_sync       = true
      enable_ysql         = true
      yb_software_version = data.yba_release_version.release_version.id
      access_key_code     = data.yba_provider_key.cloud_key.id
    }
  }
  communication_ports {}
}
