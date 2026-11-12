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

variable "home_region" {
  type    = string
  default = "us-ashburn-1"
}

variable "region_2" {
  type    = string
  default = "us-phoenix-1"
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

variable "ybdb_subnet_cidrs" {
  type    = list(string)
  default = ["10.2.2.0/24", "10.2.3.0/24"]
  validation {
    condition     = length(var.ybdb_subnet_cidrs) == 2
    error_message = "ybdb_subnet_cidrs must list exactly two CIDRs."
  }
}

variable "region2_vcn_cidr" {
  type    = string
  default = "10.3.0.0/16"
}

variable "region2_subnet_cidr" {
  type    = string
  default = "10.3.1.0/24"
}

variable "operator_cidr_ranges" {
  type = list(string)
}

variable "yba_version" {
  type        = string
  description = "YugabyteDB Anywhere build to install. Must be at least 2026.1.2.0."
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
