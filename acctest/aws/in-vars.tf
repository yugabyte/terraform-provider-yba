# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Values for these variables live in terraform.tfvars.

variable "aws_region" {
  description = "AWS region for the VPC, subnets and the YBA VM."
  type        = string
}

variable "aws_profile" {
  description = "AWS shared-config profile the provider authenticates with. Pinned (like gcp's project and azure's subscription) so `make apply-aws`/`destroy-aws` work without an exported AWS_PROFILE, and so the provider ignores any ambient AWS_* env creds (e.g. the limited yba IAM user exported into acctest/env, which lacks iam: permissions)."
  type        = string
  default     = "byoc-dev"
}

variable "prefix" {
  description = "Prefix for all created resources, to avoid collisions between parallel runs."
  type        = string

  # Must be a valid AWS resource name and short enough that the longest suffix
  # (e.g. "-yba-data") fits. S3 bucket names are derived separately (see
  # storage.tf) because they must be globally unique and lowercase.
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,23}[a-z0-9]$", var.prefix))
    error_message = "prefix must be 2-25 chars, start with a lowercase letter, end with a letter or digit, and contain only lowercase letters, digits, and hyphens."
  }
}

variable "tags" {
  description = "Tags applied to created AWS resources (via the provider's default_tags)."
  type        = map(string)
}

variable "vpc_cidr" {
  description = "CIDR range for the VPC."
  type        = string
}

variable "yba_subnet_cidr" {
  description = "CIDR range for the YBA control-plane subnet."
  type        = string
}

variable "ybdb_subnet_cidrs" {
  description = "CIDR ranges for the YBDB data-plane subnets, one per zone (us-west-2a/b/c)."
  type        = list(string)

  validation {
    condition     = length(var.ybdb_subnet_cidrs) == 3
    error_message = "ybdb_subnet_cidrs must list exactly three CIDRs (one per zone)."
  }
}

variable "operator_cidr_ranges" {
  description = "CIDR ranges for operator access (SSH, YBA UI, debugging)."
  type        = list(string)
}

# --- YBA VM + install (yba.tf) ---

# AlmaLinux 9 x86_64 AMI, resolved by yba.tf via a data source (the AMI ID
# differs per region). publisher account and name pattern are stable; "most
# recent" tracks the newest published build. The same AMI is surfaced as
# TF_VAR_AWS_AMI_ID for the provider tests' custom image bundles.
variable "base_image" {
  # name_pattern is the newer AMI (YBA VM boot image, TF_VAR_AWS_AMI_ID_NEW).
  # prev_name_pattern is the older one (TF_VAR_AWS_AMI_ID_OLD). Both pinned per
  # minor so they stay distinct. Bump as minors age out.
  description = "AlmaLinux 9 AMI lookup for the YBA VM and the image-bundle tests."
  type = object({
    owner             = string
    name_pattern      = string
    prev_name_pattern = string
  })
  default = {
    # AlmaLinux OS Foundation's AWS account that publishes the official AMIs.
    owner             = "764336703387"
    name_pattern      = "AlmaLinux OS 9.8*x86_64"
    prev_name_pattern = "AlmaLinux OS 9.7*x86_64"
  }
}

variable "yba_version" {
  description = "YugabyteDB Anywhere version (with build number) to install. Must be a valid downloadable build; provider requires >= 2024.2.0.0."
  type        = string
}

variable "yba_instance_type" {
  description = "EC2 instance type for the YBA control-plane VM."
  type        = string
  default     = "m5.xlarge"
}

variable "ssh_private_key_file" {
  description = "Path to the SSH private key used to install YBA; its .pub is added to the VM. ~ is expanded. Mirrors gcp/azure: an existing key is required because yba_installer validates the private-key path exists at plan time, so a key generated in the same apply cannot satisfy it. (Setting this to \"\" triggers in-apply key generation, which fails that plan-time check.)"
  type        = string
  default     = "~/.ssh/id_rsa"
}

variable "yba_admin_user" {
  description = "Admin (SSH) user on the YBA VM. The installer connects as this user. AlmaLinux AMIs default to ec2-user."
  type        = string
  default     = "ec2-user"
}

variable "yba_username" {
  description = "Username (email) for the initial YBA superuser (customer). Published as the yba_username output."
  type        = string
  default     = "admin@example.com"
}
