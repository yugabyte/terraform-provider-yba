# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Authentication is the operator's ~/.oci/config, which needs rights in
# compartment_ocid and in identity_domain only, not tenancy-wide IAM.

# home_region must be the tenancy's home region (IAM is only writable there).
# test_region needs at least two availability domains.
home_region = "us-sanjose-1"
test_region = "us-ashburn-1"
prefix      = "tf-acctest"

tenancy_ocid     = "ocid1.tenancy.oc1..aaaaaaaa6w2zfjmzecayfzpuwufo5jsiedlap7hce6chuvess7eu2mofvlva"
compartment_ocid = "ocid1.compartment.oc1..aaaaaaaaebaxkyhmnbwgmj2zgovsax6vseien7w62tgohejnwg4ydyiqhybq"
identity_domain  = "itest"

# OCI storage gate needs 2026.1.2.0-b137 / 2.31.0.0-b386.
yba_version = "2.31.0.0-b395"

vcn_cidr              = "10.2.0.0/16"
yba_subnet_cidr       = "10.2.1.0/24"
home_ybdb_subnet_cidr = "10.2.2.0/24"

test_vcn_cidr          = "10.3.0.0/16"
test_ybdb_subnet_cidrs = ["10.3.1.0/24", "10.3.2.0/24"]

operator_cidr_ranges = ["0.0.0.0/0"]
