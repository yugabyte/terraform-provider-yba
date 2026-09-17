# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Cloud KMS key for the encryption-at-rest acceptance tests (yba_gcp_ear_config
# and the universe encryption_at_rest block). The tests point YBA at this
# existing key ring and crypto key and never create KMS resources themselves.

resource "google_project_service" "cloudkms" {
  service            = "cloudkms.googleapis.com"
  disable_on_destroy = false
}

# GCP never deletes key rings; destroying the fixture only drops it from state.
resource "google_kms_key_ring" "ear" {
  name       = "${var.prefix}-ear"
  location   = var.gcp_region
  depends_on = [google_project_service.cloudkms]
}

# YBA accepts an existing key only with purpose ENCRYPT_DECRYPT, manual
# rotation (no rotation_period) and an enabled primary version, and records
# the key's actual protection level on the config.
resource "google_kms_crypto_key" "ear" {
  name     = "${var.prefix}-ear-master"
  key_ring = google_kms_key_ring.ear.id
  purpose  = "ENCRYPT_DECRYPT"

  version_template {
    algorithm        = "GOOGLE_SYMMETRIC_ENCRYPTION"
    protection_level = "SOFTWARE"
  }
}
