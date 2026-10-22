# Encryption-at-rest configuration on an existing OCI Vault key, with an API
# signing key for authentication.
resource "yba_oci_ear_config" "api_key" {
  name                = "oci-kms-prod"
  tenancy_id          = "ocid1.tenancy.oc1..aaaa"
  user_id             = "ocid1.user.oc1..aaaa"
  fingerprint         = "12:34:56:78:9a:bc:de"
  private_key_content = file("${path.module}/oci_api_key.pem")
  compartment_id      = "ocid1.compartment.oc1..aaaa"
  vault_id            = "ocid1.vault.oc1.iad.aaaa"
  region              = "us-ashburn-1"
  key_name            = "yugabyte-master-key"
}

# Authentication as the YugabyteDB Anywhere host (instance principal): no API
# signing key in Terraform or in state. The host must be an OCI compute
# instance in a dynamic group that can use the vault and the key. YugabyteDB
# Anywhere creates an AES-256 key when key_name is not already in the
# compartment. Requires YugabyteDB Anywhere 2026.1.2.0 or later.
resource "yba_oci_ear_config" "host_identity" {
  name                   = "oci-kms-central"
  use_instance_principal = true
  compartment_id         = "ocid1.compartment.oc1..aaaa"
  vault_id               = "ocid1.vault.oc1.iad.bbbb"
  region                 = "us-ashburn-1"
  key_name               = "yugabyte-master-key"
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_oci_ear_config.api_key.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
