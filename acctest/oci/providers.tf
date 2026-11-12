# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base for OCI acceptance tests. Applied once by an operator with
# `make -C acctest apply-oci`. `make env` includes its output once this
# directory is initialized.

terraform {
  required_version = ">= 1.5"

  backend "gcs" {
    bucket = "tf-acctest-tfstate"
    prefix = "oci"
  }

  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "~> 6.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    yba = {
      source = "yugabyte/yba"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "oci" {
  alias  = "home"
  region = var.home_region
}

provider "oci" {
  alias  = "region2"
  region = var.region_2
}

# Unauthenticated provider for bootstrap (yba_installer installs YBA and
# yba_customer_resource registers the first user and mints the token). The
# installer ignores host (it works over SSH); the customer resource needs it to
# reach the freshly-installed YBA instead of the default localhost:9000.
provider "yba" {
  alias = "bootstrap"
  host  = oci_core_instance.yba.public_ip
  # Set, so a YBA_API_KEY in the shell cannot leave unauthenticated mode.
  api_token = ""
}

provider "yba" {
  alias     = "admin"
  host      = oci_core_instance.yba.public_ip
  api_token = yba_customer_resource.customer.api_token
}
