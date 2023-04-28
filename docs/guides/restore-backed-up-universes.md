---
subcategory: ""
page_title: "Restoring data via YugabyteDB Anywhere Terraform resource"
description: |-
  Using Restore resource to perform one time operations on YugabyteDB Anywhere universes
---

# Restoring data backed up by Scheduled Backup resource

Data backed up by the scheduled backups (*yb_backups*) can be restored to universes using the defined resource for restores - *yb_restore*. This operation only triggers the Restore operation and does not track the remote state once the operation is complete.

~> **Note:** You should remove the *yb_restore* resource after the operation is complete.

You can fetch the list of backups for a universe using the *yb_backup_info* data source, which can be used in restore operations, as shown in the following example.

```terraform
data "yb_backup_info" "backup" {
  universe_uuid = "<universe-uuid>"
}

resource "yb_restore" "restore_ysql" {
  universe_uuid = "<universe-uuid>"
  keyspace = "<new-keyspace-name>"
  storage_location = data.yb_backup_info.backup.storage_location
  restore_type = data.yb_backup_info.backup.backup_type
  parallelism = 8
  storage_config_uuid = data.yb_backup_info.backup.storage_config_uuid
}
```

To fetch backups for a specific date range, specify the *date_range_start* and *date_range_end* in RFC3339 format. The most recent backup in the range is selected and stored in the ID of the data source.
