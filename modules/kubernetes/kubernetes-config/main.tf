terraform {
  required_providers {
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

resource "kubernetes_namespace" "yb_anywhere" {
  metadata {
    name = var.cluster_name
  }
}

// load in kubernetes secret obtained from Yugabyte
resource "kubernetes_secret" "secret" {
  metadata {
    name = "yugabyte-k8s-pull-secret"
    namespace = kubernetes_namespace.yb_anywhere.metadata[0].name
  }
  data = {
    ".dockerconfigjson" = var.docker_config_json
  }
  type = "kubernetes.io/dockerconfigjson"
}

resource "helm_release" "yb-release" {
  name = var.cluster_name
  repository = "https://charts.yugabyte.com"
  chart = "yugaware"
  version = var.chart_version
  namespace = kubernetes_namespace.yb_anywhere.metadata[0].name
  wait = true
}

data "kubernetes_service" "yb_anywhere" {
  metadata {
    name = var.cluster_name
  }
}