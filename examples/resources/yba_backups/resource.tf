# Deprecated: Use yba_backup_schedule instead
resource "yba_backups" "universe_backup_schedule" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["<keyspace-name>"]
  storage_config_uuid = "<storage-config-uuid>"
  frequency           = "120m"
  schedule_name       = "<schedule-name>"
  backup_type         = "PGSQL_TABLE_TYPE"
}
