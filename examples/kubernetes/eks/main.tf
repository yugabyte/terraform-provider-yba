terraform {
  required_providers {
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = ">= 2.0.3"
    }
    aws = {
      source  = "hashicorp/aws"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.1.0"
    }
  }
}

provider "aws" {
  region = "us-west-2"
}

module "eks_cluster" {
  source = "../../../modules/kubernetes/eks"
  cluster_name = "sdu-yb-anywhere"
  vpc_id = "vpc-7376f615"
  docker_config_json = base64decode(yamldecode(file("~/.yugabyte/yugabyte-k8s-secret.yml"))["data"][".dockerconfigjson"])
  iam_role = "eks-admin"
  node_count = 2
  subnet_ids = ["subnet-eb77a48d", "subnet-05b0315804f92030c", "subnet-11798e59", "subnet-1598fa4e"]
}

output "public_ip" {
  value = module.eks_cluster.public_ip
}

