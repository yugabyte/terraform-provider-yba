terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
    }
    yba = {
      version = "0.1.0-dev"
      source  = "yugabyte/yba"
    }
  }
}

variable "RESOURCES_DIR" {
  type        = string
  description = "directory on the platform runner that holds testing resources"
}

variable "AZURE_SG_ID" {
  type        = string
  description = "Azure security ID to run acceptance testing"
}

variable "AZURE_SUBNET_ID" {
  type        = string
  description = "Azure subnet ID to run acceptance testing"
}

variable "AZURE_VNET_ID" {
  type        = string
  description = "Azure vnet ID to run acceptance testing"
}

variable "AZURE_RG" {
  type        = string
  description = "Azure resource group to run acceptance testing"
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
  security_group = var.AZURE_SG_ID
  subnet_name    = var.AZURE_SUBNET_ID
  vnet_name      = var.AZURE_VNET_ID
  resource_group = var.AZURE_RG
  // files
  ssh_private_key = "${var.RESOURCES_DIR}/acctest"
  ssh_public_key  = "${var.RESOURCES_DIR}/acctest.pub"
}

output "host" {
  value     = module.azure_yb_anywhere.public_ip
  sensitive = true
}

provider "yba" {
  host = module.azure_yb_anywhere.public_ip
}

resource "yba_installation" "installation" {
  depends_on                = [module.azure_yb_anywhere]
  public_ip                 = module.azure_yb_anywhere.public_ip
  private_ip                = module.azure_yb_anywhere.private_ip
  ssh_host_ip               = module.azure_yb_anywhere.public_ip
  ssh_user                  = "tf"
  ssh_private_key           = file("${var.RESOURCES_DIR}/acctest")
  replicated_config_file    = "${var.RESOURCES_DIR}/replicated.conf"
  replicated_license_file   = "${var.RESOURCES_DIR}/acctest.rli"
  application_settings_file = "${var.RESOURCES_DIR}/application_settings.conf"
  timeouts {
    create = "15m"
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
