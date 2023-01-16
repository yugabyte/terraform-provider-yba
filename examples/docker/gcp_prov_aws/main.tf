terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
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
  cluster_name = "terraform-gcp-new-02"
}

provider "google" {
  credentials = "${local.home}/.yugabyte/yugabyte-gce.json"
  project     = "yugabyte"
  region      = "us-west1"
  zone        = "us-west1-b"
}

module "gcp_yb_anywhere" {
  source = "../../../modules/docker/gcp"

  cluster_name   = local.cluster_name
  ssh_user       = "centos"
  network_tags   = [local.cluster_name, "http-server", "https-server"]
  vpc_network    = "yugabyte-network"
  vpc_subnetwork = "subnet-us-west1"
  // files
  ssh_private_key = "${local.home}/.ssh/yugaware-1-gcp"
  ssh_public_key  = "${local.home}/.ssh/yugaware-1-gcp.pub"
}

provider "yb" {
  alias = "unauthenticated"
  // these can be set as environment variables
  host = "${module.gcp_yb_anywhere.public_ip}:80"
}

module "installation" {
  source = "../../../modules/installation"

  public_ip = module.gcp_yb_anywhere.public_ip
  private_ip = module.gcp_yb_anywhere.private_ip
  ssh_user = "centos"
  ssh_private_key_file = "${local.home}/.ssh/yugaware-1-gcp"
  replicated_directory = local.dir
  replicated_license_file_path = "${local.home}/.yugabyte/yugabyte-dev.rli"
}

provider "yb" {
  host      = "${module.gcp_yb_anywhere.public_ip}:80"
  api_token = yb_customer_resource.customer.api_token
}

resource "yb_customer_resource" "customer" {
  provider   = yb.unauthenticated
  depends_on = [module.installation]
  code       = "admin"
  email      = "demo@yugabyte.com"
  name       = "demo"
  password   = "Password1@"
}


resource "yb_cloud_provider" "aws" {
  lifecycle {
    ignore_changes = all
  }
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
data "yb_release_version" "release_version"{
  depends_on = [
    yb_cloud_provider.aws
  ]
   
}

resource "yb_universe" "aws_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "terraform-aws-in-gcp-universe--01"
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
      yb_software_version           = data.yb_release_version.release_version.id
      access_key_code               = local.provider_key
    }
  }
  communication_ports {}
}