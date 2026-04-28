// Azure Blob Storage configuration with SAS token.
resource "yba_azure_storage_config" "azure" {
  name            = "my-azure-config"
  backup_location = "https://<account>.blob.core.windows.net/<container>"
  sas_token       = "<azure-sas-token>"
}

// Azure Blob Storage configuration using managed identity.
resource "yba_azure_storage_config" "azure_iam" {
  name            = "azure-managed-identity"
  backup_location = "https://<account>.blob.core.windows.net/<container>"
  use_azure_iam   = true
}
