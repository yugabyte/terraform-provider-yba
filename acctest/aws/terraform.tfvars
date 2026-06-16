# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base config: VPC, IAM, the YBA VM + install, and a backups bucket.
# Mirrors acctest/gcp and acctest/azure.

prefix      = "tf-acctest"
aws_region  = "us-west-2"
aws_profile = "byoc-dev"

# YBA version to install on the acceptance-test VM. Must be a valid downloadable
# build (provider requires >= 2024.2.0.0). Update to the build you want to test.
yba_version = "2025.2.3.1-b2"

vpc_cidr        = "10.0.0.0/16"
yba_subnet_cidr = "10.0.1.0/24"

# One subnet per us-west-2 zone (us-west-2a/b/c) for YBDB universe nodes.
ybdb_subnet_cidrs = [
  "10.0.2.0/24",
  "10.0.3.0/24",
  "10.0.4.0/24",
]

# Open for now to keep CI simple; tighten to the runner's IP later.
operator_cidr_ranges = ["0.0.0.0/0"]

tags = {
  env             = "dev"
  managed-by      = "terraform"
  yb_dept         = "cloud"
  yb_owner        = "byoc"
  yb_task         = "dev"
  cloud-component = "byoc-aws-acctest"
}
