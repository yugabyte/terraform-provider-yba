# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0

data "oci_identity_availability_domains" "home" {
  provider       = oci.home
  compartment_id = var.tenancy_ocid
}

data "oci_identity_availability_domains" "test" {
  provider       = oci.test
  compartment_id = var.tenancy_ocid
}

resource "oci_core_vcn" "main" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  display_name   = "${var.prefix}-vcn"
  cidr_blocks    = [var.vcn_cidr]
  dns_label      = "tfacctest"
}

resource "oci_core_internet_gateway" "main" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.main.id
  display_name   = "${var.prefix}-igw"
  enabled        = true
}

resource "oci_core_route_table" "main" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.main.id
  display_name   = "${var.prefix}-public"
  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.main.id
  }
}

resource "oci_core_security_list" "main" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.main.id
  display_name   = "${var.prefix}-sl"

  egress_security_rules {
    protocol    = "all"
    destination = "0.0.0.0/0"
  }

  ingress_security_rules {
    protocol = "all"
    source   = var.vcn_cidr
  }

  dynamic "ingress_security_rules" {
    for_each = var.operator_cidr_ranges
    content {
      protocol = "6"
      source   = ingress_security_rules.value
      tcp_options {
        min = 22
        max = 22
      }
    }
  }

  dynamic "ingress_security_rules" {
    for_each = var.operator_cidr_ranges
    content {
      protocol = "6"
      source   = ingress_security_rules.value
      tcp_options {
        min = 443
        max = 443
      }
    }
  }

  dynamic "ingress_security_rules" {
    for_each = var.operator_cidr_ranges
    content {
      protocol = "6"
      source   = ingress_security_rules.value
      tcp_options {
        min = 9000
        max = 9000
      }
    }
  }

  dynamic "ingress_security_rules" {
    for_each = var.operator_cidr_ranges
    content {
      protocol = "6"
      source   = ingress_security_rules.value
      tcp_options {
        min = 9090
        max = 9090
      }
    }
  }
}

resource "oci_core_subnet" "yba" {
  provider                   = oci.home
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.main.id
  display_name               = "${var.prefix}-yba"
  cidr_block                 = var.yba_subnet_cidr
  dns_label                  = "yba"
  route_table_id             = oci_core_route_table.main.id
  security_list_ids          = [oci_core_security_list.main.id]
  prohibit_public_ip_on_vnic = false
}

# The single zone of the second test region (OCI_REGION_2).
resource "oci_core_subnet" "home_ybdb" {
  provider                   = oci.home
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.main.id
  display_name               = "${var.prefix}-ybdb-home"
  cidr_block                 = var.home_ybdb_subnet_cidr
  dns_label                  = "ybdb"
  availability_domain        = data.oci_identity_availability_domains.home.availability_domains[0].name
  route_table_id             = oci_core_route_table.main.id
  security_list_ids          = [oci_core_security_list.main.id]
  prohibit_public_ip_on_vnic = false
}

# The two-zone test region (OCI_REGION): networking and the instance
# configuration only, no VM.
resource "oci_core_vcn" "test" {
  provider       = oci.test
  compartment_id = var.compartment_ocid
  display_name   = "${var.prefix}-vcn-test"
  cidr_blocks    = [var.test_vcn_cidr]
  dns_label      = "tfacctest2"
}

resource "oci_core_internet_gateway" "test" {
  provider       = oci.test
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.test.id
  display_name   = "${var.prefix}-igw-test"
  enabled        = true
}

resource "oci_core_route_table" "test" {
  provider       = oci.test
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.test.id
  display_name   = "${var.prefix}-public-test"
  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.test.id
  }
}

resource "oci_core_security_list" "test" {
  provider       = oci.test
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.test.id
  display_name   = "${var.prefix}-sl-test"

  egress_security_rules {
    protocol    = "all"
    destination = "0.0.0.0/0"
  }

  ingress_security_rules {
    protocol = "all"
    source   = var.test_vcn_cidr
  }
}

resource "oci_core_subnet" "ybdb" {
  provider                   = oci.test
  count                      = 2
  compartment_id             = var.compartment_ocid
  vcn_id                     = oci_core_vcn.test.id
  display_name               = "${var.prefix}-ybdb-${count.index + 1}"
  cidr_block                 = var.test_ybdb_subnet_cidrs[count.index]
  dns_label                  = "ybdb${count.index + 1}"
  availability_domain        = data.oci_identity_availability_domains.test.availability_domains[count.index].name
  route_table_id             = oci_core_route_table.test.id
  security_list_ids          = [oci_core_security_list.test.id]
  prohibit_public_ip_on_vnic = false

  lifecycle {
    precondition {
      condition     = length(data.oci_identity_availability_domains.test.availability_domains) >= 2
      error_message = "test_region ${var.test_region} has one availability domain; the update test needs two zones. Pick a multi-AD region."
    }
  }
}
