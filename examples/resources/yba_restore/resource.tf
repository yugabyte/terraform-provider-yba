# Restore a backup to a universe
resource "yba_restore" "example" {
  universe_uuid       = "<target-universe-uuid>"
  storage_config_uuid = "<storage-config-uuid>"

  backup_storage_info {
    storage_location = "<storage-location-from-backup>"
    keyspace         = "my_database"
    backup_type      = "PGSQL_TABLE_TYPE"
  }
}

# Restore multiple keyspaces
resource "yba_restore" "multi_keyspace" {
  universe_uuid       = "<target-universe-uuid>"
  storage_config_uuid = "<storage-config-uuid>"

  backup_storage_info {
    storage_location = "<storage-location-1>"
    keyspace         = "database_1"
    backup_type      = "PGSQL_TABLE_TYPE"
  }

  backup_storage_info {
    storage_location = "<storage-location-2>"
    keyspace         = "database_2"
    backup_type      = "PGSQL_TABLE_TYPE"
  }
}
