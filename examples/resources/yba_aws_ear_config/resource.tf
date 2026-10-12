# Encryption-at-rest configuration on an existing AWS KMS key, with access keys
# for authentication.
resource "yba_aws_ear_config" "access_keys" {
  name              = "aws-kms-prod"
  access_key_id     = var.kms_access_key_id
  secret_access_key = var.kms_secret_access_key
  region            = "us-west-2"
  cmk_id            = "arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"

  # Optional: a custom AWS KMS endpoint, for example a VPC endpoint.
  kms_endpoint = "https://vpce-0abc123-kms.kms.us-west-2.vpce.amazonaws.com"
}

# Authentication as the YugabyteDB Anywhere host (the instance profile on
# EC2): no access keys in Terraform or in state. The KMS key must be in the
# host's region. Without cmk_id, YugabyteDB Anywhere creates the KMS key.
resource "yba_aws_ear_config" "host_identity" {
  name   = "aws-kms-central"
  region = "us-east-1"
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_aws_ear_config.access_keys.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
