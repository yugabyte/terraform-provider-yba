---
subcategory: ""
page_title: "Restoring data via YugabyteDB Anywhere Terraform resource"
description: |-
  Using Restore resource to perform one time operations on YugabyteDB Anywhere universes
---

# Restoring Backups

Data backed up by *yba_backup* (on-demand) or *yba_backup_schedule* (scheduled) can be restored to universes using the *yba_restore* resource. This operation triggers the restore and waits for completion.

~> **Note:** You should remove the *yba_restore* resource from your configuration after the operation is complete, as restores are one-time operations.

You can fetch the list of backups for a universe using the *yba_backup_info* data source, which can be used in restore operations, as shown in the following example.

```terraform
data "yba_backup_info" "backup" {
  universe_uuid = "<universe-uuid>"
}

resource "yba_restore" "restore_ysql" {
  universe_uuid       = "<target-universe-uuid>"
  storage_config_uuid = data.yba_backup_info.backup.storage_config_uuid

  backup_storage_info {
    storage_location = data.yba_backup_info.backup.storage_location
    keyspace         = "<target-keyspace-name>"
    backup_type      = data.yba_backup_info.backup.backup_type
  }
}
```

To fetch backups for a specific date range, specify the *date_range_start* and *date_range_end* in RFC3339 format. The most recent backup in the range is selected and stored in the ID of the data source.

For multi-keyspace YCQL backups, each keyspace has its own storage location. Use the *keyspace_details* attribute to reference each one:

```terraform
data "yba_backup_info" "backup" {
  universe_uuid = "<universe-uuid>"
}

# Restore each keyspace from a multi-keyspace YCQL backup
resource "yba_restore" "restore_ycql_multi" {
  universe_uuid       = "<target-universe-uuid>"
  storage_config_uuid = data.yba_backup_info.backup.storage_config_uuid

  dynamic "backup_storage_info" {
    for_each = data.yba_backup_info.backup.keyspace_details
    content {
      storage_location = backup_storage_info.value.storage_location
      keyspace         = backup_storage_info.value.keyspace
      backup_type      = data.yba_backup_info.backup.backup_type
    }
  }
}
```

## Restoring from On-Demand Backups

After creating a backup using the *yba_backup* resource, the backup's per-keyspace storage locations are available directly on the resource via the *keyspace_details* attribute — no separate data source lookup is needed.

```terraform
# Create a backup
resource "yba_backup" "my_backup" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["my_database"]
  storage_config_uuid = "<storage-config-uuid>"
  backup_type         = "PGSQL_TABLE_TYPE"
}

# Restore directly using keyspace_details from the backup resource
resource "yba_restore" "restore_from_backup" {
  universe_uuid       = "<target-universe-uuid>"
  storage_config_uuid = yba_backup.my_backup.storage_config_uuid

  backup_storage_info {
    storage_location = yba_backup.my_backup.keyspace_details[0].storage_location
    keyspace         = "my_database"
    backup_type      = yba_backup.my_backup.keyspace_details[0].backup_type
  }
}
```

For on-demand backups that cover multiple YCQL keyspaces, use a dynamic block to restore all of them:

```terraform
resource "yba_backup" "my_backup" {
  universe_uuid       = "<universe-uuid>"
  storage_config_uuid = "<storage-config-uuid>"
  backup_type         = "YQL_TABLE_TYPE"
}

resource "yba_restore" "restore_all_keyspaces" {
  universe_uuid       = "<target-universe-uuid>"
  storage_config_uuid = yba_backup.my_backup.storage_config_uuid

  dynamic "backup_storage_info" {
    for_each = yba_backup.my_backup.keyspace_details
    content {
      storage_location = backup_storage_info.value.storage_location
      keyspace         = backup_storage_info.value.keyspace
      backup_type      = backup_storage_info.value.backup_type
    }
  }
}
```
