# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# tenancy_ocid and compartment_ocid have no default. Add them here before the
# first apply. Authentication is the operator's ~/.oci/config.

home_region = "us-ashburn-1"
region_2    = "us-phoenix-1"
prefix      = "tf-acctest"

yba_version = "2026.1.2.0-b29"

vcn_cidr          = "10.2.0.0/16"
yba_subnet_cidr   = "10.2.1.0/24"
ybdb_subnet_cidrs = ["10.2.2.0/24", "10.2.3.0/24"]

region2_vcn_cidr    = "10.3.0.0/16"
region2_subnet_cidr = "10.3.1.0/24"

operator_cidr_ranges = ["0.0.0.0/0"]
