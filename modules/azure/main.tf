resource "azurerm_resource_group" "yb_rg" {
  name     = "${var.cluster_name}-rg"
  location = var.region_name

  tags = {
    environment = var.cluster_name
  }
}

resource "azurerm_virtual_network" "yb_network" {
  depends_on = [azurerm_resource_group.yb_rg]
  name                = "${var.cluster_name}-vpc"
  address_space       = ["10.0.0.0/16"]
  location            = var.region_name
  resource_group_name = azurerm_resource_group.yb_rg.name

  tags = {
    environment = var.cluster_name
  }
}

resource "azurerm_subnet" "yb_subnet" {
  depends_on = [azurerm_resource_group.yb_rg, azurerm_virtual_network.yb_network]
  name                 = "${var.cluster_name}-subnet"
  resource_group_name  = azurerm_resource_group.yb_rg.name
  virtual_network_name = azurerm_virtual_network.yb_network.name
  address_prefixes     = ["10.0.2.0/24"]
}

resource "azurerm_public_ip" "yb_public_ip" {
  depends_on = [azurerm_resource_group.yb_rg]
  name                = "${var.cluster_name}-public-ip"
  location            = var.region_name
  resource_group_name = azurerm_resource_group.yb_rg.name
  allocation_method   = "Static"
  sku                 = "Standard"

  tags = {
    environment = var.cluster_name
  }
}

resource "azurerm_network_security_group" "yb_sg" {
  depends_on = [azurerm_resource_group.yb_rg]
  location            = var.region_name
  name                = "${var.cluster_name}-sg"
  resource_group_name = azurerm_resource_group.yb_rg.name

  security_rule {
    name                       = "yb-rule"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_ranges    = [
      "22", "8800", "80", "7000", "7100", "9000", "9100", "11000", "12000", "9300", "9042", "5433", "6379"
    ]
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  tags = {
    environment = var.cluster_name
  }
}

resource "azurerm_subnet_network_security_group_association" "yb_sg_association" {
  depends_on = [azurerm_subnet.yb_subnet, azurerm_network_security_group.yb_sg]
  subnet_id                 = azurerm_subnet.yb_subnet.id
  network_security_group_id = azurerm_network_security_group.yb_sg.id
}

resource "azurerm_network_interface" "yb_network_interface" {
  depends_on = [azurerm_resource_group.yb_rg, azurerm_subnet.yb_subnet, azurerm_public_ip.yb_public_ip]
  name                = "${var.cluster_name}-network-interface"
  location            = var.region_name
  resource_group_name = azurerm_resource_group.yb_rg.name

  ip_configuration {
    name                          = "${var.cluster_name}-nic-config"
    subnet_id                     = azurerm_subnet.yb_subnet.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.yb_public_ip.id
  }

  tags = {
    environment = var.cluster_name
  }
}

resource "azurerm_virtual_machine" "yb_platform_node" {
  depends_on = [azurerm_resource_group.yb_rg, azurerm_network_interface.yb_network_interface, azurerm_public_ip.yb_public_ip]
  name                = var.cluster_name
  resource_group_name = azurerm_resource_group.yb_rg.name
  location            = var.region_name

  tags = {
    environment = var.cluster_name
  }

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

  // replicated config
  provisioner "file" {
    source      = var.replicated_filepath
    destination = "/tmp/replicated.conf"
    connection {
      host        = azurerm_public_ip.yb_public_ip.ip_address
      type        = "ssh"
      user        = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // tls certificate
  #  provisioner "file" {
  #    source = var.tls_cert_filepath
  #    destination ="/tmp/server.crt"
  #    connection {
  #      host = azurerm_public_ip.yb_public_ip.ip_address
  #      type = "ssh"
  #      user = var.ssh_user
  #      private_key = file(var.ssh_private_key)
  #    }
  #  }

  // tls key
  #  provisioner "file" {
  #    source = var.tls_key_filepath
  #    destination ="/tmp/server.key"
  #    connection {
  #      host = azurerm_public_ip.yb_public_ip.ip_address
  #      type = "ssh"
  #      user = var.ssh_user
  #      private_key = file(var.ssh_private_key)
  #    }
  #  }

  // license file
  provisioner "file" {
    source      = var.license_filepath
    destination = "/tmp/license.rli"
    connection {
      host        = azurerm_public_ip.yb_public_ip.ip_address
      type        = "ssh"
      user        = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // application settings
  provisioner "file" {
    source      = var.application_settings_filepath
    destination = "/tmp/settings.conf"
    connection {
      host        = azurerm_public_ip.yb_public_ip.ip_address
      type        = "ssh"
      user        = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // install replicated
  provisioner "remote-exec" {
    inline = [
      "sudo mv /tmp/replicated.conf /etc/replicated.conf",
      "curl -sSL https://get.replicated.com/docker | sudo bash",
    ]
    connection {
      host        = azurerm_public_ip.yb_public_ip.ip_address
      type        = "ssh"
      user        = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }
}
