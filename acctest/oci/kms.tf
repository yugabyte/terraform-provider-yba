# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Vault and master key for the OCI encryption-at-rest acceptance tests, in
# test_region (OCI_REGION). The tests point YBA at this existing key, so YBA
# creates none. Destroy only schedules the deletion; OCI keeps both for a
# 7-30 day waiting period.

resource "oci_kms_vault" "ear" {
  provider       = oci.test
  compartment_id = var.compartment_ocid
  display_name   = "${var.prefix}-ear"
  vault_type     = "DEFAULT"
}

# YBA accepts an existing key only when it is enabled and AES.
resource "oci_kms_key" "ear" {
  provider            = oci.test
  compartment_id      = var.compartment_ocid
  display_name        = "${var.prefix}-ear-master"
  management_endpoint = oci_kms_vault.ear.management_endpoint
  protection_mode     = "SOFTWARE"

  key_shape {
    algorithm = "AES"
    length    = 32
  }
}
