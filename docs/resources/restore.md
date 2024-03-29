---
page_title: "yba_restore Resource - YugabyteDB Anywhere"
description: |-
  Restoring backups for universe. This resource does not track the remote state and is only provided as a convenience tool. It is recommended to remove this resource after running terraform apply.
---

# yba_restore (Resource)

Restoring backups for universe. This resource does not track the remote state and is only provided as a convenience tool. It is recommended to remove this resource after running terraform apply.

~> **Note:** The YugabyteDB Anywhere Terraform provider supports restores in YugabyteDB Anywhere version 2.18.1 and later.

## Example Usage

```terraform
resource "yba_restore" "restore" {
  // information of backups can be retrieved from yba_backup_info
  universe_uuid       = "<target-universe-uuid>"
  keyspace            = "<keyspace-name>"
  storage_location    = "<storage-location-of-backup>"
  restore_type        = "<table-type>"
  storage_config_uuid = "<storage-config-uuid-of-backup>"
}
```

The details for configuration are available in the [YugabyteDB Anywhere Restore universe data](https://docs.yugabyte.com/preview/yugabyte-platform/back-up-restore-universes/restore-universe-data/ysql/).

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `restore_type` (String) Type of the restore. Permitted values: YQL_TABLE_TYPE, REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE.
- `storage_config_uuid` (String) UUID of the storage configuration to use. Can be retrieved from the storage config data source.
- `storage_location` (String) Storage Location of the backup to be restored.
- `universe_uuid` (String) The UUID of the target universe of restore.

### Optional

- `keyspace` (String) Target keyspace name.
- `parallelism` (Number) Number of concurrent commands to run on nodes over SSH.
- `sse` (Boolean) Is SSE.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--timeouts"></a>
### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)

## Restricted YugabyteDB Anywhere Versions

- 2.19.0.0
