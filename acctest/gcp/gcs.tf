# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Backup bucket for the GCS storage-config acceptance tests.
#

resource "google_storage_bucket" "backups" {
  # Bucket names are globally unique, so qualify with the project.
  name                        = "${var.gcp_project_id}-${var.prefix}-backups"
  location                    = var.gcp_region
  uniform_bucket_level_access = true
  force_destroy               = true
}
