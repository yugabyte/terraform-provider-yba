## v0.1.3 (May 2023)

BACKWARDS INCOMPATIBILITIES / NOTES:

The following version of YugabyteDB Anywhere Terraform Provider supports:

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
  - Upgrade softwares
  - Upgrade GFlags
  - Upgrade to SystemD
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
