resource "yba_onprem_provider" "onprem" {
  name = "<onprem-provider-name>"
  access_keys {
    key_info {
      key_pair_name             = "<ssh-key-pair-name>"
      ssh_private_key_file_path = "<ssh-key-pair-file-path>"
    }

  }
  regions {
    name = "<region-name>"
    zones {
      name = "<zone-name>"
    }

  }
  details {
    passwordless_sudo_access = true
    ssh_user                 = "<ssh-user>"
    skip_provisioning        = false
  }

  instance_types {
    instance_type_key {
      instance_type_code = "<instance-type-name>"
    }
    instance_type_details {
      volume_details_list {
        mount_path     = "<mount-paths-separated-by-commas>"
        volume_size_gb = 100
      }
    }
    mem_size_gb = 15
    num_cores   = 4
  }
  node_instances {
    instance_type = "<instance-type-name>"
    ip            = "<node-ip-instance>"
    region        = "<region-name>"
    zone          = "<zone-name>"
  }

}
