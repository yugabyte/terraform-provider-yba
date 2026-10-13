# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Identities for the Azure acceptance-test fixture:
#   - A service principal that YBA uses to provision universe VMs and manage the
#     network it needs. Its client id/secret are surfaced via the test_env
#     output (out-vars.tf) for the yba_azure_provider tests, and destroyed on
#     teardown.
#   - A user-assigned managed identity on the YBA VM (yba.tf), so that the
#     yba_azure_ear_config test can authenticate as the YBA host.

data "azuread_client_config" "current" {}

# Entra ID application backing the service principal.
resource "azuread_application" "yba" {
  display_name = "${var.prefix}-yba"
  owners       = [data.azuread_client_config.current.object_id]
}

resource "azuread_service_principal" "yba" {
  client_id = azuread_application.yba.client_id
  owners    = [data.azuread_client_config.current.object_id]
}

# Client secret for the SP, minted for the tests (passed to yba_azure_provider
# client_secret). The only long-lived credential here.
resource "azuread_service_principal_password" "yba" {
  service_principal_id = azuread_service_principal.yba.id
}

# Contributor on the resource group is fine-grained enough for YBA to provision
# VMs (with disks, NICs, public IPs) and manage the network it needs, while
# keeping the SP scoped to this fixture's resource group rather than the whole
# subscription.
resource "azurerm_role_assignment" "yba_contributor" {
  scope                = azurerm_resource_group.main.id
  role_definition_name = "Contributor"
  principal_id         = azuread_service_principal.yba.object_id
}

# Managed identity of the YBA VM. YBA authenticates as it when an Azure
# encryption-at-rest configuration has no client secret and names this
# identity's client id. Its only permissions are on the key vault key (kms.tf).
resource "azurerm_user_assigned_identity" "yba" {
  name                = "${var.prefix}-yba"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  tags                = var.tags
}
