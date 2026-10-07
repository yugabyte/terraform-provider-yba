# AWS CloudWatch Logs destination for logs.
resource "yba_aws_cloudwatch_telemetry_provider" "cw" {
  name = "cloudwatch"

  log_group  = "yba/audit"
  log_stream = "primary"
  region     = "us-west-2"
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key

  # Optional: assume an IAM role, and send the logs through a VPC endpoint.
  role_arn = "arn:aws:iam::111111111111:role/yba-cloudwatch"
  endpoint = "https://vpce-0123456789abcdef0-abcd1234.logs.us-west-2.vpce.amazonaws.com"

  # Optional tags. YBA adds them as attributes to every exported record.
  tags = {
    env = "prod"
  }
}
