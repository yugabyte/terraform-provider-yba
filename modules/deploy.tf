locals {
  dir = "/Users/stevendu/code/terraform-provider-yugabyte-platform/modules/resources"
}

terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
    aws = {
      source  = "hashicorp/aws"
    }
  }
}

provider "google" {
  credentials = "/Users/stevendu/.yugabyte/yugabyte-gce.json"
  project = "yugabyte"
  region = "us-west1"
  zone = "us-west1-b"
}

module "gcp-platform" {
  source = "./gcp"

  cluster_name = "sdu-test-yugaware"
  ssh_user = "centos"
  // files
  replicated_filepath = "${local.dir}/replicated.conf"
  license_filepath = "/Users/stevendu/.yugabyte/yw-dev.rli"
  tls_cert_filepath = ""
  tls_key_filepath = ""
  application_settings_filepath = "${local.dir}/application_settings.conf"
  ssh_private_key = "/Users/stevendu/.ssh/yugaware-1-gcp"
  ssh_public_key = "/Users/stevendu/.ssh/yugaware-1-gcp.pub"
}

provider "aws" {
  region = "us-west-2"
}

module "aws-platform" {
  source = "./aws"

  cluster_name = "sdu-test-yugaware"
  ssh_user = "ubuntu"
  ssh_keypair = "yb-dev-aws-2"
  security_group_name = "sdu_test_sg"
  vpc_id = "vpc-0fe36f6b"
  subnet_id = "subnet-f840ce9c"
  // files
  replicated_filepath = "${local.dir}/replicated.conf"
  license_filepath = "/Users/stevendu/.yugabyte/yw-dev.rli"
  tls_cert_filepath = ""
  tls_key_filepath = ""
  application_settings_filepath = "${local.dir}/application_settings.conf"
  ssh_private_key = "/Users/stevendu/.yugabyte/yb-dev-aws-2.pem"
}