# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base for AWS acceptance tests: VPC, IAM (an access-key user + an instance
# role/profile), the YBA control-plane VM + install (yba.tf), a backups S3
# bucket, and the `test_env` output holding the test env. Applied once, reused
# for many `make acctest` runs.
#
# The `yba` provider here is our locally-built dev binary, used only to install
# YBA and register the first customer (the bootstrap resources in yba.tf). It is
# wired via dev_overrides, so the `init-aws` / `apply-aws` make targets run
# `make install` first. NOTE: with dev_overrides in ~/.terraformrc, `terraform
# init` does not install yba and prints a harmless "development overrides are in
# effect" warning.

terraform {
  required_version = ">= 1.5"

  # State in GCS (gs://tf-acctest-tfstate/aws), the same bucket the gcp fixture
  # uses, just a different prefix. Reusing GCS (not an S3 backend) keeps the
  # state plumbing identical across all cloud fixtures.
  backend "gcs" {
    bucket = "tf-acctest-tfstate"
    prefix = "aws"
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = ">= 4.0"
    }
    yba = {
      source = "yugabyte/yba"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile

  default_tags {
    tags = var.tags
  }
}

# Bootstrap provider for the install/first-customer flow: it points at the YBA
# VM's address but carries no api_token (the provider runs unauthenticated until
# yba_customer_resource registers the first user and mints the token). The
# installer ignores host (it works over SSH); the customer resource needs it to
# reach the freshly-installed YBA instead of the default localhost:9000.
provider "yba" {
  alias = "bootstrap"
  host  = aws_eip.yba.public_ip
}
