# Look up an encryption-at-rest configuration by name, for example one created
# in the YugabyteDB Anywhere UI.
data "yba_ear_config" "ui_created" {
  name = "gcp-kms-prod"
}

# A universe attaches it through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = data.yba_ear_config.ui_created.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.

output "ear_config_provider" {
  value = data.yba_ear_config.ui_created.key_provider
}

# The UUID also drives an import into yba_gcp_ear_config, to start managing
# the configuration in Terraform:
#   terraform import yba_gcp_ear_config.prod <uuid>
output "ear_config_uuid" {
  value = data.yba_ear_config.ui_created.uuid
}
