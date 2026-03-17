# Basic On-Premises Provider with manual provisioning
resource "yba_onprem_provider" "example" {
  name             = "onprem-provider"
  ssh_user         = "yugabyte"
  ssh_keypair_name = "my-keypair"

  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  skip_provisioning        = true
  passwordless_sudo_access = true

  regions {
    code = "us-west"

    zones {
      code = "us-west-az1"
    }
    zones {
      code = "us-west-az2"
    }
  }

  instance_types {
    instance_type_code = "c5.large"
    num_cores          = 2
    mem_size_gb        = 4
    volume_size_gb     = 100
  }

  node_instances {
    ip            = "10.0.0.1"
    region_name   = "us-west"
    zone_name     = "us-west-az1"
    instance_type = "c5.large"
  }

  node_instances {
    ip            = "10.0.0.2"
    region_name   = "us-west"
    zone_name     = "us-west-az2"
    instance_type = "c5.large"
  }
}

# On-Premises Provider with auto-provisioning
resource "yba_onprem_provider" "provisioned" {
  name             = "onprem-auto-provisioned"
  ssh_user         = "yugabyte"
  ssh_keypair_name = "my-keypair"

  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  skip_provisioning        = false
  passwordless_sudo_access = true
  install_node_exporter    = true
  node_exporter_port       = 9300

  regions {
    code      = "dc1"
    latitude  = 37.7749
    longitude = -122.4194

    zones {
      code = "dc1-rack1"
    }
  }

  instance_types {
    instance_type_code = "large"
    num_cores          = 4
    mem_size_gb        = 16
    volume_size_gb     = 200
  }

  node_instances {
    ip            = "192.168.1.10"
    region_name   = "dc1"
    zone_name     = "dc1-rack1"
    instance_type = "large"
    instance_name = "node-1"
  }
}

# On-Premises Provider with multiple regions and NTP
resource "yba_onprem_provider" "multi_region" {
  name     = "onprem-multi-region"
  ssh_user = "centos"

  ssh_keypair_name        = "my-keypair"
  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  skip_provisioning        = true
  passwordless_sudo_access = true

  set_up_chrony = true
  ntp_servers   = ["pool.ntp.org", "time.google.com"]

  regions {
    code = "us-east"

    zones {
      code = "us-east-az1"
    }
  }

  regions {
    code = "us-west"

    zones {
      code = "us-west-az1"
    }
  }

  instance_types {
    instance_type_code = "standard"
    num_cores          = 8
    mem_size_gb        = 32
    volume_size_gb     = 500
  }
}
