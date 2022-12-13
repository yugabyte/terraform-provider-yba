terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
    }
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "azurerm" {
  features {}
}

locals {
  home = "/home/deeptikumar"
  dir = "${local.home}/code/terraform-provider-yugabytedb-anywhere/modules/resources"
}

module "azure_yb_anywhere" {
  source = "../../../modules/docker/azure"

  cluster_name   = "test-yugaware-azure"
  ssh_user       = "centos"
  region_name    = "westus2"
  subnet_name    = "yugabyte-subnet-westus2"
  vnet_name      = "yugabyte-vnet-us-west2"
  resource_group = "yugabyte-rg"
  // files
  ssh_private_key = "${local.home}/.ssh/yugaware-azure"
  ssh_public_key  = "${local.home}/.ssh/yugaware-azure.pub"
  security_group = "sg-139dde6c"
}

provider "yb" {
  alias = "unauthenticated"
  // these can be set as environment variables
  host = "${module.azure_yb_anywhere.public_ip}:80"
}

module "installation" {
  source = "../../../modules/installation"

  public_ip = module.azure_yb_anywhere.public_ip
  private_ip = module.azure_yb_anywhere.private_ip
  ssh_user = "centos"
  ssh_private_key_file = "${local.home}/.ssh/yugaware-azure"
  replicated_directory = local.dir
  replicated_license_file_path = "${local.home}/.yugabyte/yugabyte-dev.rli"
}

resource "yb_customer_resource" "customer" {
  provider   = yb.unauthenticated
  depends_on = [module.azure_yb_anywhere, yb_installation.installation]
  code       = "admin"
  email      = "demo@yugabyte.com"
  name       = "demo"
  password   = "Password1@"
}

provider "yb" {
  host      = "${module.azure_yb_anywhere.public_ip}:80"
  api_token = yb_customer_resource.customer.api_token
}

resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  config = merge(
    { YB_FIREWALL_TAGS = "cluster-server" },
    jsondecode(file("${local.home}/.yugabyte/yugabyte-gce.json"))
  )
  dest_vpc_id = "yugabyte-network"
  name        = "test-gcp-in-azure-provider"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 22
  air_gap_install = false
}

data "yb_provider_key" "gcp-key" {
  provider_id = yb_cloud_provider.gcp.id
}

locals {
  region_list  = yb_cloud_provider.gcp.regions[*].uuid
  provider_id  = yb_cloud_provider.gcp.id
  provider_key = data.yb_provider_key.gcp-key.id
}

resource "yb_universe" "gcp_universe" {
  depends_on = [yb_cloud_provider.gcp]
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "test-gcp-universe-on-azure"
      provider_type      = "gcp"
      provider           = local.provider_id
      region_list        = local.region_list
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "n1-standard-1"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "Persistent"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = "2.17.1.0-b238"
      access_key_code               = local.provider_key
    }
  }
  communication_ports {}
}