# Encryption-at-rest configuration on a key of the Vault transit secrets
# engine, with a Vault token for authentication. When the key does not exist,
# YugabyteDB Anywhere creates it.
resource "yba_hashicorp_vault_ear_config" "token" {
  name          = "vault-prod"
  vault_address = "https://vault.example.com:8200"
  token         = var.vault_token
  mount_path    = "transit/"
  key_name      = "yb-master-key"
}

# AppRole authentication in a Vault Enterprise namespace. The mount path
# starts with the namespace.
resource "yba_hashicorp_vault_ear_config" "approle" {
  name           = "vault-central"
  vault_address  = "https://vault.example.com:8200"
  role_id        = "8f5ec0a3-6a1f-4d0e-9a52-2d6c3f1b7e44"
  secret_id      = var.vault_secret_id
  auth_namespace = "yb"
  mount_path     = "yb/transit/"
  key_name       = "yb-master-key"
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_hashicorp_vault_ear_config.token.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
