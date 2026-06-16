# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Backups bucket for the S3 storage-config acceptance tests. Surfaced as
# TF_VAR_S3_BACKUP_LOCATION (s3://<bucket>/) via test_env (out-vars.tf); the IAM
# user/role are granted read/write on it in iam.tf.

# Bucket names are globally unique, 3-63 chars, lowercase. Derive one from the
# prefix plus a random suffix rather than reusing var.prefix verbatim.
resource "random_string" "bucket_suffix" {
  length  = 8
  upper   = false
  special = false
}

resource "aws_s3_bucket" "backups" {
  bucket        = "${var.prefix}-backups-${random_string.bucket_suffix.result}"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "backups" {
  bucket                  = aws_s3_bucket.backups.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
