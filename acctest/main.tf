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

module "gcp_yb_anywhere" {
  source = "../modules/docker/gcp"

  cluster_name    = local.cluster_name
  ssh_user        = "centos"
  network_tags    = [local.cluster_name, "http-server", "https-server"]
  vpc_network     = "yugabyte-network"
  vpc_subnetwork  = "subnet-us-west1"
  // files
  ssh_private_key = "/Users/stevendu/.ssh/yugaware-1-gcp"
  ssh_public_key  = "/Users/stevendu/.ssh/yugaware-1-gcp.pub"
}

output "host" {
  value = module.gcp_yb_anywhere.public_ip
}

provider "yb" {
  host = "${module.gcp_yb_anywhere.public_ip}:80"
}

resource "yb_installation" "installation" {
  public_ip                 = module.gcp_yb_anywhere.public_ip
  private_ip                = module.gcp_yb_anywhere.private_ip
  ssh_user                  = "centos"
  ssh_private_key           = file("/Users/stevendu/.ssh/yugaware-1-gcp")
  replicated_config_file    = "${local.dir}/replicated.conf"
  replicated_license_file   = "/Users/stevendu/.yugabyte/yugabyte-dev.rli"
  application_settings_file = "${local.dir}/application_settings.conf"
  cleanup                   = true
}
#
#resource "yb_customer_resource" "customer" {
#  depends_on = [yb_installation.installation]
#  code       = "admin"
#  email      = "tf@yugabyte.com"
#  name       = "tf-acctest"
#  password   = "Password1@"
#}
#
#output "api_key" {
#  value = yb_customer_resource.customer.api_token
#}