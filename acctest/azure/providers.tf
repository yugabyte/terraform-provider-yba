# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base for Azure acceptance tests: resource group, VNet, a service principal for
# YBA, the YBA control-plane VM + install (yba.tf), a backups storage account,
# and the `test_env` output holding the test env. Applied once, reused for many
# `make acctest` runs.
#
# The `yba` provider here is our locally-built dev binary, used only to install
# YBA and register the first customer (the bootstrap resources in yba.tf). It is
# wired via dev_overrides, so the `init-azure` / `apply-azure` make targets run
# `make install` first. NOTE: with dev_overrides in ~/.terraformrc, `terraform
# init` does not install yba and prints a harmless "development overrides are in
# effect" warning.

terraform {
  required_version = ">= 1.5"

  # State in GCS (gs://tf-acctest-tfstate/azure), the same bucket the gcp fixture
  # uses, just a different prefix. Reusing GCS (not an azurerm backend) keeps the
  # state plumbing identical across all cloud fixtures.
  backend "gcs" {
    bucket = "tf-acctest-tfstate"
    prefix = "azure"
  }

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 3.0"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = ">= 2.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = ">= 4.0"
    }
    yba = {
      source = "yugabyte/yba"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

provider "azurerm" {
  subscription_id = var.azure_subscription_id
  tenant_id       = var.azure_tenant_id

  features {}
}

provider "azuread" {
  tenant_id = var.azure_tenant_id
}

# Bootstrap provider for the install/first-customer flow: it points at the YBA
# VM's address but carries no api_token (the provider runs unauthenticated until
# yba_customer_resource registers the first user and mints the token). The
# installer ignores host (it works over SSH); the customer resource needs it to
# reach the freshly-installed YBA instead of the default localhost:9000.
provider "yba" {
  alias = "bootstrap"
  host  = azurerm_public_ip.yba.ip_address
}
