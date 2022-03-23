terraform {
  required_providers {
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
    google = {
      source  = "hashicorp/google"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.1.0"
    }
  }
}

locals {
  cluster_name = "yb-anywhere"
}

// Provider is configured using environment variables: GOOGLE_REGION, GOOGLE_PROJECT, GOOGLE_CREDENTIALS.
provider "google" {}

// Configure kubernetes provider with Oauth2 access token.
data "google_client_config" "client_config" {
  depends_on = [module.gke-cluster]
}

// Defer reading the cluster data until the GKE cluster exists.
data "google_container_cluster" "container_cluster" {
  name = local.cluster_name
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
  source = "../../../modules/kubernetes/gke"
  num_nodes = 2
  cluster_name = local.cluster_name
  network = "***REMOVED***"
  subnet = "***REMOVED***"
  docker_config_json = yamldecode(file("~/.yugabyte/itest_kubernetes_secret.yml"))["data"][".dockerconfigjson"]
}


