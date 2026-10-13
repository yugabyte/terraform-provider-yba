# Encryption-at-rest configuration on an existing Azure Key Vault key, with a
# service principal for authentication.
resource "yba_azure_ear_config" "client_secret" {
  name          = "azure-kv-prod"
  client_id     = "00000000-0000-0000-0000-000000000001"
  client_secret = var.kms_client_secret
  tenant_id     = "00000000-0000-0000-0000-000000000002"
  vault_url     = "https://yb-prod-vault.vault.azure.net/"
  key_name      = "yb-master-key"
}

# Authentication as the YugabyteDB Anywhere host: no client secret in
# Terraform or in state. client_id is the client ID of a managed identity on
# the YugabyteDB Anywhere VM. When the key does not exist, YugabyteDB Anywhere
# creates it as an RSA key of key_size bits.
resource "yba_azure_ear_config" "managed_identity" {
  name      = "azure-kv-central"
  client_id = "00000000-0000-0000-0000-000000000003"
  tenant_id = "00000000-0000-0000-0000-000000000002"
  vault_url = "https://yb-central-vault.vault.azure.net/"
  key_name  = "yb-master-key"
  key_size  = 4096
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_azure_ear_config.client_secret.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
