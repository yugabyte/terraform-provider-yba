---
subcategory: ""
page_title: "Creating Backup Schedules via YugabyteDB Anywhere Terraform resource"
description: |-
  Creating Backup Schedules on YugabyteDB Anywhere universes
---

# Creating Backup Schedules via YugabyteDB Anywhere Terraform resource

Use the `yba_backup_schedule` resource to configure automated recurring backups for a universe.
Combine it with a storage configuration resource (`yba_s3_storage_config`, `yba_gcs_storage_config`,
`yba_azure_storage_config`, or `yba_nfs_storage_config`) that specifies where backups are stored.

For more details, see the [YugabyteDB Anywhere Schedule Data Backups](https://docs.yugabyte.com/stable/yugabyte-platform/back-up-restore-universes/schedule-data-backups/ysql/)
documentation.

## Basic schedule with frequency

Create an S3 storage configuration and schedule daily backups of a YSQL database:

```terraform
resource "yba_s3_storage_config" "s3" {
  name              = "my-s3-config"
  backup_location   = "s3://my-bucket/yugabyte-backups"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"
}

resource "yba_backup_schedule" "daily" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["my_database"]
  storage_config_uuid = yba_s3_storage_config.s3.config_uuid
  backup_type         = "PGSQL_TABLE_TYPE"
  frequency           = "24h"
  time_before_delete  = "168h"  # retain for 7 days
  schedule_name       = "daily-ysql-backup"
  delete_backup       = true    # remove backup data when schedule is deleted
}
```

## Schedule with Cron Expression

Use a cron expression for fine-grained scheduling, such as daily at 2 AM UTC:

```terraform
resource "yba_backup_schedule" "nightly" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["db1", "db2"]
  storage_config_uuid = yba_s3_storage_config.s3.config_uuid
  backup_type         = "PGSQL_TABLE_TYPE"
  cron_expression     = "0 2 * * *"
  schedule_name       = "nightly-2am"
  time_before_delete  = "720h"  # retain for 30 days
}
```

## Full Universe Backup

Set `keyspaces` to an empty list to back up every database/keyspace in the universe. YBA resolves
the database list at each run, so databases added or dropped after `terraform apply` are picked up
automatically on the next backup:

```terraform
resource "yba_backup_schedule" "full_universe" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = []   # empty = full universe backup
  storage_config_uuid = yba_s3_storage_config.s3.config_uuid
  backup_type         = "PGSQL_TABLE_TYPE"
  frequency           = "24h"
  schedule_name       = "full-universe-daily"
}
```

## Schedule with incremental backups

Schedule full and incremental backups to increase frequency while conserving disk space.

```terraform
resource "yba_backup_schedule" "incremental" {
  universe_uuid                = "<universe-uuid>"
  keyspaces                    = ["my_database"]
  storage_config_uuid          = yba_s3_storage_config.s3.config_uuid
  backup_type                  = "PGSQL_TABLE_TYPE"
  frequency                    = "24h"     # full backup every 24h
  incremental_backup_frequency = "1h"      # incremental every 1h
  schedule_name                = "incremental-backup"
}
```

## Dynamic schedule from universe schema

Use `yba_universe_schema` when you want one backup entry per database (rather than a single
full-universe backup), but still don't want to hardcode database names. The data source
enumerates the databases at `terraform apply` time. New databases created later are not included
until you re-run `terraform apply`:

```terraform
data "yba_universe_schema" "schema" {
  universe_uuid = "<universe-uuid>"
  table_type    = "PGSQL_TABLE_TYPE"
}

resource "yba_backup_schedule" "all_databases" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = data.yba_universe_schema.schema.ysql_database_names
  storage_config_uuid = yba_s3_storage_config.s3.config_uuid
  backup_type         = "PGSQL_TABLE_TYPE"
  frequency           = "24h"
  schedule_name       = "all-databases-daily"
}
```

## Pause and Resume a Schedule

Set `enabled = false` to pause a schedule without deleting it:

```terraform
resource "yba_backup_schedule" "daily" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["my_database"]
  storage_config_uuid = yba_s3_storage_config.s3.config_uuid
  backup_type         = "PGSQL_TABLE_TYPE"
  frequency           = "24h"
  schedule_name       = "daily-backup"
  enabled             = false  # paused
}
```

Set `enabled = true` (or remove the field, since the default is true) and apply to resume.
Set `run_backup_on_enable = true` to trigger an immediate backup on resume if the next
scheduled time has already passed.

## Migrating from yba_backups

If you have an existing configuration that uses the deprecated `yba_backups` resource, migrate
to `yba_backup_schedule` by updating the resource type and renaming `keyspace` (string) to
`keyspaces` (list):

```terraform
# Before (deprecated)
resource "yba_backups" "example" {
  universe_uuid       = "<universe-uuid>"
  keyspace            = "my_keyspace"
  storage_config_uuid = "<storage-config-uuid>"
  frequency           = "24h"
  schedule_name       = "my-schedule"
  backup_type         = "PGSQL_TABLE_TYPE"
}

# After
resource "yba_backup_schedule" "example" {
  universe_uuid       = "<universe-uuid>"
  keyspaces           = ["my_keyspace"]   # changed from string to list
  storage_config_uuid = "<storage-config-uuid>"
  frequency           = "24h"
  schedule_name       = "my-schedule"
  backup_type         = "PGSQL_TABLE_TYPE"
}
```

Migrate the existing state using `state rm` followed by a re-import. A direct `terraform state mv` is not supported because the `keyspace` attribute has changed from a string to a list (see the [Upgrading to v1.0.0 guide](upgrading-to-v1.0.0#recurring-backups)):

```sh
# On the old provider version, before pinning to ~> 1.0:
terraform state rm yba_backups.example

# Then pin to ~> 1.0, write the new yba_backup_schedule HCL, and re-import using the
# existing schedule UUID:
terraform import yba_backup_schedule.example <schedule-uuid>
terraform plan
```
