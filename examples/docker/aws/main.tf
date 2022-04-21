terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

locals {
  dir          = "/Users/stevendu/code/terraform-provider-yugabyte-anywhere/modules/resources"
  cluster_name = "sdu-test-yugaware"
  google_creds = "/Users/stevendu/.yugabyte/yugabyte-gce.json"
  software_version = "2.13.1.0-b69"
}

provider "aws" {
  region = "us-west-2"
}

module "aws_yb_anywhere" {
  source = "../../../modules/docker/aws"

  cluster_name        = local.cluster_name
  ssh_user            = "ubuntu"
  ssh_keypair         = "yb-dev-aws-2"
  security_group_name = "${local.cluster_name}_sg"
  vpc_id              = "***REMOVED***"
  subnet_id           = "***REMOVED***"
  // files
  ssh_private_key = "/Users/stevendu/.yugabyte/yb-dev-aws-2.pem"
}

provider "yb" {
  alias = "unauthenticated"
  // these can be set as environment variables
  host = "${module.aws_yb_anywhere.public_ip}:80"
}

resource "yb_installation" "installation" {
  provider                  = yb.unauthenticated
  public_ip                 = module.aws_yb_anywhere.public_ip
  private_ip                = module.aws_yb_anywhere.private_ip
  ssh_user                  = "ubuntu"
  ssh_private_key           = file("/Users/stevendu/.yugabyte/yb-dev-aws-2.pem")
  replicated_config_file    = "${local.dir}/replicated.conf"
  replicated_license_file   = "/Users/stevendu/.yugabyte/yugabyte-dev.rli"
  application_settings_file = "${local.dir}/application_settings.conf"
}

resource "yb_customer_resource" "customer" {
  provider   = yb.unauthenticated
  depends_on = [yb_installation.installation]
  code       = "admin"
  email      = "sdu@yugabyte.com"
  name       = "sdu"
  password   = "Password1@"
}

provider "yb" {
  host      = "${module.aws_yb_anywhere.public_ip}:80"
  api_token = yb_customer_resource.customer.api_token
}

resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  config = merge(
    { YB_FIREWALL_TAGS = "cluster-server" },
    jsondecode(file(local.google_creds))
  )
  dest_vpc_id = "***REMOVED***"
  name        = "${local.cluster_name}-provider"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 54422
  air_gap_install = false
}

data "yb_provider_key" "gcp-key" {
  provider_id = yb_cloud_provider.gcp.id
}

resource "yb_universe" "gcp_universe" {
  depends_on = [yb_cloud_provider.gcp]
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "${local.cluster_name}-gcp-universe"
      provider_type      = "gcp"
      provider           = yb_cloud_provider.gcp.id
      region_list        = yb_cloud_provider.gcp.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "n1-standard-1"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "Persistent"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = local.software_version
      access_key_code               = data.yb_provider_key.gcp-key.id
    }
  }
  communication_ports {}
}

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