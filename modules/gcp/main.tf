data "google_compute_image" "yb_platform_image" {
  family = var.image_family
  project = var.image_project
}

resource "google_compute_firewall" "yugaware-firewall" {
  name = "${var.cluster_name}-firewall"
  network = var.vpc_network
  allow {
    protocol = "tcp"
    ports = ["22", "8800", "80", "7000", "7100", "9000", "9100", "11000", "12000", "9300", "9042", "5433", "6379"]
  }
  target_tags = [var.cluster_name]
}

resource "google_compute_instance" "yb_platform_node" {
  name = var.cluster_name
  machine_type = var.machine_type

  boot_disk {
    initialize_params {
      image = data.google_compute_image.yb_platform_image.self_link
      size = var.disk_size
    }
  }

  tags = [var.cluster_name]
  metadata = {
    sshKeys = "${var.ssh_user}:${file(var.ssh_public_key)}"
  }

  network_interface {
    network = var.vpc_network
    access_config {}
  }

  // replicated config
  provisioner "file" {
    source = var.replicated_filepath
    destination ="/etc/replicated.conf"
    connection {
      host = self.network_interface.0.access_config.0.nat_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // tls certificate
#  provisioner "file" {
#    source = var.tls_cert_filepath
#    destination ="/etc/server.crt"
#    connection {
#      host = self.network_interface.0.access_config.0.nat_ip
#      type = "ssh"
#      user = var.ssh_user
#      private_key = file(var.ssh_private_key)
#    }
#  }

  // tls key
#  provisioner "file" {
#    source = var.tls_key_filepath
#    destination ="/etc/server.key"
#    connection {
#      host = self.network_interface.0.access_config.0.nat_ip
#      type = "ssh"
#      user = var.ssh_user
#      private_key = file(var.ssh_private_key)
#    }
#  }

  // license file
  provisioner "file" {
    source = var.license_filepath
    destination ="/etc/license.rli"
    connection {
      host = self.network_interface.0.access_config.0.nat_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // application settings
  provisioner "file" {
    source = var.application_settings_filepath
    destination ="/etc/settings.conf"
    connection {
      host = self.network_interface.0.access_config.0.nat_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // install replicated
  provisioner "remote-exec" {
    inline = [
      "curl -sSL https://get.replicated.com/docker | sudo bash",
    ]
    connection {
      host = self.network_interface.0.access_config.0.nat_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }
}