# S3 archival destination (long-term audit/query log storage).
resource "yba_s3_telemetry_provider" "audit_archive" {
  name = "audit-archive"

  bucket           = "yba-audit-logs"
  region           = "us-west-2"
  access_key       = var.aws_access_key
  secret_key       = var.aws_secret_key
  directory_prefix = "yb-logs"

  include_universe_and_node_in_prefix = true
}

# S3-compatible store (e.g. MinIO) with path-style addressing and an
# hourly directory layout.
resource "yba_s3_telemetry_provider" "minio" {
  name = "minio-archive"

  bucket     = "yba-logs"
  region     = "us-east-1"
  access_key = var.minio_access_key
  secret_key = var.minio_secret_key

  endpoint         = "https://minio.internal:9000"
  force_path_style = true
  partition        = "hour"
}
