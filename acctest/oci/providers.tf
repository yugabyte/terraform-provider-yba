# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base for OCI acceptance tests. Applied once by an operator with
# `make -C acctest apply-oci`. `make env` includes its output once
# `make apply-oci` has succeeded.

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
  alias  = "test"
  region = var.test_region
}

# Unauthenticated provider for yba_installer and yba_customer_resource. The
# installer works over SSH and ignores host; the customer resource uses it.
provider "yba" {
  alias = "bootstrap"
  host  = oci_core_instance.yba.public_ip
  # Empty, so a YBA_API_KEY in the shell is not used.
  api_token = ""
}

provider "yba" {
  alias     = "admin"
  host      = oci_core_instance.yba.public_ip
  api_token = yba_customer_resource.customer.api_token
}
