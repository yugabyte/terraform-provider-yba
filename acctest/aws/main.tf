terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

variable "RESOURCES_DIR" {
  type        = string
  description = "directory on the platform runner that holds testing resources"
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
  vpc_id              = "***REMOVED***"
  subnet_id           = "***REMOVED***"
  // files
  ssh_private_key = "${var.RESOURCES_DIR}/aws-acctest.pem"
}

output "host" {
  value = module.aws_yb_anywhere.public_ip
}

provider "yb" {
  host = "${module.aws_yb_anywhere.public_ip}:80"
}

resource "yb_installation" "installation" {
  public_ip                 = module.aws_yb_anywhere.public_ip
  private_ip                = module.aws_yb_anywhere.private_ip
  ssh_user                  = "ubuntu"
  ssh_host_ip               = module.aws_yb_anywhere.public_ip
  ssh_private_key           = file("${var.RESOURCES_DIR}/aws-acctest.pem")
  replicated_config_file    = "${var.RESOURCES_DIR}/replicated.conf"
  replicated_license_file   = "${var.RESOURCES_DIR}/acctest.rli"
  application_settings_file = "${var.RESOURCES_DIR}/application_settings.conf"
}

resource "yb_customer_resource" "customer" {
  depends_on = [yb_installation.installation]
  code       = "admin"
  email      = "tf@yugabyte.com"
  name       = "acctest"
}

output "api_key" {
  value = yb_customer_resource.customer.api_token
}