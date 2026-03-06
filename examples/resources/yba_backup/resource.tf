# On-demand backup of specific database
resource "yba_backup" "my_backup" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["my_database"]
  storage_config_uuid = "<storage-config-uuid>"
  backup_type         = "PGSQL_TABLE_TYPE"
  time_before_delete  = "168h" # Auto-delete after 7 days
}

# Full universe backup (all databases)
resource "yba_backup" "full_backup" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = [] # Empty = full universe backup
  storage_config_uuid = "<storage-config-uuid>"
  backup_type         = "PGSQL_TABLE_TYPE"
}

# Incremental backup (requires YB-Controller enabled universe)
resource "yba_backup" "incremental" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["my_database"]
  storage_config_uuid = "<storage-config-uuid>"
  backup_type         = "PGSQL_TABLE_TYPE"
  base_backup_uuid    = yba_backup.full_backup.id
}
