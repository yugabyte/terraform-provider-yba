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

# Universe with encryption-in-transit certificates that Terraform manages: a
# self-signed root certificate for node-to-node encryption, and a custom server
# certificate from your organization's CA for client-to-node encryption. A change
# of root_ca or client_root_ca rotates the universe to the new certificate. A
# change of a cert_rotation trigger re-issues the server certificates from the
# current self-signed root certificate.
resource "yba_universe" "with_certificates" {
  root_ca        = yba_self_signed_certificate.generated.uuid
  client_root_ca = yba_custom_server_certificate.c2n.uuid

  cert_rotation {
    server_cert_trigger = "2026-07" # change to re-issue node-to-node server certificates
    # client_cert_trigger re-issues the client-to-node server certificates. It needs a
    # self-signed client_root_ca, so this universe, which uses a custom server certificate
    # for client-to-node encryption, does not set it.
  }

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
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = data.yba_release_version.release_version.id
      access_key_code               = data.yba_provider_key.cloud_key.id
    }
  }
  communication_ports {}
}

# Universe encrypted at rest from its first write. The encryption_at_rest block
# names the encryption-at-rest configuration whose master key wraps the universe
# keys. A later change of kms_config_uuid rotates the master key, a change of
# the trigger rotates the universe key, and enabled = false disables encryption.
resource "yba_universe" "encrypted_at_rest" {
  encryption_at_rest {
    enabled                       = true
    kms_config_uuid               = yba_gcp_ear_config.service_account.uuid
    universe_key_rotation_trigger = "2026-Q3" # change to rotate the universe key
  }

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
