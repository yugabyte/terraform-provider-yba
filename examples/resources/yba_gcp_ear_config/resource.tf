# Encryption-at-rest configuration on an existing Cloud KMS crypto key, with a
# service-account key for authentication. The project_id in the key file names
# the key ring's project.
resource "yba_gcp_ear_config" "service_account" {
  name          = "gcp-kms-prod"
  credentials   = file("${path.module}/kms-service-account.json")
  location_id   = "us-east1"
  key_ring_id   = "yugabyte-ring"
  crypto_key_id = "yugabyte-master-key"
}

# Authentication as the YugabyteDB Anywhere host (attached service account,
# workload identity, or GOOGLE_APPLICATION_CREDENTIALS): no key file in
# Terraform or in state. The key ring is in another project, so project_id
# names it. YugabyteDB Anywhere creates the key ring and an HSM crypto key when
# they do not exist. use_gcp_iam and project_id work only with YugabyteDB
# Anywhere preview releases.
resource "yba_gcp_ear_config" "host_identity" {
  name             = "gcp-kms-central"
  use_gcp_iam      = true
  project_id       = "security-kms-project"
  location_id      = "global"
  key_ring_id      = "central-ring"
  crypto_key_id    = "yugabyte-master-key"
  protection_level = "HSM"

  # Optional: a custom Cloud KMS endpoint, for Private Service Connect or a
  # restricted VIP.
  kms_endpoint = "kms.example.internal:443"
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_gcp_ear_config.service_account.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
