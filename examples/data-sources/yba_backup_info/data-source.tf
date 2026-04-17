data "yba_backup_info" "backup" {
  universe_uuid = "universe-having-backups-uuid"
}

# The backup_category field tells you whether it is a full or incremental backup.
output "backup_category" {
  value = data.yba_backup_info.backup.backup_category
}

# incremental_backup_chain contains all incremental backups in the chain (oldest first).
output "incremental_chain_uuids" {
  value = [for b in data.yba_backup_info.backup.incremental_backup_chain : b.backup_uuid]
}
