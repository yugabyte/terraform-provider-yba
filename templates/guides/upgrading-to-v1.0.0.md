---
subcategory: ""
page_title: "Upgrading to v1.0.0"
description: |-
  Breaking changes and migration steps for upgrading from v0.1.x to v1.0.0
---

# Upgrading to v1.0.0

v1.0.0 is the first major release of the YugabyteDB Anywhere Terraform provider. It introduces a set of typed, simplified resources to replace the generic resources that shipped in the `v0.1.x` line, and removes two unsupported resources.

The deprecated resources continue to work throughout the `v1.x` line. `v2.0.0` will remove them. Migrating to the new resources during `v1.0.0` keeps you on a path that avoids a future hard cutover.

This guide walks through every breaking change between `v0.1.13` and `v1.0.0` and gives the exact commands to migrate.

## At a glance

| Change | Old | New | Required action |
|---|---|---|---|
| Cloud provider | `yba_cloud_provider` | [`yba_aws_provider`](../resources/aws_provider) / [`yba_gcp_provider`](../resources/gcp_provider) / [`yba_azure_provider`](../resources/azure_provider) | Rewrite HCL, `state rm` + `import` |
| Storage config | `yba_storage_config_resource` | [`yba_s3_storage_config`](../resources/s3_storage_config) / [`yba_gcs_storage_config`](../resources/gcs_storage_config) / [`yba_azure_storage_config`](../resources/azure_storage_config) / [`yba_nfs_storage_config`](../resources/nfs_storage_config) | Rewrite HCL, `state rm` + `import` |
| Recurring backups | `yba_backups` | [`yba_backup_schedule`](../resources/backup_schedule) | Rewrite HCL, `state rm` + `import` |
| YBA release import | `yba_releases` | (removed) | Delete from HCL; manage releases through the YBA UI / API |
| Replicated installation | `yba_installation` | (removed) | Delete from HCL; use [`yba_installer`](../resources/installer) for new installs |
| Provider env vars | `YB_HOST` / `YB_API_KEY` / `YB_ENABLE_HTTPS` | `YBA_HOST` / `YBA_API_TOKEN` / `YBA_ENABLE_HTTPS` | Rename env vars (old names still work as fallback) |

## Recommended ordering

When migrating a non-trivial configuration, do the work in this order so that downstream resources keep their references valid:

1. Pin the provider version (see below) and run `terraform init -upgrade`.
2. Migrate **storage configs** first - they have no dependents.
3. Migrate **cloud providers** next. Universes reference the cloud provider via the `clusters[*].user_intent.provider` field (and data sources reference it via `provider_id`); those references continue to resolve after the `state rm` + `import` because the UUID does not change.
4. Migrate **backup schedules** (`yba_backups` -> `yba_backup_schedule`).
5. Delete any remaining `yba_releases` and `yba_installation` blocks (see below).
6. Run `terraform plan` end-to-end; resolve any remaining drift before applying.

## Pin the provider version

Pin to `~> 1.0` in every root module so `terraform init` does not float to a future major release:

```terraform
terraform {
  required_providers {
    yba = {
      source  = "yugabyte/yba"
      version = "~> 1.0"
    }
  }
}
```

## Provider environment variables

The provider now prefers `YBA_`-prefixed environment variables. The legacy `YB_` names still work and are treated as fallbacks.

| Preferred | Legacy fallback |
|---|---|
| `YBA_HOST` | `YB_HOST` |
| `YBA_API_TOKEN` (or `YBA_API_KEY`) | `YB_API_KEY` |
| `YBA_ENABLE_HTTPS` | `YB_ENABLE_HTTPS` |

If you have an `api_token` resolution chain, the lookup order is `YBA_API_TOKEN` -> `YBA_API_KEY` -> `YB_API_KEY`. Rename your CI secrets at your convenience; nothing breaks until a future release removes the legacy names (no earlier than `v2.0.0`).

## Cloud providers

Three cloud-specific resources with flatter schemas replace the generic `yba_cloud_provider`. The full per-cloud field mapping lives on each new resource's reference page; this section shows the migration recipe.

### AWS

