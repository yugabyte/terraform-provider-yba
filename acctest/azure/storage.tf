# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Backup storage account + container for the Azure storage-config acceptance
# tests. A container-scoped SAS token is generated for the storage-config
# (the test passes backup_location + sas_token to yba_azure_storage_config).

# Storage account names are globally unique, 3-24 chars, lowercase alphanumeric
# only (no hyphens), so derive one from the de-hyphenated prefix plus a random
# suffix rather than reusing var.prefix verbatim.
resource "random_string" "storage_suffix" {
  length  = 8
  upper   = false
  special = false
}

locals {
  storage_account_name = substr(
    "${replace(var.prefix, "-", "")}${random_string.storage_suffix.result}", 0, 24
  )
}

resource "azurerm_storage_account" "backups" {
  name                     = local.storage_account_name
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  min_tls_version          = "TLS1_2"
  tags                     = var.tags
}

resource "azurerm_storage_container" "backups" {
  name                  = "backups"
  storage_account_id    = azurerm_storage_account.backups.id
  container_access_type = "private"
}

# Account-level SAS token scoped to blob storage, with the permissions YBA needs
# to write and manage backups. The start/expiry below are a fixed calendar window
# (not relative to apply, which would churn the token on every plan), so refresh
# it by bumping those dates before it expires. Surfaced via test_env (out-vars.tf).
data "azurerm_storage_account_sas" "backups" {
  connection_string = azurerm_storage_account.backups.primary_connection_string
  https_only        = true

  resource_types {
    service   = true
    container = true
    object    = true
  }

  services {
    blob  = true
    queue = false
    table = false
    file  = false
  }

  start  = "2026-01-01T00:00:00Z"
  expiry = "2027-01-01T00:00:00Z"

  permissions {
    read    = true
    write   = true
    delete  = true
    list    = true
    add     = true
    create  = true
    update  = false
    process = false
    tag     = false
    filter  = false
  }
}
