# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# The acceptance-test env as KEY='value' lines, ready to source into a shell.
# Holds the base topology (TF_VAR_AZURE_*) and the YBA endpoint
# (TF_VAR_AZURE_YBA_HOST/TF_VAR_AZURE_YBA_API_KEY). `make -C acctest env` sources
# every fixture's test_env into the single, gitignored `env` file; CI writes that
# file back from the ACCTEST_ENV secret and sources it the same way.
#
# The yba_azure_provider resource takes the VNet and subnet by *name* (not full
# resource ID) — see internal/provider/azure/resource_azure_provider.go ("Virtual
# network name", "Subnet for this zone") — so the VNet/subnet vars are names. The
# subscription/RG/tenant/client values are what YBA uses to authenticate the SP.

locals {
  backup_location = "https://${azurerm_storage_account.backups.name}.blob.core.windows.net/${azurerm_storage_container.backups.name}"

  test_env = <<-EOT
    TF_VAR_AZURE_SUBSCRIPTION_ID='${var.azure_subscription_id}'
    TF_VAR_AZURE_TENANT_ID='${var.azure_tenant_id}'
    TF_VAR_AZURE_RG='${azurerm_resource_group.main.name}'
    TF_VAR_AZURE_CLIENT_ID='${azuread_application.yba.client_id}'
    TF_VAR_AZURE_CLIENT_SECRET='${azuread_service_principal_password.yba.value}'
    TF_VAR_AZURE_NETWORK_SUBSCRIPTION_ID='${var.azure_subscription_id}'
    TF_VAR_AZURE_NETWORK_RG='${azurerm_resource_group.main.name}'
    TF_VAR_AZURE_VNET_ID='${azurerm_virtual_network.main.name}'
    TF_VAR_AZURE_SUBNET_ID='${azurerm_subnet.ybdb[0].name}'
    TF_VAR_AZURE_SUBNET_ID_1='${azurerm_subnet.ybdb[0].name}'
    TF_VAR_AZURE_SUBNET_ID_2='${azurerm_subnet.ybdb[1].name}'
    TF_VAR_AZURE_SUBNET_ID_3='${azurerm_subnet.ybdb[2].name}'
    TF_VAR_AZURE_BACKUP_LOCATION='${local.backup_location}'
    TF_VAR_AZURE_SAS_TOKEN='${data.azurerm_storage_account_sas.backups.sas}'
    TF_VAR_AZURE_YBA_HOST='${azurerm_public_ip.yba.ip_address}'
    TF_VAR_AZURE_YBA_API_KEY='${yba_customer_resource.customer.api_token}'
    AZURE_SUBSCRIPTION_ID='${var.azure_subscription_id}'
    AZURE_TENANT_ID='${var.azure_tenant_id}'
    AZURE_RG='${azurerm_resource_group.main.name}'
    AZURE_CLIENT_ID='${azuread_application.yba.client_id}'
    AZURE_CLIENT_SECRET='${azuread_service_principal_password.yba.value}'
  EOT
}

# The acceptance-test env, read at run time by `make -C acctest env`.
output "test_env" {
  description = "Acceptance-test env (TF_VAR_AZURE_*) as KEY='value' lines."
  value       = local.test_env
  sensitive   = true # contains AZURE_CLIENT_SECRET, AZURE_SAS_TOKEN and the YBA API key
}

output "yba_url" {
  value = "https://${azurerm_public_ip.yba.ip_address}"
}

output "yba_username" {
  description = "Username (email) of the initial YBA superuser."
  value       = var.yba_username
}

output "yba_password" {
  description = "Password of the initial YBA superuser."
  value       = random_password.customer.result
  sensitive   = true
}
