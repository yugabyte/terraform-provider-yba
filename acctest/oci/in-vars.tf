# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "tenancy_ocid" {
  type        = string
  description = "Tenancy OCID. Set in terraform.tfvars before the first apply."
  validation {
    condition     = startswith(var.tenancy_ocid, "ocid1.tenancy.")
    error_message = "tenancy_ocid must be an ocid1.tenancy OCID."
  }
}

variable "compartment_ocid" {
  type        = string
  description = "Compartment OCID for the stand. Set in terraform.tfvars before the first apply."
  validation {
    condition     = startswith(var.compartment_ocid, "ocid1.compartment.")
    error_message = "compartment_ocid must be an ocid1.compartment OCID."
  }
}

variable "identity_domain" {
  type        = string
  description = "Identity domain in compartment_ocid that holds the test user, group and dynamic group."
}

variable "home_region" {
  type        = string
  default     = "us-sanjose-1"
  description = "The tenancy's home region: IAM, the YBA VM, and the single-zone second test region (OCI_REGION_2)."
}

variable "test_region" {
  type        = string
  default     = "us-ashburn-1"
  description = "The two-zone test region (OCI_REGION). Needs at least two availability domains; no VM runs here."
}

variable "prefix" {
  type    = string
  default = "tf-acctest"
}

variable "vcn_cidr" {
  type    = string
  default = "10.2.0.0/16"
}

variable "yba_subnet_cidr" {
  type    = string
  default = "10.2.1.0/24"
}

variable "home_ybdb_subnet_cidr" {
  type    = string
  default = "10.2.2.0/24"
}

variable "test_vcn_cidr" {
  type    = string
  default = "10.3.0.0/16"
}

variable "test_ybdb_subnet_cidrs" {
  type    = list(string)
  default = ["10.3.1.0/24", "10.3.2.0/24"]
  validation {
    condition     = length(var.test_ybdb_subnet_cidrs) == 2
    error_message = "test_ybdb_subnet_cidrs must list exactly two CIDRs."
  }
}

variable "operator_cidr_ranges" {
  type = list(string)
}

variable "yba_version" {
  type        = string
  description = "YugabyteDB Anywhere version (with build number) to install. Must be a valid downloadable build; the OCI resources require >= 2026.1.2.0-b137, or >= 2.31.0.0-b386 on the preview line (the yba_oci_storage_config minimum)."
}

variable "yba_shape" {
  type    = string
  default = "VM.Standard.E4.Flex"
}

variable "yba_ocpus" {
  type    = number
  default = 2
}

variable "yba_memory_in_gbs" {
  type    = number
  default = 16
}

variable "yba_admin_user" {
  type    = string
  default = "opc"
}

variable "yba_username" {
  type    = string
  default = "admin@example.com"
}
