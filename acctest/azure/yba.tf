# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# The acceptance-test YBA: a control-plane VM, the YBA install (over SSH), and
# the initial customer. Applied once as part of the base; its endpoint
# (TF_VAR_AZURE_YBA_HOST/TF_VAR_AZURE_YBA_API_KEY) is exposed through the
# `test_env` output so `make acctest` just consumes it. Tear down with the base.
# Mirrors acctest/gcp.

locals {
  yba_ssh_host = azurerm_public_ip.yba.ip_address
}

# Dedicated SSH keypair for the standing YBA VM, generated once and kept in the
# shared remote state (alongside random_password.customer). The public half goes
# on the VM and the private half is fed to the installer inline via
# ssh_private_key, so applies don't depend on (or drift against) any developer's
# ~/.ssh keypair. Passing it inline (not as a file path) sidesteps the
# installer's plan-time file-existence check, which a key generated in the same
# apply can't satisfy.
resource "tls_private_key" "yba" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Static public IP for the YBA VM: a stable address for the installer (SSH) and
# the UI/API. Exposed as TF_VAR_AZURE_YBA_HOST.
resource "azurerm_public_ip" "yba" {
  name                = "${var.prefix}-yba"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  allocation_method   = "Static"
  sku                 = "Standard"
  tags                = var.tags
}

resource "azurerm_network_interface" "yba" {
  name                = "${var.prefix}-yba"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags

  ip_configuration {
    name                          = "primary"
    subnet_id                     = azurerm_subnet.yba.id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.yba.id
  }
}

# Persistent state for YBA, mounted at /opt/yugabyte/data by the custom-data
# script. Kept on a separate disk as in byoc-setup.
resource "azurerm_managed_disk" "data" {
  name                 = "${var.prefix}-yba-data"
  location             = azurerm_resource_group.main.location
  resource_group_name  = azurerm_resource_group.main.name
  storage_account_type = "Premium_LRS"
  create_option        = "Empty"
  disk_size_gb         = 250
  tags                 = var.tags
}

# Single YBA control-plane VM (no HA).
resource "azurerm_linux_virtual_machine" "yba" {
  name                = "${var.prefix}-yba"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  size                = var.yba_vm_size
  admin_username      = var.yba_admin_user
  tags                = var.tags

  network_interface_ids = [azurerm_network_interface.yba.id]

  # custom_data runs the mount/preflight script on first boot (cloud-init runs
  # an executable custom_data payload directly).
  custom_data = base64encode(file("${path.module}/../resources/azure-mount-data-disk.sh"))

  admin_ssh_key {
    username   = var.yba_admin_user
    public_key = tls_private_key.yba.public_key_openssh
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Premium_LRS"
    disk_size_gb         = 100
  }

  source_image_reference {
    publisher = var.base_image.publisher
    offer     = var.base_image.offer
    sku       = var.base_image.sku
    version   = var.base_image.version
  }
}

# Attach the data disk at LUN 0; the custom-data script resolves it via the
# stable /dev/disk/azure/scsi1/lun0 symlink.
resource "azurerm_virtual_machine_data_disk_attachment" "data" {
  managed_disk_id    = azurerm_managed_disk.data.id
  virtual_machine_id = azurerm_linux_virtual_machine.yba.id
  lun                = 0
  caching            = "ReadWrite"
}

# Randomly generated password for the initial YBA superuser.
resource "random_password" "customer" {
  length           = 16
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
  override_special = "!#$%*-_"
}

# Install YugabyteDB Anywhere on the VM over SSH. The SSH key is the generated
# keypair (passed inline); the license file is at the repo root and yba-ctl.yml
# is in acctest/resources (both validated to exist at plan time). The installer
# connects as the VM admin user.
resource "yba_installer" "install" {
  provider = yba.bootstrap

  ssh_host_ip               = local.yba_ssh_host
  ssh_user                  = var.yba_admin_user
  ssh_private_key           = tls_private_key.yba.private_key_openssh
  yba_license_file          = "${path.module}/../../yugabyte_anywhere.lic"
  application_settings_file = "${path.module}/../resources/yba-ctl.yml"
  yba_version               = var.yba_version
  host_os                   = "linux"
  host_architecture         = "x86_64"

  # The VM is an implicit dependency (via ssh_host_ip), but the data-disk
  # attachment and NSG that opens SSH/443 are not — without this the installer
  # can start before the disk is mounted or the firewall exists and fail.
  depends_on = [
    azurerm_virtual_machine_data_disk_attachment.data,
    azurerm_subnet_network_security_group_association.yba,
  ]
}

# Register the initial superuser; exposes the API token (published as
# TF_VAR_AZURE_YBA_API_KEY).
resource "yba_customer_resource" "customer" {
  provider = yba.bootstrap

  code     = "admin"
  email    = var.yba_username
  name     = "admin"
  password = random_password.customer.result

  lifecycle {
    ignore_changes = [password]
  }

  depends_on = [yba_installer.install]
}
