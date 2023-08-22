resource "azurerm_public_ip" "yb_public_ip" {
  name                = "${var.cluster_name}-public-ip"
  location            = var.region_name
  resource_group_name = var.resource_group
  allocation_method   = "Static"
  sku                 = "Standard"

  tags = merge(var.tags, {
    environment = var.cluster_name
  })
}

resource "azurerm_network_security_group" "yb_sg" {
  location            = var.region_name
  name                = "${var.cluster_name}-1-sg"
  resource_group_name = var.resource_group

  security_rule {
    name              = "yb-rule"
    priority          = 100
    direction         = "Inbound"
    access            = "Allow"
    protocol          = "Tcp"
    source_port_range = "*"
    destination_port_ranges = [
      "22", "8800", "80", "443"
    ]
    source_address_prefix      = var.runner_ip
    destination_address_prefix = "*"
  }

  tags = merge(var.tags, {
    environment = var.cluster_name
  })
}

data "azurerm_subnet" "subnet" {
  name                 = var.subnet_name
  virtual_network_name = var.vnet_name
  resource_group_name  = var.resource_group
}

resource "azurerm_network_interface_security_group_association" "yba_sg_association" {
  depends_on                = [azurerm_network_security_group.yb_sg,azurerm_network_interface.yb_network_interface]
  network_interface_id      = azurerm_network_interface.yb_network_interface.id
  network_security_group_id = azurerm_network_security_group.yb_sg.id
}

resource "azurerm_network_interface" "yb_network_interface" {
  depends_on          = [azurerm_public_ip.yb_public_ip]
  name                = "${var.cluster_name}-network-interface"
  location            = var.region_name
  resource_group_name = var.resource_group

  ip_configuration {
    name                          = "${var.cluster_name}-nic-config"
    subnet_id                     = data.azurerm_subnet.subnet.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.yb_public_ip.id
  }

  tags = merge(var.tags, {
    environment = var.cluster_name
  })
}

resource "azurerm_virtual_machine" "yb_anywhere_node" {
  depends_on                    = [azurerm_network_interface.yb_network_interface, azurerm_public_ip.yb_public_ip]
  name                          = var.cluster_name
  resource_group_name           = var.resource_group
  location                      = var.region_name
  delete_os_disk_on_termination = true

  tags = merge(var.tags, {
    environment = var.cluster_name
  })

  storage_image_reference {
    publisher = "Canonical"
    offer     = "UbuntuServer"
    sku       = "16.04-LTS"
    version   = "latest"
  }

  network_interface_ids = [azurerm_network_interface.yb_network_interface.id]
  vm_size               = var.vm_size

  storage_os_disk {
    name              = "${var.cluster_name}-disk"
    caching           = "ReadWrite"
    create_option     = "FromImage"
    managed_disk_type = "Standard_LRS"
    disk_size_gb      = var.disk_size
  }

  os_profile {
    computer_name  = var.cluster_name
    admin_username = var.ssh_user
  }

  os_profile_linux_config {
    disable_password_authentication = true
    ssh_keys {
      path     = "/home/${var.ssh_user}/.ssh/authorized_keys"
      key_data = file(var.ssh_public_key)
    }
  }
}
