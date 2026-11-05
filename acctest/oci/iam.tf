# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "tls_private_key" "api" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "oci_identity_user" "yba" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${var.prefix}-yba"
  description    = "API key user for yba_oci_provider acceptance tests"
}

resource "oci_identity_api_key" "yba" {
  provider  = oci.home
  user_id   = oci_identity_user.yba.id
  key_value = tls_private_key.api.public_key_pem
}

resource "oci_identity_group" "yba" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${var.prefix}-yba"
  description    = "Acceptance-test API users"
}

resource "oci_identity_user_group_membership" "yba" {
  provider = oci.home
  group_id = oci_identity_group.yba.id
  user_id  = oci_identity_user.yba.id
}

resource "oci_identity_policy" "api" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${var.prefix}-api"
  description    = "Let the API-key user manage the acceptance compartment"
  statements = [
    "Allow group ${oci_identity_group.yba.name} to manage all-resources in compartment id ${var.compartment_ocid}",
  ]
}

resource "oci_identity_dynamic_group" "yba" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${var.prefix}-yba-instance"
  description    = "YBA acceptance-test instance principal"
  matching_rule  = "instance.id = '${oci_core_instance.yba.id}'"
}

resource "oci_identity_policy" "instance" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
  name           = "${var.prefix}-instance"
  description    = "Let the YBA VM manage the acceptance compartment"
  statements = [
    "Allow dynamic-group ${oci_identity_dynamic_group.yba.name} to manage all-resources in compartment id ${var.compartment_ocid}",
  ]
}

data "oci_core_images" "ol9" {
  provider                 = oci.home
  compartment_id           = var.compartment_ocid
  operating_system         = "Oracle Linux"
  operating_system_version = "9"
  shape                    = var.yba_shape
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
  state                    = "AVAILABLE"
}

data "oci_core_images" "region2" {
  provider                 = oci.region2
  compartment_id           = var.compartment_ocid
  operating_system         = "Oracle Linux"
  operating_system_version = "9"
  shape                    = var.yba_shape
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
  state                    = "AVAILABLE"
}

resource "oci_core_instance_configuration" "ybdb" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  display_name   = "${var.prefix}-ybdb"

  instance_details {
    instance_type = "compute"
    launch_details {
      compartment_id = var.compartment_ocid
      display_name   = "${var.prefix}-ybdb"
      shape          = var.yba_shape

      shape_config {
        ocpus         = var.yba_ocpus
        memory_in_gbs = var.yba_memory_in_gbs
      }

      source_details {
        source_type = "image"
        image_id    = data.oci_core_images.ol9.images[0].id
      }

      create_vnic_details {
        subnet_id        = oci_core_subnet.ybdb[0].id
        assign_public_ip = "true"
      }
    }
  }
}
