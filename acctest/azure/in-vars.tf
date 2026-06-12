# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Values for these variables live in terraform.tfvars.

variable "azure_subscription_id" {
  description = "Azure subscription ID for all created resources."
  type        = string
}

variable "azure_tenant_id" {
  description = "Azure tenant ID (Entra ID directory) the subscription lives in."
  type        = string
}

variable "prefix" {
  description = "Prefix for all created resources, to avoid collisions between parallel runs."
  type        = string

  # Must be a valid Azure resource name and short enough that the longest suffix
  # (e.g. "-yba-data") fits within Azure's name limits. Storage account names are
  # derived separately (see storage.tf) because they disallow hyphens.
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,23}[a-z0-9]$", var.prefix))
    error_message = "prefix must be 2-25 chars, start with a lowercase letter, end with a letter or digit, and contain only lowercase letters, digits, and hyphens."
  }
}

variable "tags" {
  description = "Tags applied to created Azure resources."
  type        = map(string)
}

variable "azure_region" {
  description = "Azure region for the resource group, VNet and the YBA VM."
  type        = string
}

variable "vnet_cidr" {
  description = "CIDR range for the VNet."
  type        = string
}

variable "yba_subnet_cidr" {
  description = "CIDR range for the YBA control-plane subnet."
  type        = string
}

variable "ybdb_subnet_cidrs" {
  description = "CIDR ranges for the three YBDB data-plane subnets, one per zone (westus2-1/2/3)."
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

# AlmaLinux 9 x86_64 gen2 marketplace image. publisher/offer/sku are stable;
# version "latest" tracks the newest published build. No marketplace plan/terms
# are required for this image (it carries no purchase plan).
variable "base_image" {
  description = "AlmaLinux 9 marketplace image reference for the YBA VM."
  type = object({
    publisher = string
    offer     = string
    sku       = string
    version   = string
  })
  default = {
    publisher = "almalinux"
    offer     = "almalinux-x86_64"
    sku       = "9-gen2"
    version   = "latest"
  }
}

variable "yba_version" {
  description = "YugabyteDB Anywhere version (with build number) to install. Must be a valid downloadable build; provider requires >= 2024.2.0.0."
  type        = string
}

variable "yba_vm_size" {
  description = "VM size for the YBA control-plane VM."
  type        = string
  default     = "Standard_D4s_v3"
}

variable "ssh_private_key_file" {
  description = "Path to the SSH private key used to install YBA; its .pub is added to the VM. ~ is expanded. Mirrors gcp/aws: an existing key is required because yba_installer validates the private-key path exists at plan time, so a key generated in the same apply cannot satisfy it. (Setting this to \"\" triggers in-apply key generation, which fails that plan-time check.)"
  type        = string
  default     = "~/.ssh/id_rsa"
}

variable "yba_admin_user" {
  description = "Admin (SSH) user created on the YBA VM. The installer connects as this user."
  type        = string
  default     = "yugabyte"
}

variable "yba_username" {
  description = "Username (email) for the initial YBA superuser (customer). Published as the yba_username output."
  type        = string
  default     = "admin@example.com"
}
