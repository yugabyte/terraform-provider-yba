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

variable "RESOURCES_DIR" {
  type        = string
  description = "directory on the platform runner that holds testing resources"
}
variable "PORTAL_PASSWORD" {
  type        = string
  description = "password for the management portal"
}

resource "random_uuid" "random" {
}

provider "google" {}

module "gcp_yb_anywhere" {
  source = "../modules/docker/gcp"

  cluster_name    = "tf-acctest-${random_uuid.random.result}"
  ssh_user        = "tf"
  network_tags    = ["terraform-acctest-yugaware", "http-server", "https-server"]
  vpc_network     = "default"
  vpc_subnetwork  = "default"
  // files
  ssh_private_key = "${var.RESOURCES_DIR}/acctest"
  ssh_public_key  = "${var.RESOURCES_DIR}/acctest.pub"
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
  ssh_user                  = "tf"
  ssh_private_key           = file("${var.RESOURCES_DIR}/acctest")
  replicated_config_file    = "${var.RESOURCES_DIR}/replicated.conf"
  replicated_license_file   = "${var.RESOURCES_DIR}/acctest.rli"
  application_settings_file = "${var.RESOURCES_DIR}/application_settings.conf"
  cleanup                   = true
}

resource "yb_customer_resource" "customer" {
  depends_on = [yb_installation.installation]
  code       = "admin"
  email      = "tf@yugabyte.com"
  name       = "acctest"
  password   = var.PORTAL_PASSWORD
}

output "api_key" {
  value = yb_customer_resource.customer.api_token
}