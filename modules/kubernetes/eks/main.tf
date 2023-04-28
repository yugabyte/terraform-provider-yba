terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
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

data "aws_eks_cluster" "yb-anywhere" {
  depends_on = [module.eks-cluster]
  name       = var.cluster_name
}

data "aws_eks_cluster_auth" "yb-anywhere" {
  depends_on = [module.eks-cluster]
  name       = var.cluster_name
}

provider "kubernetes" {
  host                   = data.aws_eks_cluster.yb-anywhere.endpoint
  cluster_ca_certificate = base64decode(data.aws_eks_cluster.yb-anywhere.certificate_authority[0].data)
  token                  = data.aws_eks_cluster_auth.yb-anywhere.token
}

provider "helm" {
  kubernetes {
    host                   = data.aws_eks_cluster.yb-anywhere.endpoint
    cluster_ca_certificate = base64decode(data.aws_eks_cluster.yb-anywhere.certificate_authority[0].data)
    token                  = data.aws_eks_cluster_auth.yb-anywhere.token
  }
}

module "eks-cluster" {
  source       = "./eks-cluster"
  cluster_name = var.cluster_name
  vpc_id       = var.vpc_id
  node_count   = var.node_count
  iam_role     = var.iam_role
  subnet_ids   = var.subnet_ids
}

module "kubernetes-config" {
  depends_on         = [module.eks-cluster]
  source             = "../kubernetes-config"
  cluster_name       = var.cluster_name
  docker_config_json = var.docker_config_json
}