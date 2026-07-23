# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Identity for the AWS acceptance-test fixture, in two forms YBA can consume:
#
#   * an IAM user with an access key — passed to yba_aws_provider
#     (access_key_id/secret_access_key) and the S3 storage-config tests, and
#     surfaced via the test_env output (out-vars.tf);
#   * an IAM role + instance profile attached to the YBA VM — exercised by the
#     use_iam_instance_profile=true provider test, where YBA authenticates with
#     the VM's role instead of a key.
#
# Both carry the same permissions: provision universe EC2 (instances, volumes,
# SGs, key pairs), manage NLBs for the load-balancer attach tests, and
# read/write backups in the fixture's S3 bucket.

# EC2 to provision universe nodes; S3 scoped to the backups bucket for the
# storage-config tests. EC2 actions are not resource-scopable in a way YBA's
# dynamic provisioning tolerates, so EC2 is account-wide here (this is a
# throwaway dev account); S3 is scoped to just the backups bucket.
data "aws_iam_policy_document" "yba" {
  statement {
    sid       = "EC2Provisioning"
    effect    = "Allow"
    actions   = ["ec2:*"]
    resources = ["*"]
  }

  # NLB attach tests (yba_universe_load_balancer_config): the in-test aws_lb
  # plus the target groups and listeners YBA creates on it during attach. The
  # LB and target-group names are dynamic (name_prefix / YBA-derived), so like
  # EC2 this doesn't resource-scope usefully.
  statement {
    sid       = "LoadBalancerAttach"
    effect    = "Allow"
    actions   = ["elasticloadbalancing:*"]
    resources = ["*"]
  }

  statement {
    sid    = "BackupsBucket"
    effect = "Allow"
    actions = [
      "s3:ListBucket",
      "s3:GetBucketLocation",
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
    ]
    resources = [
      aws_s3_bucket.backups.arn,
      "${aws_s3_bucket.backups.arn}/*",
    ]
  }
}

resource "aws_iam_policy" "yba" {
  name   = "${var.prefix}-yba"
  policy = data.aws_iam_policy_document.yba.json
}

# --- Access-key user (credentials-based provider + S3 storage config) ---

resource "aws_iam_user" "yba" {
  name          = "${var.prefix}-yba"
  force_destroy = true
}

resource "aws_iam_user_policy_attachment" "yba" {
  user       = aws_iam_user.yba.name
  policy_arn = aws_iam_policy.yba.arn
}

# The only long-lived credential here; surfaced via test_env (out-vars.tf) and
# destroyed on teardown.
resource "aws_iam_access_key" "yba" {
  user = aws_iam_user.yba.name
}

# --- Instance role/profile (use_iam_instance_profile=true provider test) ---

data "aws_iam_policy_document" "ec2_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "yba" {
  name               = "${var.prefix}-yba"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

resource "aws_iam_role_policy_attachment" "yba" {
  role       = aws_iam_role.yba.name
  policy_arn = aws_iam_policy.yba.arn
}

resource "aws_iam_instance_profile" "yba" {
  name = "${var.prefix}-yba"
  role = aws_iam_role.yba.name
}
