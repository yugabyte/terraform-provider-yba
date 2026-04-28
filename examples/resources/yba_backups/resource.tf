# Deprecated: yba_backups remains supported through the v1.x line and is scheduled
# for removal in v2.0.0. For new configurations, use yba_backup_schedule (note that
# the field "keyspace" is renamed to "keyspaces" and takes a list).
# See the "Upgrading to v1.0.0" guide for migration steps.

resource "yba_backups" "universe_backup_schedule" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["<keyspace-name>"]
  storage_config_uuid = "<storage-config-uuid>"
  frequency           = "120m"
  schedule_name       = "<schedule-name>"
  backup_type         = "PGSQL_TABLE_TYPE"
}
