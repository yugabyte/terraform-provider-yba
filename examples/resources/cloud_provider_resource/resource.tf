terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "yb" {
  apikey = "***REMOVED***"
  host = "portal.dev.yugabyte.com"
}

resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  config = {
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    YB_FIREWALL_TAGS = "cluster-server"
  }
  dest_vpc_id = "***REMOVED***"
  name = "sdu-test-gcp-provider"
  regions {
    code = "us-central1"
    name = "us-central1"
  }
  ssh_port = 54422
  air_gap_install = false
}

data "yb_provider_key" "gcp-key" {}

resource "yb_universe" "gcp_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name = "sdu-test-gcp-universe"
      provider_type = "aws"
      provider = yb_cloud_provider.gcp.id
      region_list = [for r in yb_cloud_provider.gcp.regions : r.uuid]
      num_nodes = 3
      replication_factor = 3
      instance_type = "c5.large"
      device_info {
        num_volumes = 1
        volume_size = 250
        storage_type = "GP2"
      }
      assign_public_ip = true
      use_time_sync = true
      enable_ysql = true
      enable_node_to_node_encrypt = true
      enable_client_to_node_encrypt = true
      yb_software_version = "2.7.3.0-b80"
      access_key_code = data.yb_provider_key.gcp-key.id
    }
  }
}

output "provider" {
  value = yb_cloud_provider.gcp
}
