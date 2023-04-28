resource "yb_restore" "restore" {
  // information of backups can be retrieved from yb_backup_info
  universe_uuid       = "<target-universe-uuid>"
  keyspace            = "<keyspace-name>"
  storage_location    = "<storage-location-of-backup>"
  restore_type        = "<table-type>"
  storage_config_uuid = "<storage-config-uuid-of-backup>"
}