```hcl
# Before (v0.1.x)
resource "yba_cloud_provider" "aws" {
  code = "aws"
  name = "aws-provider"
  aws_config_settings {
    access_key_id     = "<aws-access-key-id>"
    secret_access_key = "<aws-secret-access-key>"
  }
  regions {
    code              = "us-west-2"
    vnet_name         = "<aws-vpc-id>"
    security_group_id = "<aws-sg-id>"
    zones {
      code   = "us-west-2a"
      subnet = "<subnet-id-a>"
    }
  }
}

# After (v1.0.0)
resource "yba_aws_provider" "aws" {
  name              = "aws-provider"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"
  regions {
    code              = "us-west-2"
    vpc_id            = "<aws-vpc-id>"
    security_group_id = "<aws-sg-id>"
    zones {
      code   = "us-west-2a"
      subnet = "<subnet-id-a>"
    }
  }
}
```

Key differences:

- `aws_config_settings.*` is lifted to top-level fields (`access_key_id`, `secret_access_key`, `use_iam_instance_profile`, `hosted_zone_id`).
- `regions.vnet_name` is renamed to `regions.vpc_id`.
- The top-level `code`, `dest_vpc_id`, `host_vpc_id`, and `host_vpc_region` fields are dropped.

### GCP

```hcl
# Before
resource "yba_cloud_provider" "gcp" {
  code = "gcp"
  name = "gcp-provider"
  gcp_config_settings {
    project_id  = "my-gcp-project"
    network     = "my-vpc-network"
    credentials = "<service-account-json>"
  }
  regions {
    code = "us-west1"
    zones {
      subnet = "<gcp-shared-subnet-id>"
    }
  }
}

# After
resource "yba_gcp_provider" "gcp" {
  name        = "gcp-provider"
  project_id  = "my-gcp-project"
  network     = "my-vpc-network"
  credentials = "<service-account-json>"
  regions {
    code          = "us-west1"
    shared_subnet = "<gcp-shared-subnet-id>"
  }
}
```

Key differences:

- `gcp_config_settings.*` is lifted to top-level fields.
- A single `regions.shared_subnet` (applied to all zones in the region) replaces the per-zone `subnet`. GCP zones are now computed (YBA auto-discovers them), so you no longer declare them.

### Azure

```hcl
# Before
resource "yba_cloud_provider" "azure" {
  code = "azu"
  name = "azure-provider"
  azure_config_settings {
    client_id       = "<azure-client-id>"
    client_secret   = "<azure-client-secret>"
    tenant_id       = "<azure-tenant-id>"
    subscription_id = "<azure-subscription-id>"
    resource_group  = "<azure-resource-group>"
  }
  regions {
    code      = "eastus"
    vnet_name = "<vnet-name>"
    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }
}

# After
resource "yba_azure_provider" "azure" {
  name            = "azure-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"
  regions {
    code = "eastus"
    vnet = "<vnet-name>"
    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }
}
```

Key differences:

- `azure_config_settings.*` is lifted to top-level fields.
- `regions.vnet_name` is renamed to `regions.vnet`.

### Re-binding the state

State is not transferable across schemas. After rewriting the HCL, drop the old resource from state and re-import using the same provider UUID:

```sh
PROVIDER_UUID=$(terraform show -json | jq -r '.values.root_module.resources[]
  | select(.address == "yba_cloud_provider.aws") | .values.id')

terraform state rm yba_cloud_provider.aws
terraform import yba_aws_provider.aws "$PROVIDER_UUID"
terraform plan
```

Reconcile any remaining plan diff against your new HCL until `terraform plan` reports no changes.

## Storage configurations

Per-backend resources with flatter schemas replace the generic `yba_storage_config_resource`. The `name` field is repurposed (it was the backend code; it is now the human-readable name), and `config_name` is dropped.

### S3 (illustrative; GCS / Azure / NFS follow the same shape)

```hcl
# Before
resource "yba_storage_config_resource" "s3" {
  name            = "S3"
  config_name     = "my-s3-config"
  backup_location = "s3://my-bucket/yugabyte-backups"
  s3_credentials {
    access_key_id     = "<aws-access-key-id>"
    secret_access_key = "<aws-secret-access-key>"
  }
}

# After
resource "yba_s3_storage_config" "s3" {
  name              = "my-s3-config"
  backup_location   = "s3://my-bucket/yugabyte-backups"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"
}
```

The mapping from the old `name` value to the new resource type:

| Old `name` | New resource |
|---|---|
| `S3` | `yba_s3_storage_config` |
| `GCS` | `yba_gcs_storage_config` |
| `AZ` | `yba_azure_storage_config` |
| `NFS` | `yba_nfs_storage_config` |

The typed resources promote per-backend credential fields to the top level and drop the credential sub-blocks (`s3_credentials`, `gcs_credentials`, `azure_credentials`). NFS configs no longer need a credential block.

