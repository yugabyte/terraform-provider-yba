# Encryption-at-rest configuration on an existing Cloud KMS crypto key,
# authenticating with a service-account key. The key file's project_id names
# the key ring's project.
resource "yba_gcp_ear_config" "service_account" {
  name          = "gcp-kms-prod"
  credentials   = file("~/.gcp/kms-service-account.json")
  location_id   = "us-east1"
  key_ring_id   = "yugabyte-ring"
  crypto_key_id = "yugabyte-master-key"
}

# The same key ring reached with the YugabyteDB Anywhere host's own identity
# (attached service account, workload identity, or GOOGLE_APPLICATION_CREDENTIALS):
# no key file in Terraform or in state. The key ring lives in another project,
# so project_id names it explicitly. See the resource description for the
# YugabyteDB Anywhere version this needs.
resource "yba_gcp_ear_config" "host_identity" {
  name          = "gcp-kms-central"
  use_gcp_iam   = true
  project_id    = "security-kms-project"
  location_id   = "global"
  key_ring_id   = "central-ring"
  crypto_key_id = "yugabyte-master-key"

  # Optional: protection level for a key YugabyteDB Anywhere creates, and a
  # custom Cloud KMS endpoint (Private Service Connect / restricted VIP).
  protection_level = "HSM"
  kms_endpoint     = "kms.example.internal:443"
}

# A universe attaches a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_gcp_ear_config.service_account.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
