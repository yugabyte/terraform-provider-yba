---
page_title: "yba_backup_info Data Source - YugabyteDB Anywhere"
description: |-
  Retrieve list of backups.
---

# yba_backup_info (Data Source)

Retrieve list of backups.

~> **Note:** The YugabyteDB Anywhere Terraform provider supports fetching backups in YugabyteDB Anywhere version 2.18.1 and later.

## Example Usage

```terraform
data "yba_backup_info" "backup" {
  universe_uuid = "universe-having-backups-uuid"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `date_range_end` (String) End date of range in which to fetch backups, in RFC3339 format.
- `date_range_start` (String) Start date of range in which to fetch backups, in RFC3339 format.
- `universe_name` (String) The name of the universe whose latest backup you want to fetch.
- `universe_uuid` (String) The UUID of the universe whose latest backup you want to fetch.

### Read-Only

- `backup_type` (String) Type of the backup fetched.
- `id` (String) The ID of this resource.
- `storage_config_uuid` (String) UUID of the storage configuration used for backup.
- `storage_location` (String) Storage location of the backup.

## Restricted YugabyteDB Anywhere Versions

- 2.19.0.0
