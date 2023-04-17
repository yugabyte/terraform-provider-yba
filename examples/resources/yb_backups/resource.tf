resource "yb_backups" "universe_backup_schedule" {
    universe_uuid        = "<universe-uuid>"
    keyspace             = "<keyspace name>"
    storage_config_uuid  = "<storage-config-uuid>"
    frequency            = "120m"
    schedule_name        = "<schedule-name>"
    backup_type          = "<table-type>"
}

resource "yb_backups" "universe_backup_schedule_detailed" {
    universe_uuid        = "<universe-uuid>"
    keyspace             = "<keyspace name>"
    storage_config_uuid  = "<storage-config-uuid>"
    time_before_delete   = "24h"
    sse                  = false
    transactional_backup = false
    frequency            = "120m"
    parallelism          = 8
    delete_backup        = true
    schedule_name        = "<schedule-name>"
    backup_type          = "<table-type>"
}