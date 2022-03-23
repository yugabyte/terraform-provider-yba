terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.1.0"
    }
  }
}

# GKE cluster
resource "google_container_cluster" "primary" {
  name     = var.cluster_name

  remove_default_node_pool = true
  initial_node_count       = 1

  network    = var.network
  subnetwork = var.subnet
}

# Separately Managed Node Pool
resource "google_container_node_pool" "primary_nodes" {
  name       = "${google_container_cluster.primary.name}-node-pool"
  cluster    = google_container_cluster.primary.name
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

resource "kubernetes_namespace" "yb_anywhere" {
  metadata {
    name = var.cluster_name
  }
}

// load in kubernetes secret obtained from Yugabyte
resource "kubernetes_secret" "example" {
  metadata {
    name = "yugabyte-k8s-pull-secret"
    namespace = kubernetes_namespace.yb_anywhere.metadata.name
  }
  data = {
    ".dockerconfigjson" = var.docker_config_json
  }
  type = "kubernetes.io/dockerconfigjson"
}

resource "helm_release" "yb-release" {
  name = var.cluster_name
  repository = "https://charts.yugabyte.com"
  chart = "yugabytedb/yugaware"
  version = var.chart_version
  namespace = kubernetes_namespace.yb_anywhere.metadata.name
  wait = true
}

data "kubernetes_service" "yb_anywhere" {
  metadata {
    name = var.cluster_name
  }
}