Re-bind the state to the new address:

```sh
CONFIG_UUID=<previous yba_storage_config_resource id>
terraform state rm yba_storage_config_resource.s3
terraform import yba_s3_storage_config.s3 "$CONFIG_UUID"
terraform plan
```

## Recurring backups

`yba_backup_schedule` replaces the `yba_backups` resource. The HCL fields have changed (notably `keyspace` -> `keyspaces`), so the migration is a drop-and-re-import rather than a `terraform state mv`.

```hcl
# Before (v0.1.x)
resource "yba_backups" "daily" {
  universe_uuid       = yba_universe.example.universe_uuid
  storage_config_uuid = yba_s3_storage_config.s3.id
  keyspace            = "my_keyspace"
  # ...
}

# After (v1.0.0)
resource "yba_backup_schedule" "daily" {
  universe_uuid       = yba_universe.example.universe_uuid
  storage_config_uuid = yba_s3_storage_config.s3.id
  keyspaces           = ["my_keyspace"]
  # ...
}
```

- `keyspaces` (list of strings) replaces the singular `keyspace` (string). Pass `[]` for a full universe backup.
- `transactional_backup` is deprecated; use `table_by_table_backup` instead.

Before upgrading the provider, drop the old resource from state on v0.1.x:

```sh
terraform state rm yba_backups.daily
```

Then upgrade the provider, write the new `yba_backup_schedule` HCL, and re-import using the existing schedule UUID:

```sh
terraform import yba_backup_schedule.daily <schedule-uuid>
terraform plan
```

~> **Note:** A direct `terraform state mv yba_backups.daily yba_backup_schedule.daily` does not work cleanly across the schema change; use the `state rm` + `import` recipe above.

## On-prem provider

~> **Known issues:** `yba_onprem_provider` is undergoing schema changes in v1.0.0. If you currently manage on-premises infrastructure with this resource, we recommend pinning the provider to `version = "~> 0.1"` and remaining on the v0.1.x line until the on-prem support in the v1.x line stabilizes. A follow-up release will document the migration path.

## Removed resources

### `yba_releases`

The `yba_releases` resource has been removed. YBDB release management is no longer exposed through Terraform; manage releases through the YBA UI or API directly. The `yba_release_version` data source (used to look up available release versions for universes) is unchanged.

Remove every `yba_releases` block from your HCL and drop the state entry:

```sh
terraform state rm yba_releases.<name>
```

### `yba_installation`

The legacy Replicated-based `yba_installation` resource has been removed. For new YBA installations, use [`yba_installer`](../resources/installer), which deploys YBA using the supported `yba-installer` tool.

Remove every `yba_installation` block from your HCL and drop the state entry. There is no in-place migration to `yba_installer`; existing Replicated-based YBA installs are not managed by this provider after v1.0.0.

## Universe field renames and behavior changes

The `yba_universe` schema is unchanged in field names between `v0.1.13` and `v1.0.0`, but several edit behaviors and validations are stricter. See the [universe edit actions](universe-edit-actions) guide for the complete list. The notable items:

- A number of `user_intent` fields are now immutable post-create. `universe_name` and `access_key_code` are the most common; the full list (provider, YSQL/YCQL/YEDIS toggles and passwords, IP-assignment and host-name settings, AWS ARN, TLS root CAs, and restricted communication ports) is in the [universe edit actions guide](universe-edit-actions#fields-that-cannot-be-changed-via-terraform-after-creation). Editing any of these on an existing universe is rejected at plan time.
- A `full_move` block (`allow` / `force`) is required to acknowledge full-move edits (volume_size decrease, num_volumes change, storage_type change). Plan-time validation rejects these edits when `allow = false`.
- `communication_ports` changes that would trigger a full move now require an explicit acknowledgment.
- `client_root_ca` now works correctly when set independently from `root_ca`.

If your `v0.1.x` configuration relied on any of these being silently permitted, you will see plan-time errors after upgrading. Update the HCL to acknowledge the operation (typically by adding a `full_move { allow = true }` block) or split the change into a separate apply.

## Verifying the upgrade

After completing the migration, run a full plan against a non-production environment first:

```sh
terraform init -upgrade
terraform plan -detailed-exitcode
```

`plan -detailed-exitcode` exits with `0` for no changes, `1` for an error, and `2` for any pending changes. Treat exit code `2` as a signal to review the diff before applying. See [Managing drift](../index#managing-drift-from-out-of-band-changes) for the recommended CI pattern.
