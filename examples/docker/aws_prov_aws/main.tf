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
  home         = "/home/deeptikumar"
  dir          = "${local.home}/code/terraform-provider-yugabytedb-anywhere/modules/resources"
  cluster_name = "terraform-aws"
  google_creds = "${local.home}/.yugabyte/yugabyte-gce.json"
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
  vpc_id              = "vpc-0fe36f6b"
  subnet_id           = "subnet-f840ce9c"
  // files
  ssh_private_key = "${local.home}/.yugabyte/yb-dev-aws-2.pem"
}

provider "yb" {
  alias = "unauthenticated"
  // these can be set as environment variables
  host = "${module.aws_yb_anywhere.public_ip}:80"
}

module "installation" {
  source = "../../../modules/installation"

  public_ip = module.aws_yb_anywhere.public_ip
  private_ip = module.aws_yb_anywhere.private_ip
  ssh_user = "ubuntu"
  ssh_private_key_file = "${local.home}/.yugabyte/yb-dev-aws-2.pem"
  replicated_directory = local.dir
  replicated_license_file_path = "${local.home}/.yugabyte/yugabyte-dev.rli"
}

resource "yb_customer_resource" "customer" {
  provider   = yb.unauthenticated
  depends_on = [module.installation]
  code       = "admin"
  email      = "demo@yugabyte.com"
  name       = "demo"
  password   = "Password1@"
}

provider "yb" {
  host      = "${module.aws_yb_anywhere.public_ip}:80"
  api_token = yb_customer_resource.customer.api_token
}

resource "yb_cloud_provider" "aws" {
  code = "aws"
  config = {
    "AWS_ACCESS_KEY_ID" = "<access-key-id>",
    "AWS_SECRET_ACCESS_KEY" = "<secret-access-key>"
  }
  
  name        = "${local.cluster_name}-provider"
  regions {
    code = "us-west-2"
    name = "us-west-2"
    security_group_id = "sg-139dde6c"
    vnet_name = "vpc-0fe36f6b"
    zones {
      code = "us-west-2a"
      name = "us-west-2a"
      subnet = "subnet-6553f513"
    }
    zones {
      code = "us-west-2b"
      name = "us-west-2b"
      subnet = "subnet-f840ce9c"
    }
    zones {
      code = "us-west-2c"
      name = "us-west-2c"
      subnet = "subnet-01ac5b59"
    }
  }
  
  ssh_port        = 22
  air_gap_install = false
}

data "yb_provider_key" "aws-key" {
  provider_id = yb_cloud_provider.aws.id
}

locals {
  region_list  = yb_cloud_provider.aws.regions[*].uuid
  provider_id  = yb_cloud_provider.aws.id
  provider_key = data.yb_provider_key.aws-key.id
}

resource "yb_universe" "aws_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "terraform-aws-universe--01"
      provider_type      = "aws"
      provider           = local.provider_id
      region_list        = local.region_list
      num_nodes          = 1
      replication_factor = 1
      instance_type      = "c5.large"
      device_info {
        num_volumes  = 1
        volume_size  = 250
        disk_iops = 3000
        throughput = 125
        storage_type = "GP3"
        storage_class = "standard"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = "2.17.1.0-b238"
      access_key_code               = local.provider_key
    }
  }
  communication_ports {}
}