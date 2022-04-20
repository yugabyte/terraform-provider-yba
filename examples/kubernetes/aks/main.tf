terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
    azurerm = {
      source = "hashicorp/azurerm"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.1.0"
    }
  }
}

provider "azurerm" {
  features {}
}

module "aks_cluster" {
  source             = "../../../modules/kubernetes/aks"
  cluster_name       = "sdu-yb-anywhere"
  region_name        = "westus2"
  num_nodes          = 2
  docker_config_json = base64decode(yamldecode(file("~/.yugabyte/yugabyte-k8s-secret.yml"))["data"][".dockerconfigjson"])
}

output "public_ip" {
  value = module.aks_cluster.public_ip
}


