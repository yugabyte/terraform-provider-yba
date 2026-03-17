# Basic Azure Provider
resource "yba_azure_provider" "example" {
  name            = "azure-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
    zones {
      code   = "eastus-2"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}

# Azure Provider with custom SSH key pair
resource "yba_azure_provider" "ssh_example" {
  name            = "azure-ssh-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  ssh_keypair_name        = "my-keypair"
  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  regions {
    code = "westus2"
    vnet = "<vnet-name>"

    zones {
      code   = "westus2-1"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}

# Azure Provider with multiple regions
resource "yba_azure_provider" "multi_region_example" {
  name            = "azure-multi-region-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  regions {
    code              = "eastus"
    vnet              = "<eastus-vnet>"
    security_group_id = "<eastus-nsg-id>"

    zones {
      code   = "eastus-1"
      subnet = "<eastus-subnet-1>"
    }
    zones {
      code   = "eastus-2"
      subnet = "<eastus-subnet-2>"
    }
  }

  regions {
    code              = "westus2"
    vnet              = "<westus2-vnet>"
    security_group_id = "<westus2-nsg-id>"

    zones {
      code   = "westus2-1"
      subnet = "<westus2-subnet-1>"
    }
  }

  air_gap_install = false
}

# Azure Provider with separate network subscription
resource "yba_azure_provider" "network_sub_example" {
  name            = "azure-network-sub-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  network_subscription_id = "<azure-network-subscription-id>"
  network_resource_group  = "<azure-network-resource-group>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}

# Azure Provider with custom image bundle
resource "yba_azure_provider" "image_bundle_example" {
  name            = "azure-image-bundle-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }

  image_bundles {
    name           = "custom-x86-bundle"
    use_as_default = true
    details {
      arch     = "x86_64"
      ssh_user = "azureuser"
      ssh_port = 22
      region_overrides = {
        "eastus" = "/subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Compute/images/<image-name>"
      }
    }
  }

  air_gap_install = false
}

# Azure Provider with Private DNS Zone
resource "yba_azure_provider" "dns_example" {
  name            = "azure-dns-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  hosted_zone_id = "<private-dns-zone-name>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}
