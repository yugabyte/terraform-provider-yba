# YBA creates, reads and deletes S3 telemetry providers only when this global
# runtime config is true. The default is false.
resource "yba_runtime_config" "allow_s3" {
  key   = "yb.telemetry.allow_s3"
  value = "true"
}

# S3 destination to archive audit logs and query logs.
resource "yba_s3_telemetry_provider" "audit_archive" {
  name = "audit-archive"

  bucket           = "yba-audit-logs"
  region           = "us-west-2"
  access_key       = var.aws_access_key
  secret_key       = var.aws_secret_key
  directory_prefix = "yb-logs"
  file_prefix      = "audit-"

  # Optional: assume an IAM role to write to the bucket.
  role_arn = "arn:aws:iam::111111111111:role/yba-s3-archive"

  include_universe_and_node_in_prefix = true

  # Optional tags. YBA adds them as attributes to every exported record.
  tags = {
    env = "prod"
  }

  depends_on = [yba_runtime_config.allow_s3]
}

# S3-compatible store, such as MinIO, with path-style addressing and one
# directory per hour.
resource "yba_s3_telemetry_provider" "minio" {
  name = "minio-archive"

  bucket     = "yba-logs"
  region     = "us-east-1"
  access_key = var.minio_access_key
  secret_key = var.minio_secret_key

  endpoint         = "http://minio.example.com:9000"
  disable_ssl      = true
  force_path_style = true
  partition        = "hour"

  # Object format: OTLP_JSON (the YBA default) or SUMO_IC.
  marshaler = "OTLP_JSON"

  depends_on = [yba_runtime_config.allow_s3]
}
