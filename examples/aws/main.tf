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
}

provider "aws" {
  region = "us-west-2"
}

module "aws-platform" {
  source = "../../modules/aws"

  cluster_name        = local.cluster_name
  ssh_user            = "ubuntu"
  ssh_keypair         = "yb-dev-aws-2"
  security_group_name = "sdu_test_sg"
  vpc_id              = "***REMOVED***"
  subnet_id           = "***REMOVED***"
  // files
  ssh_private_key               = "/Users/stevendu/.yugabyte/yb-dev-aws-2.pem"
}

provider "yb" {
  // these can be set as environment variables
  host = "${module.aws-platform.public_ip}:80"
}

resource "yb_customer_resource" "customer" {
  code     = "admin"
  email    = "sdu@yugabyte.com"
  name     = "sdu"
  password = "Password1@"
}

resource "yb_installation" "installation" {
  public_ip                 = module.aws-platform.public_ip
  ssh_user                  = "centos"
  ssh_private_key           = file("/Users/stevendu/.ssh/yugaware-1-gcp")
  replicated_config_file    = "${local.dir}/replicated.conf"
  replicated_license_file   = "/Users/stevendu/.yugabyte/yugabyte-dev.rli"
  application_settings_file = "${local.dir}/application_settings.conf"
}

resource "yb_cloud_provider" "gcp" {
  connection_info {
    cuuid     = yb_customer_resource.customer.cuuid
    api_token = yb_customer_resource.customer.api_token
  }

  code = "gcp"
  config = merge(
    { YB_FIREWALL_TAGS = "cluster-server" },
    jsondecode(file("/Users/stevendu/.yugabyte/yugabyte-gce.json"))
  )
  dest_vpc_id = "***REMOVED***"
  name        = "sdu-test-gcp-provider"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 54422
  air_gap_install = false
}

data "yb_provider_key" "gcp-key" {
  connection_info {
    cuuid     = yb_customer_resource.customer.cuuid
    api_token = yb_customer_resource.customer.api_token
  }

  provider_id = yb_cloud_provider.gcp.id
}

locals {
  region_list  = yb_cloud_provider.gcp.regions[*].uuid
  provider_id  = yb_cloud_provider.gcp.id
  provider_key = data.yb_provider_key.gcp-key.id
}

resource "yb_universe" "gcp_universe" {
  connection_info {
    cuuid     = yb_customer_resource.customer.cuuid
    api_token = yb_customer_resource.customer.api_token
  }

  depends_on = [yb_cloud_provider.gcp]
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "sdu-test-gcp-universe"
      provider_type      = "gcp"
      provider           = local.provider_id
      region_list        = local.region_list
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
      yb_software_version           = "2.12.1.0-b41"
      access_key_code               = local.provider_key
    }
  }
  communication_ports {}
}