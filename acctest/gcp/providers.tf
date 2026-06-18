# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Base for GCP acceptance tests: VPC, IAM, the YBA VM + install (yba.tf), and
# the `test_env` output holding the test env. Applied once, reused for many
# `make acctest` runs.
#
# The `yba` provider here is our locally-built dev binary, used only to install
# YBA and register the first customer (the bootstrap resources in yba.tf). It
# is wired via dev_overrides, so `make acctest-setup-gcp` runs `make install`
# first. NOTE: with dev_overrides in ~/.terraformrc, `terraform init` does not
# install yba and prints a harmless "development overrides are in effect" warning.

terraform {
  required_version = ">= 1.5"

  # State in GCS (gs://tf-acctest-tfstate/gcp).
  backend "gcs" {
    bucket = "tf-acctest-tfstate"
    prefix = "gcp"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
    yba = {
      source = "yugabyte/yba"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = ">= 4.0"
    }
  }
}

provider "google" {
  project        = var.gcp_project_id
  region         = var.gcp_region
  default_labels = var.labels
}

# Bootstrap provider for the install/first-customer flow: it points at the YBA
# VM's address but carries no api_token (the provider runs unauthenticated until
# yba_customer_resource registers the first user and mints the token). The
# installer ignores host (it works over SSH); the customer resource needs it to
# reach the freshly-installed YBA instead of the default localhost:9000.
provider "yba" {
  alias = "bootstrap"
  host  = google_compute_address.yba.address
}
