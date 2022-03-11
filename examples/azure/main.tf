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
  dir = "/Users/stevendu/code/terraform-provider-yugabyte-anywhere/modules/resources"
}

module "azure-platform" {
  source = "../../modules/azure"

  cluster_name = "sdu-test-yugaware"
  ssh_user     = "sdu"
  region_name  = "westus2"
  subnet_name    = "***REMOVED***"
  vnet_name = "***REMOVED***"
  vnet_resource_group = "yugabyte-rg"
  // files
  ssh_private_key               = "/Users/stevendu/.ssh/yugaware-azure"
  ssh_public_key                = "/Users/stevendu/.ssh/yugaware-azure.pub"
  replicated_filepath           = "${local.dir}/replicated.conf"
  application_settings_filepath = "${local.dir}/application_settings.conf"
  tls_cert_filepath             = ""
  tls_key_filepath              = ""
  license_filepath              = "/Users/stevendu/.yugabyte/yugabyte-dev.rli"
}

provider "yb" {
  host = "${module.azure-platform.public_ip}:80"
}

resource "yb_customer_resource" "customer" {
  depends_on = [module.azure-platform]
  code       = "admin"
  email      = "sdu@yugabyte.com"
  name       = "sdu"
  password   = "Password1@"
}

resource "yb_cloud_provider" "gcp" {
  connection_info {
    cuuid     = yb_customer_resource.customer.cuuid
    api_token = yb_customer_resource.customer.api_token
  }

  code = "gcp"
  config = {
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    YB_FIREWALL_TAGS            = "cluster-server"
  }
  dest_vpc_id = "***REMOVED***"
  name        = "sdu-test-gcp-provider"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 54422
  air_gap_install = false
}

data "yb_provider_key" "gcp-key" {
  connection_info {
    cuuid     = yb_customer_resource.customer.cuuid
    api_token = yb_customer_resource.customer.api_token
  }

  provider_id = yb_cloud_provider.gcp.id
}

locals {
  region_list  = yb_cloud_provider.gcp.regions[*].uuid
  provider_id  = yb_cloud_provider.gcp.id
  provider_key = data.yb_provider_key.gcp-key.id
}

resource "yb_universe" "gcp_universe" {
  connection_info {
    cuuid     = yb_customer_resource.customer.cuuid
    api_token = yb_customer_resource.customer.api_token
  }

  depends_on = [yb_cloud_provider.gcp]
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "sdu-test-gcp-universe-on-azure"
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
      yb_software_version           = "2.13.1.0-b24"
      access_key_code               = local.provider_key
    }
  }
  communication_ports {}
}