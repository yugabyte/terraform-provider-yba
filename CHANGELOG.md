## v1.0.0 (June 2024)

The following version of YugabyteDB Anywhere Terraform Provider includes support for:

### Breaking changes

- Introduction of `create_vpc` boolean in GCP cloud provider configuration and difference from `use_host_vpc` for network settings. Set `use_host_vpc` to false (`network` field is required for this setting)  to use existing VPC in GCP providers.
- Switching GCP `application_credentials` input (Map of strings) to `credentials` (String) in yba_cloud_provider

### Notes

- S3 and GCS release import paths have been deprecated in favour of HTTP path in yba_releases and will be removed in the next major release
- Destination VPC ID, Host VPC ID and Host VPC Regions have been deprecated in favour of cloud specific settings in yba_cloud_provider and will be removed in the next major release
- SSH Port, SSH User and YB Image (in Regions) have been deprecated in favour of image bundle support in yba_cloud_provider and will be removed in the next major release

### Enhancements

- Support Instance templates in GCP cloud provider regions in yba_cloud_provider
- Provide universe node details in yba_universe
- Support Image Bundles in yba_cloud_provider


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
