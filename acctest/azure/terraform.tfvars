# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base config: resource group, VNet, service principal, the YBA VM + install,
# and a backups storage account. Mirrors acctest/gcp.

prefix                = "tf-acctest"
azure_subscription_id = "f4172720-99cc-408f-badd-cb6733e9e68d"
azure_tenant_id       = "810c029b-d266-4f13-a23a-54b66cfb5f83"
azure_region          = "westus2"

# YBA version to install on the acceptance-test VM. Must be a valid downloadable
# build (provider requires >= 2024.2.0.0). Update to the build you want to test.
yba_version = "2.31.0.0-b164"

vnet_cidr       = "10.0.0.0/16"
yba_subnet_cidr = "10.0.1.0/24"

# One subnet per westus2 zone (westus2-1/2/3) for YBDB universe nodes.
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
  cloud-component = "byoc-azure-acctest"
}
