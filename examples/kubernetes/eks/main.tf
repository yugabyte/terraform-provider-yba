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

provider "aws" {}

module "aws_cluster" {
  source = "../../../modules/kubernetes/eks"
  cluster_name = "sdu-yb-anywhere"
  vpc_id = "vpc-0fe36f6b"
  docker_config_json = base64decode(yamldecode(file("~/.yugabyte/yugabyte-k8s-secret.yml"))["data"][".dockerconfigjson"])
}


