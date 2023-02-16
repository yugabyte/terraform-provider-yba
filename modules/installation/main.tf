terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "yb" {
  alias = "unauthenticated"
  host = "${var.public_ip}:80"
}

resource "yb_installation" "installation" {
  provider                  = yb.unauthenticated
  public_ip                 = var.public_ip
  private_ip                = var.private_ip
  ssh_host_ip               = var.ssh_host_ip != "" ? var.ssh_host_ip : var.public_ip
  ssh_user                  = var.ssh_user
  ssh_private_key           = file(var.ssh_private_key_file)
  replicated_config_file    = "${var.replicated_directory}/replicated.conf"
  replicated_license_file   = var.replicated_license_file_path
  application_settings_file = "${var.replicated_directory}/application_settings.conf"
}