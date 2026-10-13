# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# HashiCorp Vault for the yba_hashicorp_vault_ear_config acceptance test: a
# dev-mode server on a small VM in the YBA subnet. YBA reaches it at the VM's
# internal IP. The external IP serves only the download of Vault: the
# firewall admits only VPC and IAP traffic (network.tf).
#
# The token, role ID and secret ID come from this state, and the boot script
# gives them to Vault on every start, so the instance metadata holds them in
# clear. They open only this in-memory test Vault, and their policy covers
# only its transit mount (gcp-vault-bootscript.sh).

locals {
  vault_mount_path = "yb-transit/"
  vault_key_name   = "yba-ear"
}

resource "random_password" "vault_root_token" {
  length  = 32
  special = false
}

resource "random_password" "vault_token" {
  length  = 32
  special = false
}

# YBA lists the role ID in clear, so it is not a secret here either.
resource "random_uuid" "vault_role_id" {}

resource "random_password" "vault_secret_id" {
  length  = 32
  special = false
}

resource "google_compute_instance" "vault" {
  name         = "${var.prefix}-vault"
  machine_type = "e2-small"
  zone         = "${var.gcp_region}-a"

  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
    }
  }

  network_interface {
    network    = google_compute_network.main.id
    subnetwork = google_compute_subnetwork.yba.id

    access_config {}
  }

  # metadata_startup_script replaces the VM when the script changes. The dev
  # server reads the script only when it starts, so an in-place metadata
  # update would leave it on the old credentials and policy.
  metadata_startup_script = templatefile(
    "${path.module}/../resources/gcp-vault-bootscript.sh", {
      vault_version = "1.18.3"
      root_token    = random_password.vault_root_token.result
      token         = random_password.vault_token.result
      role_id       = random_uuid.vault_role_id.result
      secret_id     = random_password.vault_secret_id.result
      mount_path    = local.vault_mount_path
      key_name      = local.vault_key_name
  })
}
