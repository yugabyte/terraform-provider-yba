# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "tls_private_key" "yba" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Persistent state for YBA, mounted at /opt/yugabyte/data by the user-data
# script. Kept on a separate volume as in the other fixtures. Must be in the
# VM's availability domain.
resource "oci_core_volume" "data" {
  provider            = oci.home
  compartment_id      = var.compartment_ocid
  availability_domain = data.oci_identity_availability_domains.home.availability_domains[0].name
  display_name        = "${var.prefix}-yba-data"
  size_in_gbs         = 250
}

resource "oci_core_instance" "yba" {
  provider            = oci.home
  compartment_id      = var.compartment_ocid
  availability_domain = data.oci_identity_availability_domains.home.availability_domains[0].name
  display_name        = "${var.prefix}-yba"
  shape               = var.yba_shape

  shape_config {
    ocpus         = var.yba_ocpus
    memory_in_gbs = var.yba_memory_in_gbs
  }

  source_details {
    source_type             = "image"
    source_id               = data.oci_core_images.ol9.images[0].id
    boot_volume_size_in_gbs = 100
  }

  create_vnic_details {
    subnet_id        = oci_core_subnet.yba.id
    assign_public_ip = "true"
  }

  # user_data runs the mount/preflight script on first boot (cloud-init runs an
  # executable user_data payload directly).
  metadata = {
    ssh_authorized_keys = tls_private_key.yba.public_key_openssh
    user_data           = base64encode(file("${path.module}/../resources/oci-mount-data-disk.sh"))
  }
}

# Attach the data volume; the user-data script resolves it as the non-root
# block device.
resource "oci_core_volume_attachment" "data" {
  provider        = oci.home
  attachment_type = "paravirtualized"
  instance_id     = oci_core_instance.yba.id
  volume_id       = oci_core_volume.data.id
}

resource "random_password" "customer" {
  length           = 16
  min_lower        = 1
  min_upper        = 1
  min_numeric      = 1
  min_special      = 1
  override_special = "!#$%*-_"
}

resource "yba_installer" "install" {
  provider = yba.bootstrap

  ssh_host_ip               = oci_core_instance.yba.public_ip
  ssh_user                  = var.yba_admin_user
  ssh_private_key           = tls_private_key.yba.private_key_openssh
  yba_license_file          = "${path.module}/../../yugabyte_anywhere.lic"
  application_settings_file = "${path.module}/../resources/yba-ctl.yml"
  yba_version               = var.yba_version
  host_os                   = "linux"
  host_architecture         = "x86_64"

  depends_on = [
    oci_core_volume_attachment.data,
    oci_core_route_table.main,
    oci_core_security_list.main,
  ]

  lifecycle {
    replace_triggered_by = [oci_core_instance.yba.id]
  }
}

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
