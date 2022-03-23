terraform {
  required_providers {
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.1.0"
    }
  }
}

data "azurerm_kubernetes_cluster" "yb-anywhere" {
  depends_on          = [module.aks-cluster]
  name                = var.cluster_name
  resource_group_name = var.cluster_name
}

provider "kubernetes" {
  host                   = data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.host
  client_certificate     = base64decode(data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.client_certificate)
  client_key             = base64decode(data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.client_key)
  cluster_ca_certificate = base64decode(data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.cluster_ca_certificate)
}

provider "helm" {
  kubernetes {
    host                   = data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.host
    client_certificate     = base64decode(data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.client_certificate)
    client_key             = base64decode(data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.client_key)
    cluster_ca_certificate = base64decode(data.azurerm_kubernetes_cluster.yb-anywhere.kube_config.0.cluster_ca_certificate)
  }
}

provider "azurerm" {
  features {}
}

module "aks-cluster" {
  source       = "./aks-cluster"
  cluster_name = var.cluster_name
  region_name     = var.region_name
  num_nodes = var.num_nodes
}

module "kubernetes-config" {
  depends_on       = [module.aks-cluster]
  source           = "../kubernetes-config"
  cluster_name     = var.cluster_name
  docker_config_json = var.docker_config_json
}

data "kubernetes_service" "yb_anywhere" {
  depends_on = [module.kubernetes-config]
  metadata {
    name = "${var.cluster_name}-yugaware-ui"
    namespace = var.cluster_name
  }
}