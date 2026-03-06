---
subcategory: ""
page_title: "Creating Backup Schedules via YugabyteDB Anywhere Terraform resource"
description: |-
  Creating Backup Schedules on YugabyteDB Anywhere universes
---

# Creating Backup Schedules via YugabyteDB Anywhere Terraform resource

You can schedule backups using the following definition after configuring a storage configuration resource (refer to *yba_storage_config_resource*).

```terraform
resource "yba_storage_config_resource" "storage" {
  name = "S3"
  backup_location = "<s3-backup-bucket-location>"
  config_name  = "<config-name>"
}

data "yba_storage_configs" "configs" {
  config_name = yba_storage_config_resource.storage.config_name
}

resource "yba_backup_schedule" "universe_backup_schedule" {

  universe_uuid = "<universe-uuid>"
  keyspaces = ["<keyspace-name>"]
  storage_config_uuid = data.yba_storage_configs.configs.id
  time_before_delete = "24h"
  sse = false
  transactional_backup = false
  delete_backup = true
  frequency = "1h"
  parallelism = 8
  schedule_name = "<schedule-name>"
  backup_type ="<backup-table-type>"
}
```
