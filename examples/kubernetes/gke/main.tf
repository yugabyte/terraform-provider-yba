terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
    google = {
      source = "hashicorp/google"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.1.0"
    }
  }
}

// Provider is configured using environment variables: GOOGLE_REGION, GOOGLE_PROJECT, GOOGLE_CREDENTIALS.
provider "google" {}

module "gke_cluster" {
  source             = "../../../modules/kubernetes/gke"
  num_nodes          = 2
  cluster_name       = "sdu-yb-anywhere"
  network            = "***REMOVED***"
  subnet             = "***REMOVED***"
  docker_config_json = base64decode(yamldecode(file("~/.yugabyte/yugabyte-k8s-secret.yml"))["data"][".dockerconfigjson"])
  cpu_max            = 10
  memory_max         = 100
}

output "public_ip" {
  value = module.gke_cluster.public_ip
}


