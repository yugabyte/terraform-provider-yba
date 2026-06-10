# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# The acceptance-test YBA: a control-plane VM, the YBA install (over SSH), and
# the initial customer. Applied once as part of the base; its endpoint
# (YBA_HOST/YBA_API_KEY) is exposed through the `test_env` output so
# `make acctest` just consumes it. Tear down with the base.

locals {
  ssh_private_key_file = pathexpand(var.ssh_private_key_file)
  ssh_public_key_file  = "${local.ssh_private_key_file}.pub"
  yba_ssh_host         = google_compute_address.yba.address
}

# Resolve base_image (a family reference) to a concrete image. YBA's image-bundle
# validation needs a specific image URI, not a family. Exposed as TF_VAR_GCP_IMAGE.
data "google_compute_image" "ybdb" {
  project = regex("projects/([^/]+)/", var.base_image)[0]
  family  = regex("/family/(.+)$", var.base_image)[0]
}

resource "google_compute_address" "yba" {
  name         = "${var.prefix}-yba"
  region       = var.gcp_region
  address_type = "EXTERNAL"
}

# Persistent state for YBA, mounted at /opt/yugabyte/data by the startup
# script. Kept on a separate disk as in byoc-setup.
resource "google_compute_disk" "data" {
  name = "${var.prefix}-yba-data"
  zone = "${var.gcp_region}-a"
  type = "pd-balanced"
  size = 250
}

# Single YBA control-plane VM (no HA).
resource "google_compute_instance" "yba" {
  name         = "${var.prefix}-yba"
  machine_type = var.yba_machine_type
  zone         = "${var.gcp_region}-a"

  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = var.base_image
      size  = 100
      type  = "pd-balanced"
    }
  }

  # device_name must match the VM hostname; the startup script resolves the
  # data disk at /dev/disk/by-id/google-$(hostname -s).
  attached_disk {
    source      = google_compute_disk.data.self_link
    device_name = "${var.prefix}-yba"
  }

  network_interface {
    network    = google_compute_network.main.id
    subnetwork = google_compute_subnetwork.yba.id

    access_config {
      nat_ip = google_compute_address.yba.address
    }
  }

  # Standard gcloud keypair (matches the private key the installer uses).
  metadata = {
    ssh-keys       = "yugabyte:${file(local.ssh_public_key_file)}"
    startup-script = file("${path.module}/../resources/gcp-bootscript.sh")
  }

  service_account {
    email  = google_service_account.yba.email
    scopes = ["cloud-platform"]
  }
}

# Randomly generated password for the initial YBA superuser.
resource "random_password" "customer" {
  length           = 16
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
  override_special = "!#$%*-_"
}

# Install YugabyteDB Anywhere on the VM over SSH. The provider takes file
# paths (validated to exist at plan time): the SSH key is the standard gcloud
# keypair, the license is at the repo root, and yba-ctl.yml is in acctest/resources.
resource "yba_installer" "install" {
  provider = yba.bootstrap

  ssh_host_ip               = local.yba_ssh_host
  ssh_user                  = "yugabyte"
  ssh_private_key_file_path = local.ssh_private_key_file
  yba_license_file          = "${path.module}/../../yugabyte_anywhere.lic"
  application_settings_file = "${path.module}/../resources/yba-ctl.yml"
  yba_version               = var.yba_version
  host_os                   = "linux"
  host_architecture         = "x86_64"

  # The instance is an implicit dependency (via ssh_host_ip), but the firewall
  # that opens SSH/443 is not — without this the installer can start before
  # the rule exists and fail to connect.
  depends_on = [google_compute_firewall.operator]
}

# Register the initial superuser; exposes the API token (published as YBA_API_KEY).
resource "yba_customer_resource" "customer" {
  provider = yba.bootstrap

  code     = "admin"
  email    = var.yba_username
  name     = "admin"
  password = random_password.customer.result

  lifecycle {
    ignore_changes = [password]
  }

  depends_on = [yba_installer.install]
}
