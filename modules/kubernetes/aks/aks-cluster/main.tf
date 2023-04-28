terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
    }
  }
}

resource "azurerm_resource_group" "rg" {
  name     = var.cluster_name
  location = var.region_name
}

resource "azurerm_kubernetes_cluster" "yb-anywhere" {
  name                = var.cluster_name
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  dns_prefix          = var.cluster_name

  default_node_pool {
    name       = "default"
    node_count = var.num_nodes
    vm_size    = "Standard_DS4_v2"
  }

  identity {
    type = "SystemAssigned"
  }
}