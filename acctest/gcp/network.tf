# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Single VPC with two subnets: yba hosts the YBA control-plane VM, ybdb hosts
# the YBDB universe nodes YBA provisions. One VPC means no peering needed.

resource "google_compute_network" "main" {
  name                    = "${var.prefix}-vpc"
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
}

resource "google_compute_subnetwork" "yba" {
  name          = "${var.prefix}-yba"
  region        = var.gcp_region
  network       = google_compute_network.main.id
  ip_cidr_range = var.yba_cidr
}

resource "google_compute_subnetwork" "ybdb" {
  name          = "${var.prefix}-ybdb"
  region        = var.gcp_region
  network       = google_compute_network.main.id
  ip_cidr_range = var.ybdb_cidr
}

# Firewalls, kept minimal: allow everything inside the VPC, plus operator
# access (SSH + YBA UI/API) from var.operator_cidr_ranges.

# Allow all traffic between nodes inside the VPC (YBA <-> YBDB, YBDB <-> YBDB).
resource "google_compute_firewall" "internal" {
  name    = "${var.prefix}-allow-internal"
  network = google_compute_network.main.id

  allow {
    protocol = "tcp"
  }
  allow {
    protocol = "udp"
  }
  allow {
    protocol = "icmp"
  }

  source_ranges = [
    google_compute_subnetwork.yba.ip_cidr_range,
    google_compute_subnetwork.ybdb.ip_cidr_range,
  ]
}

# Firewall with target tag "yb-db-node" so the provider's WithFirewallTags
# acceptance test (yb_firewall_tags = "yb-db-node") validates against the project.
resource "google_compute_firewall" "yb_db_node" {
  name    = "${var.prefix}-yb-db-node"
  network = google_compute_network.main.id

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  target_tags = ["yb-db-node"]
  source_ranges = [
    google_compute_subnetwork.yba.ip_cidr_range,
    google_compute_subnetwork.ybdb.ip_cidr_range,
  ]
}

# Allow operator access to YBA (SSH for the installer, 443 for the UI/API).
resource "google_compute_firewall" "operator" {
  name    = "${var.prefix}-allow-operator"
  network = google_compute_network.main.id

  allow {
    protocol = "tcp"
    ports = [
      "22",  # SSH
      "443", # YBA UI/API
    ]
  }

  source_ranges = var.operator_cidr_ranges
}
