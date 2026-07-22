# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base config: VPC, IAM, and the YBA VM + install.

prefix         = "tf-acctest"
gcp_project_id = "byoc-dev"
gcp_region     = "us-west1"

# YBA version to install on the acceptance-test VM. Must be a valid downloadable
# build (provider requires >= 2024.2.0.0). Update to the build you want to test.
yba_version = "2.31.0.0-b164"

yba_cidr  = "10.0.1.0/24"
ybdb_cidr = "10.0.2.0/24"

# Reaches only VMs tagged yba-install-target — the throwaway VMs the
# OS-image-upgrade long test creates and SSHes into from GitHub runners, whose
# IPs are unstable (hence world-open). The standing YBA VM is untagged: no
# direct ingress, IAP tunnel only.
operator_cidr_ranges = ["0.0.0.0/0"]

labels = {
  env             = "dev"
  managed-by      = "terraform"
  yb_dept         = "cloud"
  yb_owner        = "byoc"
  yb_task         = "dev"
  cloud-component = "byoc-gcp-acctest"
}
