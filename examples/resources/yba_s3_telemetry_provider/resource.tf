# S3 archival destination (long-term audit/query log storage).
resource "yba_s3_telemetry_provider" "audit_archive" {
  name = "audit-archive"

  bucket           = "yba-audit-logs"
  region           = "us-west-2"
  access_key       = var.aws_access_key
  secret_key       = var.aws_secret_key
  directory_prefix = "yb-logs"
  file_prefix      = "audit-"

  # Optional: assume a role for the bucket writes.
  role_arn = "arn:aws:iam::111111111111:role/yba-s3-archive"

  include_universe_and_node_in_prefix = true

  # Optional tags, upserted as attributes onto every exported record.
  tags = {
    env = "prod"
  }
}

# S3-compatible store (e.g. MinIO) with path-style addressing and an
# hourly directory layout.
resource "yba_s3_telemetry_provider" "minio" {
  name = "minio-archive"

  bucket     = "yba-logs"
  region     = "us-east-1"
  access_key = var.minio_access_key
  secret_key = var.minio_secret_key

  endpoint         = "http://minio.internal:9000"
  disable_ssl      = true
  force_path_style = true
  partition        = "hour"

  # Serialization format: OTLP_JSON (YBA default) or SUMO_IC (logs only).
  marshaler = "OTLP_JSON"
}
