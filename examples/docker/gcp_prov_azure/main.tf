terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

locals {
  home         = "/home/deeptikumar"
  dir          = "${local.home}/code/terraform-provider-yugabytedb-anywhere/modules/resources"
  cluster_name = "azure-in-gcp"
}

provider "google" {
  credentials = "${local.home}/.yugabyte/yugabyte-gce.json"
  project     = "yugabyte"
  region      = "us-west1"
  zone        = "us-west1-b"
}

module "gcp_yb_anywhere" {
  source = "../../../modules/docker/gcp"

  cluster_name   = local.cluster_name
  ssh_user       = "centos"
  network_tags   = [local.cluster_name, "http-server", "https-server"]
  vpc_network    = "yugabyte-network"
  vpc_subnetwork = "subnet-us-west1"
  // files
  ssh_private_key = "${local.home}/.ssh/yugaware-1-gcp"
  ssh_public_key  = "${local.home}/.ssh/yugaware-1-gcp.pub"
}

provider "yb" {
  alias = "unauthenticated"
  // these can be set as environment variables
  host = "${module.gcp_yb_anywhere.public_ip}:80"
}

module "installation" {
  source = "../../../modules/installation"

  public_ip = module.gcp_yb_anywhere.public_ip
  private_ip = module.gcp_yb_anywhere.private_ip
  ssh_user = "centos"
  ssh_private_key_file = "${local.home}/.ssh/yugaware-1-gcp"
  replicated_directory = local.dir
  replicated_license_file_path = "${local.home}/.yugabyte/yugabyte-dev.rli"
}

provider "yb" {
  host      = "${module.gcp_yb_anywhere.public_ip}:80"
  api_token = yb_customer_resource.customer.api_token
}

resource "yb_customer_resource" "customer" {
  provider   = yb.unauthenticated
  depends_on = [module.installation]
  code       = "admin"
  email      = "demo@yugabyte.com"
  name       = "demo"
  password   = "Password1@"
}


resource "yb_cloud_provider" "azure" {
  code = "azu"
  config = {
    "AZURE_SUBSCRIPTION_ID" = "a0fdddea-9fb1-473b-b069-fd3e0b1125db"
    "AZURE_RG" = "yugabyte-rg"
    "AZURE_TENANT_ID" = "810c029b-d266-4f13-a23a-54b66cfb5f83"
    "AZURE_CLIENT_SECRET" = "9un6.0~ix8iFKs4JpY5iP5_1p~RRyihJ_l"
    "AZURE_CLIENT_ID" = "d43d538c-d84d-42ab-b7b7-8df00cdabff8"
  }
  
  name        = "${local.cluster_name}-provider"
  regions {
    code = "westus2"
    name = "westus2"
    vnet_name = "yugabyte-vnet-us-west2"
    zones {
      code = "westus2-1"
      name = "westus2-1"
      subnet = "yugabyte-subnet-westus2"
    }
    zones {
      code = "westus2-2"
      name = "westus2-2"
      subnet = "yugabyte-subnet-westus2"
    }
    zones {
      code = "westus2-3"
      name = "westus2-3"
      subnet = "yugabyte-subnet-westus2"
    }
  }
  
  ssh_port        = 22
  air_gap_install = false
}

data "yb_provider_key" "azure-key" {
  provider_id = yb_cloud_provider.azure.id
}

locals {
  region_list  = yb_cloud_provider.azure.regions[*].uuid
  provider_id  = yb_cloud_provider.azure.id
  provider_key = data.yb_provider_key.azure-key.id
}

resource "yb_universe" "azure_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "terraform-azure-in-gcp"
      provider_type      = "azu"
      provider           = local.provider_id
      region_list        = local.region_list
      num_nodes          = 1
      replication_factor = 1
      instance_type      = "Standard_D2ds_v5"
      device_info {
        disk_iops     = 3000
      num_volumes   = 1
      storage_class = "standard"
      storage_type  = "Premium_LRS" 
      throughput    = 125 
      volume_size   = 250 
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