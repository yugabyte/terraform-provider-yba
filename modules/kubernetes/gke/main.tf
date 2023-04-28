terraform {
  required_providers {
    // Google provider is configured using environment variables: GOOGLE_REGION, GOOGLE_PROJECT, GOOGLE_CREDENTIALS
    google = {
      source = "hashicorp/google"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.1.0"
    }
  }
}

// Configure kubernetes provider with Oauth2 access token.
data "google_client_config" "client_config" {
  depends_on = [module.gke-cluster]
}

// Defer reading the cluster data until the GKE cluster exists.
data "google_container_cluster" "container_cluster" {
  name       = var.cluster_name
  depends_on = [module.gke-cluster]
}

provider "kubernetes" {
  host  = "https://${data.google_container_cluster.container_cluster.endpoint}"
  token = data.google_client_config.client_config.access_token
  cluster_ca_certificate = base64decode(
    data.google_container_cluster.container_cluster.master_auth[0].cluster_ca_certificate,
  )
}

provider "helm" {
  kubernetes {
    host  = "https://${data.google_container_cluster.container_cluster.endpoint}"
    token = data.google_client_config.client_config.access_token
    cluster_ca_certificate = base64decode(
      data.google_container_cluster.container_cluster.master_auth[0].cluster_ca_certificate,
    )
  }
}

module "gke-cluster" {
  source       = "./gke-cluster"
  cluster_name = var.cluster_name
  network      = var.network
  subnet       = var.subnet
  num_nodes    = var.num_nodes
  memory_max   = var.memory_max
  cpu_max      = var.cpu_max
}

module "kubernetes-config" {
  depends_on         = [module.gke-cluster]
  source             = "../kubernetes-config"
  cluster_name       = var.cluster_name
  docker_config_json = var.docker_config_json
}