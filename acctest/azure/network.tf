# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Resource group, one VNet with four subnets (one for the YBA control-plane VM,
# three for the YBDB universe nodes YBA provisions, one per westus2 zone), and a
# network security group opening operator access plus intra-VNet traffic. One
# VNet means no peering needed.

# Dedicated resource group for the whole fixture.
resource "azurerm_resource_group" "main" {
  name     = "${var.prefix}-rg"
  location = var.azure_region
  tags     = var.tags
}

resource "azurerm_virtual_network" "main" {
  name                = "${var.prefix}-vnet"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  address_space       = [var.vnet_cidr]
  tags                = var.tags
}

# Subnet hosting the YBA control-plane VM.
resource "azurerm_subnet" "yba" {
  name                 = "${var.prefix}-yba"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [var.yba_subnet_cidr]
}

# Three subnets hosting YBDB universe nodes, one per westus2 zone. The Azure
# MultipleZones acceptance test references all three (AZURE_SUBNET_ID_1/2/3) and
# the single-zone tests use the first (AZURE_SUBNET_ID).
resource "azurerm_subnet" "ybdb" {
  count                = length(var.ybdb_subnet_cidrs)
  name                 = "${var.prefix}-ybdb-${count.index + 1}"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = [var.ybdb_subnet_cidrs[count.index]]
}

# Network security group, kept minimal: allow everything inside the VNet, plus
# operator access (SSH + YBA UI/API + Prometheus) from var.operator_cidr_ranges.
resource "azurerm_network_security_group" "main" {
  name                = "${var.prefix}-nsg"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags

  # Allow all traffic between nodes inside the VNet (YBA <-> YBDB, YBDB <-> YBDB).
  security_rule {
    name                       = "allow-intra-vnet"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "VirtualNetwork"
    destination_address_prefix = "VirtualNetwork"
  }

  # Allow operator access to YBA (SSH for the installer, 443 for the UI/API,
  # 9090 for Prometheus) from the configured operator ranges.
  security_rule {
    name                       = "allow-operator"
    priority                   = 200
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_ranges    = ["22", "443", "9090"]
    source_address_prefixes    = var.operator_cidr_ranges
    destination_address_prefix = "*"
  }
}

# Associate the NSG with every subnet so the rules apply uniformly.
resource "azurerm_subnet_network_security_group_association" "yba" {
  subnet_id                 = azurerm_subnet.yba.id
  network_security_group_id = azurerm_network_security_group.main.id
}

resource "azurerm_subnet_network_security_group_association" "ybdb" {
  count                     = length(azurerm_subnet.ybdb)
  subnet_id                 = azurerm_subnet.ybdb[count.index].id
  network_security_group_id = azurerm_network_security_group.main.id
}
