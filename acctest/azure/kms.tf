# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Key vault key for the encryption-at-rest acceptance test
# (yba_azure_ear_config). The test points YBA at this existing key, so no test
# run creates keys in the vault.
#
# The vault uses Azure RBAC. The operator who applies the fixture creates,
# reads and deletes the key, so the operator needs Key Vault Crypto Officer or
# Key Vault Administrator on the subscription or the resource group before
# the apply. A role assignment in the same apply can take minutes to take
# effect, which fails the key create.

# Key vault names are globally unique, 3-24 chars, letters, digits and
# hyphens. Derive one from the de-hyphenated prefix, as storage.tf does.
resource "random_string" "key_vault_suffix" {
  length  = 6
  upper   = false
  special = false
}

locals {
  key_vault_name = substr(
    "${replace(var.prefix, "-", "")}ear${random_string.key_vault_suffix.result}", 0, 24
  )
}

resource "azurerm_key_vault" "ear" {
  name                       = local.key_vault_name
  location                   = azurerm_resource_group.main.location
  resource_group_name        = azurerm_resource_group.main.name
  tenant_id                  = var.azure_tenant_id
  sku_name                   = "standard"
  rbac_authorization_enabled = true
  soft_delete_retention_days = 7
  purge_protection_enabled   = false
  tags                       = var.tags
}

# YBA accepts an existing key that is enabled, has no expiration date, and
# permits wrapKey and unwrapKey.
resource "azurerm_key_vault_key" "ear" {
  name         = "yba-ear"
  key_vault_id = azurerm_key_vault.ear.id
  key_type     = "RSA"
  key_size     = 2048
  key_opts     = ["wrapKey", "unwrapKey"]
}

# YBA's permissions on the key (Get, List, Wrap Key and Unwrap Key), for both
# identities the test uses: the service principal (client secret) and the YBA
# VM's managed identity (iam.tf).
resource "azurerm_role_assignment" "ear_key_user" {
  for_each = {
    service_principal = azuread_service_principal.yba.object_id
    vm_identity       = azurerm_user_assigned_identity.yba.principal_id
  }

  scope                = azurerm_key_vault_key.ear.resource_versionless_id
  role_definition_name = "Key Vault Crypto User"
  principal_id         = each.value
  principal_type       = "ServicePrincipal"
}
