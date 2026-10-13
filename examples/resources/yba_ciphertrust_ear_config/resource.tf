# Encryption-at-rest configuration on an existing AES key in CipherTrust
# Manager, with a user name and password for authentication. CipherTrust KMS
# must be allowed in YugabyteDB Anywhere, on the global scope.
resource "yba_runtime_config" "allow_ciphertrust" {
  key   = "yb.kms.allow_ciphertrust"
  value = "true"
}

resource "yba_ciphertrust_ear_config" "password" {
  name        = "ciphertrust-prod"
  manager_url = "https://ciphertrust.example.com"
  auth_type   = "PASSWORD"
  username    = "yba-kms"
  password    = var.ciphertrust_password
  key_name    = "yb-master-key"

  depends_on = [yba_runtime_config.allow_ciphertrust]
}

# Authentication with a refresh token. When the key does not exist,
# YugabyteDB Anywhere creates it as an AES key of key_size bits.
resource "yba_ciphertrust_ear_config" "refresh_token" {
  name          = "ciphertrust-central"
  manager_url   = "https://ciphertrust.example.com"
  auth_type     = "REFRESH_TOKEN"
  refresh_token = var.ciphertrust_refresh_token
  key_name      = "yb-central-key"
  key_size      = 128

  depends_on = [yba_runtime_config.allow_ciphertrust]
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_ciphertrust_ear_config.password.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
