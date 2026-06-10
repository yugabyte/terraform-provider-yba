# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# IAM for the GCP acceptance-test fixture. A single service account, scoped to
# the control-plane job on the YBA VM so it holds only the privileges that job
# needs:
#
#   yba — control plane on the YBA VM. Compute + storage.

# Attached to the YBA VM. YBA uses it to provision universe nodes and write
# backups, so it needs compute + storage.
resource "google_service_account" "yba" {
  account_id   = "${var.prefix}-yba"
  display_name = "YBA acceptance-test control plane"
}

# A key for the yba SA, minted for the tests (passed to yba_gcp_provider
# credentials and the GCS storage-config test). Surfaced via the test_env output
# (out-vars.tf), destroyed on teardown. The only long-lived credential here.
resource "google_service_account_key" "yba" {
  service_account_id = google_service_account.yba.name
}

resource "google_project_iam_member" "compute_admin" {
  project = var.gcp_project_id
  role    = "roles/compute.admin"
  member  = "serviceAccount:${google_service_account.yba.email}"
}

# Scoped to the backups bucket only (not project-wide) — the SA just needs to
# read/write backups there for the GCS storage-config test.
resource "google_storage_bucket_iam_member" "backups_admin" {
  bucket = google_storage_bucket.backups.name
  role   = "roles/storage.admin"
  member = "serviceAccount:${google_service_account.yba.email}"
}

# YBA provisions universe VMs and attaches itself as their runtime SA, which
# requires yba to actAs itself.
resource "google_service_account_iam_member" "yba_acts_as_self" {
  service_account_id = google_service_account.yba.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.yba.email}"
}
