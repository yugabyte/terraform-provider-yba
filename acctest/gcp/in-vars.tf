# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Values for these variables live in terraform.tfvars.

variable "gcp_project_id" {
  description = "GCP project ID"
  type        = string
}

variable "prefix" {
  description = "Prefix for all created resources, to avoid collisions between parallel runs."
  type        = string

  # Must be a valid GCP resource name and short enough that the longest suffix
  # ("-yba" on a service-account account_id, max 30 chars) fits.
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,23}[a-z0-9]$", var.prefix))
    error_message = "prefix must be 2-25 chars, start with a lowercase letter, end with a letter or digit, and contain only lowercase letters, digits, and hyphens."
  }
}

variable "labels" {
  description = "Labels applied to created GCP resources."
  type        = map(string)
}

variable "gcp_region" {
  description = "GCP region for the VPC, subnets and the YBA VM"
  type        = string
}

variable "yba_cidr" {
  description = "CIDR range for the YBA control-plane subnet."
  type        = string
}

variable "ybdb_cidr" {
  description = "CIDR range for the YBDB data-plane subnet (universe nodes)."
  type        = string
}

variable "operator_cidr_ranges" {
  description = "CIDR ranges for operator access (SSH, YBA UI, debugging)"
  type        = list(string)
}

# --- YBA VM + install (yba.tf) ---

variable "base_image" {
  description = "Boot image family for the YBA VM."
  type        = string
  default     = "projects/almalinux-cloud/global/images/family/almalinux-9"

  # yba.tf parses this with regex to resolve a concrete image, so it must be a
  # full image-family path, not a bare family or a specific-image URI.
  validation {
    condition     = can(regex("^projects/[^/]+/global/images/family/.+$", var.base_image))
    error_message = "base_image must be projects/<project>/global/images/family/<family>."
  }
}

variable "yba_version" {
  description = "YugabyteDB Anywhere version (with build number) to install. Must be a valid downloadable build; provider requires >= 2024.2.0.0."
  type        = string
}

variable "yba_machine_type" {
  description = "Machine type for the YBA control-plane VM."
  type        = string
  default     = "n2-standard-4"
}

variable "ssh_private_key_file" {
  description = "Path to the SSH private key used to install YBA; its .pub is added to the VM. ~ is expanded."
  type        = string
  default     = "~/.ssh/google_compute_engine"
}

variable "yba_username" {
  description = "Username (email) for the initial YBA superuser (customer). Published as the yba_username output."
  type        = string
  default     = "admin@example.com"
}
