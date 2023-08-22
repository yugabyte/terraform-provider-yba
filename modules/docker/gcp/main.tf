data "google_compute_image" "yb_anywhere_image" {
  family  = var.image_family
  project = var.image_project
}

resource "google_compute_firewall" "yugaware-firewall" {
 name    = "${var.cluster_name}-fw"
 network = var.vpc_network
 allow {
   protocol = "tcp"
   ports    = ["22", "80", "443"]
 }
 source_ranges = ["${var.runner_ip}"]
 target_tags   = ["terraform-acctest-yugaware"]
}

resource "google_compute_instance" "yb_anywhere_node" {
  name         = var.cluster_name
  machine_type = var.machine_type

  boot_disk {
    initialize_params {
      image = data.google_compute_image.yb_anywhere_image.self_link
      size  = var.disk_size
    }
  }

  tags = var.network_tags
  metadata = {
    ssh-keys = "${var.ssh_user}:${file(var.ssh_public_key)}"
  }

  labels = var.tags

  network_interface {
    network    = var.vpc_network
    subnetwork = var.vpc_subnetwork
    access_config {}
  }
}