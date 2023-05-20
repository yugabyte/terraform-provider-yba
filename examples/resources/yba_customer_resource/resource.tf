provider "yba" {
  alias = "unauthenticated"
  host  = "<host-ip-address>:80"
}

resource "yba_customer_resource" "customer" {
  // use unauthenticcated provider to create customer
  provider = yba.unauthenticated
  code     = "<code>"
  email    = "<email-id>"
  name     = "<customer-name>"
}
