# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# The acceptance-test env as KEY='value' lines, ready to source into a shell.
# Holds the base topology (TF_VAR_GCP_*) and the YBA endpoint. `make acctest`
# sources it; `make -C acctest push-github-secrets` publishes it (CI writes it
# back to acctest/env and sources it the same way).
#
# The YBA endpoint is exported twice. Un-prefixed YBA_HOST/YBA_API_KEY is the
# shared client read by the package init and used by the non-provider tests
# (storage-config, cloud_provider, user, customer). The prefixed
# TF_VAR_GCP_YBA_HOST/TF_VAR_GCP_YBA_API_KEY points the GCP provider tests at
# this same YBA, mirroring how aws/azure target their own fixture YBAs.
#
# Both are the tunnel-local endpoint (local.yba_api_host): the standing YBA has
# no direct ingress. Every consumer tunnels via acctest/with-yba-tunnel.sh —
# `make acctest` (CI and local) and the apply-gcp/destroy-gcp fixture targets.

locals {
  test_env = <<-EOT
    TF_VAR_GCP_CREDENTIALS='${jsonencode(jsondecode(base64decode(google_service_account_key.yba.private_key)))}'
    TF_VAR_GCP_PROJECT_ID='${var.gcp_project_id}'
    TF_VAR_GCP_VPC_NETWORK='${google_compute_network.main.name}'
    TF_VAR_GCP_REGION='${var.gcp_region}'
    TF_VAR_GCP_SUBNETWORK='${google_compute_subnetwork.ybdb.name}'
    TF_VAR_GCP_IMAGE='${data.google_compute_image.ybdb.self_link}'
    TF_VAR_GCP_YBA_VERSION='${var.yba_version}'
    TF_VAR_GCS_BACKUP_LOCATION='gs://${google_storage_bucket.backups.name}'
    TF_VAR_GCP_YBA_HOST='${local.yba_api_host}'
    TF_VAR_GCP_YBA_API_KEY='${yba_customer_resource.customer.api_token}'
    YBA_HOST='${local.yba_api_host}'
    YBA_API_KEY='${yba_customer_resource.customer.api_token}'
  EOT
}

# The acceptance-test env, read at run time by `make acctest`.
output "test_env" {
  description = "Acceptance-test env (TF_VAR_GCP_*, YBA endpoint) as KEY='value' lines."
  value       = local.test_env
  sensitive   = true # contains GCP_CREDENTIALS and YBA API tokens
}

output "yba_url" {
  description = "YBA UI/API endpoint; reachable only while an IAP tunnel is up (acctest/with-yba-tunnel.sh)."
  value       = "https://${local.yba_api_host}"
}

output "yba_username" {
  description = "Username (email) of the initial YBA superuser."
  value       = var.yba_username
}

output "yba_password" {
  description = "Password of the initial YBA superuser."
  value       = random_password.customer.result
  sensitive   = true
}

