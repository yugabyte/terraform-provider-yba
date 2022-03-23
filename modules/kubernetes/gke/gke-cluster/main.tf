terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
    }
  }
}

# GKE cluster
resource "google_container_cluster" "container_cluster" {
  name     = var.cluster_name

  remove_default_node_pool = true
  initial_node_count       = 1

  network    = var.network
  subnetwork = var.subnet

  cluster_autoscaling {
    enabled = true
    resource_limits {
      resource_type = "cpu"
      minimum = 4
      maximum = var.cpu_max
    }

    resource_limits {
      resource_type = "memory"
      minimum = 15
      maximum = var.memory_max
    }
  }
}

# Separately Managed Node Pool
resource "google_container_node_pool" "nodes" {
  name       = "${google_container_cluster.container_cluster.name}-node-pool"
  cluster    = google_container_cluster.container_cluster.name
  node_count = var.num_nodes

  node_config {
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]

    labels = {
      env = var.cluster_name
    }

    machine_type = "n1-standard-1"
    tags         = ["gke-node", "${var.cluster_name}-gke"]
    metadata = {
      disable-legacy-endpoints = "true"
    }
  }
}