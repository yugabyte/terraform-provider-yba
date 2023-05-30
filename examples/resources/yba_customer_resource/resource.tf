provider "yba" {
  alias = "unauthenticated"
  host  = "<host-ip-address>"
}

resource "yba_customer_resource" "customer" {
  // use unauthenticcated provider to create customer
  provider = yba.unauthenticated
  code     = "<code>"
  email    = "<email-id>"
  name     = "<customer-name>"
}
