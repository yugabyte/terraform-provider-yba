# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# KMS key for the encryption-at-rest acceptance test (yba_aws_ear_config). The
# test points YBA at this existing key: without cmk_id, YBA creates a key that
# nothing deletes. The default key policy lets IAM policies grant access, and
# the yba policy (iam.tf) does.

resource "aws_kms_key" "ear" {
  description             = "${var.prefix} YBA encryption-at-rest acceptance tests"
  deletion_window_in_days = 7
}
