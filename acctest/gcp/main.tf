terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
    yba = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yba"
    }
  }
}

variable "RESOURCES_DIR" {
  type        = string
  description = "directory on the platform runner that holds testing resources"
}

variable "GCP_VPC_SUBNETWORK" {
  type        = string
  description = "GCP VPC subnet to run acceptance testing"
}

variable "GCP_VPC_NETWORK" {
  type        = string
  description = "GCP VPC network to run acceptance testing"
}

variable "RUNNER_IP" {
  description = "IP of the runners to be able to connect to the instances"
  type = string
}

resource "random_uuid" "random" {
}

provider "google" {}

module "gcp_yb_anywhere" {
  source = "../../modules/docker/gcp"

  cluster_name   = "tf-acctest-${random_uuid.random.result}"
  ssh_user       = "tf"
  network_tags   = ["terraform-acctest-yugaware", "http-server", "https-server"]
  vpc_network    = var.GCP_VPC_NETWORK
  vpc_subnetwork = var.GCP_VPC_SUBNETWORK
  // files
  ssh_private_key = "${var.RESOURCES_DIR}/acctest"
  ssh_public_key  = "${var.RESOURCES_DIR}/acctest.pub"
  runner_ip =  "${var.RUNNER_IP}"
}

output "host" {
  value     = module.gcp_yb_anywhere.public_ip
  sensitive = true
}

provider "yba" {
  host = module.gcp_yb_anywhere.public_ip
}

resource "yba_installation" "installation" {
  public_ip                 = module.gcp_yb_anywhere.public_ip
  private_ip                = module.gcp_yb_anywhere.private_ip
  ssh_host_ip               = module.gcp_yb_anywhere.private_ip
  ssh_user                  = "tf"
  ssh_private_key           = file("${var.RESOURCES_DIR}/acctest")
  replicated_config_file    = "${var.RESOURCES_DIR}/replicated.conf"
  replicated_license_file   = "${var.RESOURCES_DIR}/acctest.rli"
  application_settings_file = "${var.RESOURCES_DIR}/application_settings.conf"
  timeouts {
    create = "30m"
  }
}

resource "yba_customer_resource" "customer" {
  depends_on = [yba_installation.installation]
  code       = "admin"
  email      = "tf@yugabyte.com"
  name       = "acctest"
}

output "api_key" {
  value     = yba_customer_resource.customer.api_token
  sensitive = true
}
