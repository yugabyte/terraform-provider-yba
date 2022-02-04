terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "yb" {
  apikey = "***REMOVED***"
  host = "portal.dev.yugabyte.com"
}

data "yb_customer" "customer" {}

output "customer" {
  value = data.yb_customer.customer
}