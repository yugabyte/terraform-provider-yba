// OCI Object Storage with a Customer Secret Key (S3-compatible).
resource "yba_oci_storage_config" "s3_compatible" {
  name            = "oci-backups"
  backup_location = "s3://backup-sjc/yugabyte"
  region          = "us-sanjose-1"
  namespace       = "mytenancy"

  access_key_id     = "<oci-access-key-id>"
  secret_access_key = "<oci-secret-access-key>"
  host_base         = "mytenancy.compat.objectstorage.us-sanjose-1.oraclecloud.com"

  region_locations {
    region    = "us-sanjose-1"
    location  = "s3://backup-sjc/yugabyte"
    host_base = "mytenancy.compat.objectstorage.us-sanjose-1.oraclecloud.com"
  }

  region_locations {
    region    = "us-ashburn-1"
    location  = "s3://backup-iad/yugabyte"
    host_base = "mytenancy.compat.objectstorage.us-ashburn-1.oraclecloud.com"
  }
}

// OCI Object Storage with instance principal. The host and the universe nodes
// must belong to a dynamic group that can manage objects in the bucket.
resource "yba_oci_storage_config" "instance_principal" {
  name            = "oci-backups-iam"
  backup_location = "https://objectstorage.us-sanjose-1.oraclecloud.com/n/mytenancy/b/backup-sjc"
  region          = "us-sanjose-1"
  namespace       = "mytenancy"
  use_oci_iam     = true
}
