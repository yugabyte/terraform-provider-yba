# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# The acceptance-test env as KEY='value' lines, ready to source into a shell.
# Holds the base topology (TF_VAR_AWS_*), the S3 backup location, and the YBA
# endpoint (TF_VAR_AWS_YBA_HOST/TF_VAR_AWS_YBA_API_KEY). `make -C acctest env`
# sources every fixture's test_env into the single, gitignored `env` file; CI
# writes that file back from the ACCTEST_ENV secret and sources it the same way.
#
# The yba_aws_provider resource takes the security group / VPC / subnet by *ID*
# (see internal/provider/aws/resource_aws_provider.go), so the SG/VPC/subnet
# vars are IDs. The access key id/secret are what YBA uses to authenticate.

locals {
  test_env = <<-EOT
    TF_VAR_AWS_ACCESS_KEY_ID='${aws_iam_access_key.yba.id}'
    TF_VAR_AWS_SECRET_ACCESS_KEY='${aws_iam_access_key.yba.secret}'
    TF_VAR_AWS_SG_ID='${aws_security_group.main.id}'
    TF_VAR_AWS_VPC_ID='${aws_vpc.main.id}'
    TF_VAR_AWS_ZONE_SUBNET_ID='${aws_subnet.ybdb[0].id}'
    TF_VAR_AWS_ZONE_SUBNET_ID_2='${aws_subnet.ybdb[1].id}'
    TF_VAR_AWS_AMI_ID='${data.aws_ami.almalinux.id}'
    TF_VAR_AWS_AMI_ID_NEW='${data.aws_ami.almalinux.id}'
    TF_VAR_AWS_AMI_ID_OLD='${data.aws_ami.almalinux_prev.id}'
    TF_VAR_S3_BACKUP_LOCATION='s3://${aws_s3_bucket.backups.bucket}'
    TF_VAR_AWS_YBA_HOST='${aws_eip.yba.public_ip}'
    TF_VAR_AWS_YBA_API_KEY='${yba_customer_resource.customer.api_token}'
    AWS_ACCESS_KEY_ID='${aws_iam_access_key.yba.id}'
    AWS_SECRET_ACCESS_KEY='${aws_iam_access_key.yba.secret}'
  EOT
}

# The acceptance-test env, read at run time by `make -C acctest env`.
output "test_env" {
  description = "Acceptance-test env (TF_VAR_AWS_*) as KEY='value' lines."
  value       = local.test_env
  sensitive   = true # contains the AWS secret key and the YBA API key
}

output "yba_url" {
  value = "https://${aws_eip.yba.public_ip}"
}

output "yba_username" {
  description = "Username (email) of the initial YBA superuser."
  value       = var.yba_username
}

output "yba_password" {
  description = "Password of the initial YBA superuser."
  value       = random_password.customer.result
  sensitive   = true
}
