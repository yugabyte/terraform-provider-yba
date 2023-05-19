## v0.1.0 (May 2023)

BACKWARDS INCOMPATIBILITIES / NOTES:

The following version of YugabyteDB Anywhere Terraform Provider supports the following:

### Resources

- Backup Schedules (yb_backups)
- Cloud Providers (yb_cloud_provider), with support for
  - GCP
  - AWS
  - Azure
- Customer (yb_customer_resource)
- YugabyteDB Anywhere Installation via Replicated (yb_installation)
- YBDB Release Import (yb_releases)
- Restores (yb_restore)
- Storage Configuration (yb_storage_config_resource) referring to Backup Target Storage Configuration
- Universe (yb_universe)

### Data Sources

- Backup Information (yb_backup_info)
- Cloud Provider Access Key Information (yb_provider_key)
- Available YBDB Release Versions (yb_release_version)
- Storage Configuration Information (yb_storage_configs)

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
