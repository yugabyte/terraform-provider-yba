// NFS storage configuration.
resource "yba_nfs_storage_config" "nfs" {
  name            = "my-nfs-config"
  backup_location = "/mnt/nfs/yugabyte-backups"
}

// NFS storage configuration with a custom bucket name.
resource "yba_nfs_storage_config" "nfs_custom" {
  name            = "nfs-custom-bucket"
  backup_location = "/mnt/nfs"
  nfs_bucket      = "yugabyte_backup"
}
