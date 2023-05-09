data "google_compute_image" "yb_anywhere_image" {
  family  = var.image_family
  project = var.image_project
}

#resource "google_compute_firewall" "yugaware-firewall" {
#  name    = "${var.cluster_name}-firewall"
#  network = var.vpc_network
#  allow {
#    protocol = "tcp"
#    ports    = ["22", "8800", "80", "7000", "7100", "9000", "9100", "11000", "12000", "9300", "9042", "5433", "6379"]
#  }
#  source_ranges = ["0.0.0.0/0"]
#  target_tags   = [var.cluster_name]
#}

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