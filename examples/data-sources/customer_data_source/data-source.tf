terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

data "yb_customer" "customer" {}

output "customer" {
  value = data.yb_customer.customer
}