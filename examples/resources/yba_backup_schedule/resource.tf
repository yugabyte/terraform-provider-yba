# Schedule backups using frequency
resource "yba_backup_schedule" "hourly" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["my_database"]
  storage_config_uuid = "<storage-config-uuid>"
  frequency           = "1h"
  schedule_name       = "hourly-backup"
  backup_type         = "PGSQL_TABLE_TYPE"
}

# Schedule backups using cron expression
resource "yba_backup_schedule" "daily_cron" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["db1", "db2"]
  storage_config_uuid = "<storage-config-uuid>"
  cron_expression     = "0 2 * * *" # Daily at 2 AM
  schedule_name       = "daily-backup"
  backup_type         = "PGSQL_TABLE_TYPE"
  time_before_delete  = "168h" # 7 days retention
}

# Full universe backup (all databases)
resource "yba_backup_schedule" "full_universe" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = [] # Empty list = full universe backup
  storage_config_uuid = "<storage-config-uuid>"
  frequency           = "24h"
  schedule_name       = "full-universe-daily"
  backup_type         = "PGSQL_TABLE_TYPE"
}
