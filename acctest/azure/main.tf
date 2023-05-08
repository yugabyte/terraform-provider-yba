terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
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

resource "random_uuid" "random" {
}

provider "azurerm" {
  features {}
}

module "azure_yb_anywhere" {
  source = "../../modules/docker/azure"

  cluster_name   = "tf-acctest-${random_uuid.random.result}"
  ssh_user       = "tf"
  region_name    = "westus2"
  subnet_name    = "***REMOVED***"
  vnet_name      = "***REMOVED***"
  resource_group = "yugabyte-rg"
  // files
  ssh_private_key = "${var.RESOURCES_DIR}/acctest"
  ssh_public_key  = "${var.RESOURCES_DIR}/acctest.pub"
}

output "host" {
  value = module.azure_yb_anywhere.public_ip
}

provider "yb" {
  host = "${module.azure_yb_anywhere.public_ip}:80"
}

resource "yb_installation" "installation" {
  public_ip                 = module.azure_yb_anywhere.public_ip
  private_ip                = module.azure_yb_anywhere.private_ip
  ssh_host_ip               = module.azure_yb_anywhere.public_ip
  ssh_user                  = "tf"
  ssh_private_key           = file("${var.RESOURCES_DIR}/acctest")
  replicated_config_file    = "${var.RESOURCES_DIR}/replicated.conf"
  replicated_license_file   = "${var.RESOURCES_DIR}/acctest.rli"
  application_settings_file = "${var.RESOURCES_DIR}/application_settings.conf"
}

resource "yb_customer_resource" "customer" {
  depends_on = [yb_installation.installation]
  code       = "admin"
  email      = "tf@yugabyte.com"
  name       = "acctest"
}

output "api_key" {
  value = yb_customer_resource.customer.api_token
}