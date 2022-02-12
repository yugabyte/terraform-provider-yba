terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "yb" {
  // these can be set as environment variables
  apikey = "***REMOVED***"
  host = "portal.dev.yugabyte.com"
}

resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  custom_host_cidrs = []
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
  dest_vpc_id = "yugabyte-network"
  name = "sdu-test-gcp-provider"
  regions {
    code = "us-central1"
    name = "us-central1"
  }
  ssh_port = 54422
  air_gap_install = false
}

data "yb_provider_key" "gcp-key" {
  provider_id = yb_cloud_provider.gcp.id
}

locals {
  region_list = yb_cloud_provider.gcp.computed_regions[*].uuid
  provider_id = yb_cloud_provider.gcp.id
  provider_key = data.yb_provider_key.gcp-key.id
}

output "provider" {
  value = yb_cloud_provider.gcp
}

resource "yb_universe" "gcp_universe" {
  depends_on = [yb_cloud_provider.gcp]
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name = "sdu-test-gcp-universe"
      provider_type = "gcp"
      provider = local.provider_id
      region_list = local.region_list
      num_nodes = 3
      replication_factor = 3
      instance_type = "n1-standard-1"
      device_info {
        num_volumes = 1
        volume_size = 375
        storage_type = "Persistent"
      }
      assign_public_ip = true
      use_time_sync = true
      enable_ysql = true
      enable_node_to_node_encrypt = true
      enable_client_to_node_encrypt = true
      yb_software_version = "2.7.3.0-b80"
      access_key_code = local.provider_key

    }
  }
}

data "yb_storage_configs" "configs" {}

resource "yb_backups" "gcp_universe_backup" {
  depends_on = [yb_universe.gcp_universe]
  uni_uuid = yb_universe.gcp_universe.id
  action_type = "CREATE"
  keyspace = "postgres"
  storage_config_uuid = data.yb_storage_configs.configs.uuid_list[0]
  time_before_delete = 864000000
  sse = false
  transactional_backup = false
  parallelism = 8
  backup_type = "PGSQL_TABLE_TYPE"
}


