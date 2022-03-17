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

provider "google" {
  credentials = "/Users/stevendu/.yugabyte/yugabyte-gce.json"
  project     = "yugabyte"
  region      = "us-west1"
  zone        = "us-west1-b"
}

locals {
  dir          = "/Users/stevendu/code/terraform-provider-yugabyte-anywhere/modules/resources"
  cluster_name = "terraform-acctest-yugaware"
}

module "gcp-platform" {
  source = "../modules/gcp"

  cluster_name   = local.cluster_name
  ssh_user       = "centos"
  network_tags   = [local.cluster_name, "http-server", "https-server"]
  vpc_network    = "***REMOVED***"
  vpc_subnetwork = "***REMOVED***"
  // files
  ssh_private_key               = "/Users/stevendu/.ssh/yugaware-1-gcp"
  ssh_public_key                = "/Users/stevendu/.ssh/yugaware-1-gcp.pub"
}

provider "yb" {
  host = "${module.gcp-platform.public_ip}:80"
}

resource "yb_installation" "installation" {
  public_ip                 = module.gcp-platform.public_ip
  ssh_user                  = "centos"
  ssh_private_key           = file("/Users/stevendu/.ssh/yugaware-1-gcp")
  replicated_config_file    = "${local.dir}/replicated.conf"
  replicated_license_file   = "/Users/stevendu/.yugabyte/yugabyte-dev.rli"
  application_settings_file = "${local.dir}/application_settings.conf"
}

resource "yb_customer_resource" "customer" {
  code     = "admin"
  email    = "tf@yugabyte.com"
  name     = "tf-acctest"
  password = "Password1@"
}