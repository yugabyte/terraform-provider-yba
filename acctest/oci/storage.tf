# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Backup buckets for the OCI storage-config acceptance tests, one per test
# region, reached through the S3-compatible API with a customer secret key of
# the API-key user. OCI deletes only an empty bucket.

data "oci_objectstorage_namespace" "main" {
  provider = oci.home
}

locals {
  namespace = data.oci_objectstorage_namespace.main.namespace
}

# OCI_REGION
resource "oci_objectstorage_bucket" "backups" {
  provider       = oci.test
  compartment_id = var.compartment_ocid
  namespace      = local.namespace
  name           = "${var.prefix}-backups-${var.test_region}"
}

# OCI_REGION_2
resource "oci_objectstorage_bucket" "backups_2" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  namespace      = local.namespace
  name           = "${var.prefix}-backups-${var.home_region}"
}

resource "oci_identity_domains_customer_secret_key" "yba" {
  provider      = oci.home
  idcs_endpoint = local.idcs_endpoint
  schemas       = ["urn:ietf:params:scim:schemas:oracle:idcs:customerSecretKey"]
  display_name  = "${var.prefix}-yba"

  user {
    value = oci_identity_domains_user.yba.id
  }
}
