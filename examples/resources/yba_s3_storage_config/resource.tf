// S3 storage configuration with access keys.
resource "yba_s3_storage_config" "s3" {
  name              = "my-s3-config"
  backup_location   = "s3://my-bucket/yugabyte-backups"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"
}

// S3 storage configuration using IAM instance profile.
resource "yba_s3_storage_config" "s3_iam" {
  name                     = "s3-iam-config"
  backup_location          = "s3://my-bucket/yugabyte-backups"
  use_iam_instance_profile = true
}

// S3-compatible storage (e.g. MinIO).
resource "yba_s3_storage_config" "minio" {
  name              = "minio-config"
  backup_location   = "s3://my-bucket/yugabyte-backups"
  access_key_id     = "<minio-access-key>"
  secret_access_key = "<minio-secret-key>"
  aws_host_base     = "minio.example.com:9000"
  path_style_access = true
}
