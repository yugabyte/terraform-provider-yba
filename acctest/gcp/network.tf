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

# Firewalls, kept minimal: allow everything inside the VPC, plus IAP TCP
# forwarding. The standing YBA VM has no direct internet ingress — every
# operator/CI connection (SSH + YBA UI/API) rides an IAP tunnel
# (acctest/with-yba-tunnel.sh). Only ephemeral install-test VMs (tagged
# yba-install-target) accept direct traffic, from var.operator_cidr_ranges.

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

# IAP TCP forwarding requires the IAP API on the project.
resource "google_project_service" "iap" {
  project            = var.gcp_project_id
  service            = "iap.googleapis.com"
  disable_on_destroy = false
}

# 35.235.240.0/20 is Google's fixed IAP relay range — tunnel traffic reaches the
# VM from it, never from the client's IP. VPC-wide is safe: IAP still requires
# per-caller IAM (roles/iap.tunnelResourceAccessor) before it forwards anything.
resource "google_compute_firewall" "iap" {
  name    = "${var.prefix}-allow-iap"
  network = google_compute_network.main.id

  allow {
    protocol = "tcp"
    ports = [
      "22",  # SSH (installer)
      "443", # YBA UI/API
    ]
  }

  source_ranges = ["35.235.240.0/20"]
}

# Direct access for ephemeral install-test VMs only (the OS-image-upgrade long
# test SSHes to its throwaway VM from the runner; runner IPs are unstable so an
# IAP tunnel can't be pre-arranged mid-test). The standing YBA VM is untagged
# and stays unreachable. Tag literal also lives in the long test's config
# (resource_yba_installer_os_upgrade_test.go).
resource "google_compute_firewall" "install_target" {
  name    = "${var.prefix}-allow-install-target"
  network = google_compute_network.main.id

  allow {
    protocol = "tcp"
    ports = [
      "22",  # SSH
      "443", # YBA UI/API
    ]
  }

  target_tags   = ["yba-install-target"]
  source_ranges = var.operator_cidr_ranges
}
