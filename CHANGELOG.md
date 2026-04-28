## v1.0.0 (May 2026)

The first major release of the YugabyteDB Anywhere Terraform Provider. It introduces typed, simplified replacements for the generic resources that shipped in the v0.1.x line, and removes two unsupported resources. The deprecated v0.1.x resources continue to work throughout v1.x; their planned removal is v2.0.0.

See the [Upgrading to v1.0.0](docs/guides/upgrading-to-v1.0.0.md) guide for migration steps.

### Breaking changes

- The provider now requires YugabyteDB Anywhere stable `2024.2.0.0` or later, or preview `2.23.1.0` or later.
- `yba_releases` resource removed. Manage YBDB releases through the YBA UI or API. The `yba_release_version` data source is unchanged.
- `yba_installation` resource (Replicated-based installer) removed. Use `yba_installer` for new YBA deployments.
- `yba_universe`: `universe_name` is now immutable post-create.
- `yba_universe`: access key rotation post-create is blocked; rotate keys through the cloud provider resource.
- `yba_universe`: full-move-triggering edits (volume_size decrease, num_volumes change, storage_type change) require an explicit `full_move { allow = true }` acknowledgment.
- `yba_universe`: `communication_ports` changes that would trigger a full move require the same acknowledgment.
- `yba_cloud_provider` (deprecated, still available): GCP `application_credentials` (Map of strings) replaced by `credentials` (String). The same field on the new `yba_gcp_provider` is `credentials` (String).
- `yba_cloud_provider` (deprecated, still available): GCP `create_vpc` boolean introduced and is now distinct from `use_host_vpc`. Set `use_host_vpc = false` and supply `network` to use an existing VPC. The same fields are present on `yba_gcp_provider`.
- Image-bundle-based provisioning replaces per-region `ssh_port`, `ssh_user`, and `yb_image` on the new typed providers; configure VM images through `image_bundles` blocks.
- The `dest_vpc_id`, `host_vpc_id`, and `host_vpc_region` fields on `yba_cloud_provider` are not carried over to the new typed providers; cloud-specific network settings live on each typed resource.

### Deprecations (kept through v1.x, planned removal in v2.0.0)

- `yba_cloud_provider` -> use `yba_aws_provider`, `yba_gcp_provider`, or `yba_azure_provider`.
- `yba_storage_config_resource` -> use `yba_s3_storage_config`, `yba_gcs_storage_config`, `yba_azure_storage_config`, or `yba_nfs_storage_config`.
- `yba_backups` -> use `yba_backup_schedule`. HCL fields changed (notably `keyspace` -> `keyspaces`); migrate via `terraform state rm` + re-import on `yba_backup_schedule`.
- Provider env vars `YB_HOST` / `YB_API_KEY` / `YB_ENABLE_HTTPS` are now fallbacks for `YBA_HOST` / `YBA_API_TOKEN` / `YBA_ENABLE_HTTPS`.

### New resources

- `yba_aws_provider`, `yba_gcp_provider`, `yba_azure_provider` - typed cloud provider resources with flat schemas.
- `yba_s3_storage_config`, `yba_gcs_storage_config`, `yba_azure_storage_config`, `yba_nfs_storage_config` - typed storage configuration resources.
- `yba_backup` - on-demand (non-scheduled) backup resource.
- `yba_backup_schedule` - replaces `yba_backups`.

### New data sources

- `yba_provider_image_bundles` - list image bundles for a provider.
- `yba_universe_schema` - inspect namespaces and tables on a universe.

### Enhancements

- `yba_universe`: smart-resize support when YBA reports it as an option (in-place rolling update for eligible edits).
- `yba_universe`: support for rollback and finalize during DB version upgrades; new `db_version_upgrade_options` block.
- `yba_universe`: support for OS upgrade via `image_bundle_uuid`.
- `yba_universe`: dedicated master placement, including device_info import.
- `yba_universe`: per-AZ replication factor placement on multi-AZ clusters.
- `yba_universe`: validate that `client_root_ca` works independently from `root_ca`.
- `yba_universe`: clearer plan-time validation for cloud_list and user_intent fields.
- `yba_universe`: node details (IP, state, AZ placement) are exposed on the resource for downstream consumption.
- `yba_restore`: prevent parallel restores on the same universe.
- `yba_backup_info`: incremental backups are now reported by the data source.
- `yba_cloud_provider`: support per-region `instance_template` for GCP regions.

### Documentation

