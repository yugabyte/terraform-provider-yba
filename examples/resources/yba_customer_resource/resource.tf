provider "yba" {
  alias = "unauthenticated"
  host  = "<host-ip-address>"
}

variable "customer_password" {
  type      = string
  sensitive = true
}

resource "yba_customer_resource" "customer" {
  // use unauthenticated provider to create customer
  provider = yba.unauthenticated
  code     = "<code>"
  email    = "<email-id>"
  name     = "<customer-name>"
  password = var.customer_password
}
