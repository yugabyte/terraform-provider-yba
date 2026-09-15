# Look up an encryption-at-rest configuration by name, for example one created
# in the YugabyteDB Anywhere UI, and attach it to a universe.
data "yba_ear_config" "ui_created" {
  name = "gcp-kms-prod"
}

resource "yba_universe" "encrypted" {
  encryption_at_rest {
    kms_config_uuid = data.yba_ear_config.ui_created.uuid
  }
  # ... clusters, communication_ports, ...
}

output "ear_config_provider" {
  value = data.yba_ear_config.ui_created.key_provider
}

# The UUID also drives an import into the matching resource, to start
# managing the configuration in Terraform:
#   terraform import yba_gcp_ear_config.prod <uuid>
output "ear_config_uuid" {
  value = data.yba_ear_config.ui_created.uuid
}