- New "Upgrading to v1.0.0" guide consolidating breaking changes and migration steps.
- New "Universe edit actions" guide documenting which configuration changes map to which YBA tasks.
- All `Sensitive` schema fields now carry a consistent state-encryption note recommending an encrypted Terraform backend.
- Per-resource migration sections on each typed provider and storage config page.
- Universe import documentation now calls out that `ysql_password` and `ycql_password` are not imported, with the `lifecycle.ignore_changes` workaround.
- Provider index documents the `terraform plan -detailed-exitcode` CI pattern for managing drift from out-of-band UI / API changes.

## v0.1.11 (March 2024)

The following version of YugabyteDB Anywhere Terraform Provider includes support for:

### Enhancements

- Allow GCP shared VPC project and host project to be declared separately in yba_cloud_provider
- Support Azure Network Subscription ID and Azure Network Resource Group in yba_cloud_provider

### Data Sources

- Fetch regions of a provider (yba_provider_regions)

## v0.1.10 (January 2024)

The following version of YugabyteDB Anywhere Terraform Provider includes support for:

### Enhancements

- Allow credentials to be added as fields in yba_cloud_provider and yba_storage_config_resource
- Remove storing of node instances in state file if not provided inline in yba_onprem_provider

## v0.1.9 (October 2023)

The following version of YugabyteDB Anywhere Terraform Provider includes support for:

### Enhancements

- Remove check for GCP credentials if *use_host_credentials* is set
- Rename local development provider location and add path to makefile

### Resources

- Adding nodes to an on premises provider (yba_onprem_node_instance)

## v0.1.8 (September 2023)

The following version of YugabyteDB Anywhere Terraform Provider includes support for:

### Enhancements

- Increasing timeout for REST API calls to prevent Client Timeouts

## v0.1.7 (September 2023)

The following version of YugabyteDB Anywhere Terraform Provider includes support for:

### Enhancements

- Provider deletion task waits for completion.
- Deprecating YugabyteDB Anywhere Installation via Replicated resource (yba_installation)

### Data Sources

- Filters for Nodes in on-premises Providers (yba_onprem_nodes)
- Filters for Providers (yba_provider_filter)
- Filters for Universes (yba_universe_filter)

### Resources

- YugabyteDB Anywhere Installation via YBA Installer (yba_installer)

## v0.1.6 (August 2023)

The following version of YugabyteDB Anywhere Terraform Provider includes support for the following:

### Workflows

- Import On premises provider into terraform configuration

### Enhancements

- Use YugabyteDB Anywhere host IAM credentials to create AWS cloud providers and S3 storage configurations
- Restrict Schdeuled backups for YugabyteDB Anywhere versions == 2.19.0
- Guide for onprem provider and universes
- Provide error messages on task failures on the command line

## v0.1.5 (July 2023)

The following version of YugabyteDB Anywhere Terraform Provider includes support for the following:

### Resources

- On Premises Provider (yba_onprem_provider)

### Data Sources

- Preflight checks for Nodes used in On Premises Providers (yba_onprem_preflight)

### Workflows

- Create and Edit Incremental Backup Schedules

### Enhancements

- Insecure HTTPS connection to YugabyteDB Anywhere
- Detailed requirements for yba_universe resource fields

## v0.1.4 (May 2023)

BACKWARDS INCOMPATIBILITIES / NOTES:

The following version of YugabyteDB Anywhere Terraform Provider supports the following:

### Resources

- Backup Schedules (yba_backups)
- Cloud Providers (yba_cloud_provider), with support for
  - GCP
  - AWS
  - Azure
- Customer (yba_customer_resource)
- YugabyteDB Anywhere Installation via Replicated (yba_installation)
- YBDB Release Import (yba_releases)
- Restores (yba_restore)
- Storage Configuration (yba_storage_config_resource) referring to Backup Target Storage Configuration
- Universe (yba_universe)

### Data Sources

- Backup Information (yba_backup_info)
- Cloud Provider Access Key Information (yba_provider_key)
- Available YBDB Release Versions (yba_release_version)
- Storage Configuration Information (yba_storage_configs)

### Workflows

- YBA Installation
- Create Cloud Provider
- Create and Edit Universe
  - Upgrade software
  - Upgrade GFlags
  - Upgrade to Systemd
  - Toggle TLS settings
  - Edit cluster parameters:
    - Instance type
    - Number of Nodes
    - Number of Volumes per instance
    - Volume Size
    - User Tags
  - Delete Read Replicas (Adding Read Replica after universe creation currently not allowed)
- Create and Edit Backup Storage Configs
  - Edit storage configuration name
  - Edit credentials
- Create and Edit Backup Schedules
  - Edit cron expression
  - Edit frequency of backups
- Restore
