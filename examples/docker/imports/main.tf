terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
    aws = {
      source = "hashicorp/aws"
    }
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "google" {
  credentials = "/home/deeptikumar/.yugabyte/yugabyte-gce.json"
  project     = "yugabyte"
  region      = "us-west1"
  zone        = "us-west1-b"
}
provider "aws" {
  region = "us-west-2"
}
locals {
  dir          = "/home/deeptikumar/code/terraform-provider-yugabytedb-anywhere/modules/resources"
}

resource "aws_instance" "vm" {
  ami = ""
  instance_type = ""
  tags  = {
          Name = "terraform-aws" 
        }
  tags_all = {
         Name = "terraform-aws"
  }
}

provider "yb" {
  alias = "unauthenticated"
  // these can be set as environment variables
  host = "${aws_instance.vm.public_ip}:80"
}

module "installation" {
  source = "../../../modules/installation"

  public_ip = aws_instance.vm.public_ip
  private_ip = aws_instance.vm.private_ip
  ssh_user = "ubuntu"
  ssh_private_key_file = "/home/deeptikumar/.yugabyte/yb-dev-aws-2.pem"
  replicated_directory = local.dir
  replicated_license_file_path = "/home/deeptikumar/.yugabyte/yugabyte-dev.rli"
}
resource "yb_customer_resource" "name" {
  code      = ""
  email     = ""
  name      = ""
  password  = ""

}

resource "yb_cloud_provider" "aws" {
  code              = "aws"
  name              = "terraform-aws-provider"

  regions {}
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
  version = ""
}

resource "yb_universe" "aws_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "terraform-aws-universe--02"
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