terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
    yba = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

variable "RESOURCES_DIR" {
  type        = string
  description = "directory on the platform runner that holds testing resources"
}

variable "AWS_VPC_ID" {
  type        = string
  description = "AWS VPC ID to run acceptance testing"
}

variable "AWS_SUBNET_ID" {
  type        = string
  description = "AWS subnet ID to run acceptance testing"
}

resource "random_uuid" "random" {
}

provider "aws" {
  region = "us-west-2"
}

module "aws_yb_anywhere" {
  source = "../../modules/docker/aws"

  cluster_name        = "tf-acctest-${random_uuid.random.result}"
  ssh_user            = "ubuntu"
  ssh_keypair         = "aws-acctest"
  security_group_name = "tf-acctest-sg-${random_uuid.random.result}"
  vpc_id              = var.AWS_VPC_ID
  subnet_id           = var.AWS_SUBNET_ID
  // files
  ssh_private_key = "${var.RESOURCES_DIR}/aws-acctest.pem"
}

output "host" {
  value = module.aws_yb_anywhere.public_ip
}

provider "yba" {
  host = "${module.aws_yb_anywhere.public_ip}:80"
}

resource "yba_installation" "installation" {
  public_ip                 = module.aws_yb_anywhere.public_ip
  private_ip                = module.aws_yb_anywhere.private_ip
  ssh_user                  = "ubuntu"
  ssh_host_ip               = module.aws_yb_anywhere.public_ip
  ssh_private_key           = file("${var.RESOURCES_DIR}/aws-acctest.pem")
  replicated_config_file    = "${var.RESOURCES_DIR}/replicated.conf"
  replicated_license_file   = "${var.RESOURCES_DIR}/acctest.rli"
  application_settings_file = "${var.RESOURCES_DIR}/application_settings.conf"
}

resource "yba_customer_resource" "customer" {
  depends_on = [yba_installation.installation]
  code       = "admin"
  email      = "tf@yugabyte.com"
  name       = "acctest"
}

output "api_key" {
  value = yba_customer_resource.customer.api_token
}
