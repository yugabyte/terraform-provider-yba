# AWS CloudWatch Logs destination for audit/query logs.
resource "yba_aws_cloudwatch_telemetry_provider" "cw" {
  name = "cloudwatch"

  log_group  = "yba/audit"
  log_stream = "primary"
  region     = "us-west-2"
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key

  # Optional: assume a role and use a VPC endpoint.
  role_arn = "arn:aws:iam::111111111111:role/yba-cloudwatch"
  endpoint = "https://logs.us-west-2.amazonaws.com"

  # Optional tags, upserted as attributes onto every exported record.
  tags = {
    env = "prod"
  }
}
