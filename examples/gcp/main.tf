terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
    yb = {
      version = "~> 0.1.0"
      source = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "google" {
  credentials = "/Users/stevendu/.yugabyte/yugabyte-gce.json"
  project     = "yugabyte"
  region      = "us-west1"
  zone        = "us-west1-b"
}

locals {
  dir = "/Users/stevendu/code/terraform-provider-yugabyte-platform/modules/resources"
}

module "gcp-platform" {
  source = "../../modules/gcp"

  cluster_name                  = "sdu-test-yugaware"
  ssh_user                      = "centos"
  // files
  replicated_filepath           = "${local.dir}/replicated.conf"
  license_filepath              = "/Users/stevendu/.yugabyte/yw-dev.rli"
  tls_cert_filepath             = ""
  tls_key_filepath              = ""
  application_settings_filepath = "${local.dir}/application_settings.conf"
  ssh_private_key               = "/Users/stevendu/.ssh/yugaware-1-gcp"
  ssh_public_key                = "/Users/stevendu/.ssh/yugaware-1-gcp.pub"
}

provider "yb" {
  // these can be set as environment variables
  host = "${module.gcp-platform.public_ip}:80"
}

resource "yb_customer_resource" "customer" {
  code = "admin"
  email = "sdu@yugabyte.com"
  name = "sdu"
  password = "Password1@"
}

resource "yb_cloud_provider" "gcp" {
  customer_id = yb_customer_resource.customer.id
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
  dest_vpc_id = "yugabyte-network"
  name = "sdu-test-gcp-provider"
  regions {
    code = "us-central1"
    name = "us-central1"
  }
  ssh_port = 54422
  air_gap_install = false
}

#data "yb_provider_key" "gcp-key" {
#  provider_id = yb_cloud_provider.gcp.id
#}
#
#locals {
#  region_list = yb_cloud_provider.gcp.regions[*].uuid
#  provider_id = yb_cloud_provider.gcp.id
#  provider_key = data.yb_provider_key.gcp-key.id
#}
#
#resource "yb_universe" "gcp_universe" {
#  depends_on = [yb_cloud_provider.gcp]
#  clusters {
#    cluster_type = "PRIMARY"
#    user_intent {
#      universe_name = "sdu-test-gcp-universe"
#      provider_type = "gcp"
#      provider = local.provider_id
#      region_list = local.region_list
#      num_nodes = 3
#      replication_factor = 3
#      instance_type = "n1-standard-1"
#      device_info {
#        num_volumes = 1
#        volume_size = 375
#        storage_type = "Persistent"
#      }
#      assign_public_ip = true
#      use_time_sync = true
#      enable_ysql = true
#      enable_node_to_node_encrypt = true
#      enable_client_to_node_encrypt = true
#      yb_software_version = "2.7.3.0-b80"
#      access_key_code = local.provider_key
#    }
#  }
#  communication_ports {}
#}
#
#data "yb_storage_configs" "configs" {}

#resource "yb_backups" "gcp_universe_backup" {
#  depends_on = [yb_universe.gcp_universe]
#
#  uni_uuid = yb_universe.gcp_universe.id
#  keyspace = "postgres"
#  storage_config_uuid = data.yb_storage_configs.configs.uuid_list[0]
#  time_before_delete = 864000000
#  sse = false
#  transactional_backup = false
#  frequency = 864000000
#  parallelism = 8
#  backup_type = "PGSQL_TABLE_TYPE"
#}

#resource "yb_user" "user" {
#  email = "sdu@yugabyte.com"
#  password = "Password1@"
#  role = "ReadOnly"
#}