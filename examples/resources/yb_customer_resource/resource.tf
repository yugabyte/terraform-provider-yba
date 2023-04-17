provider "yb" {
    alias = "unauthenticated"
    host = "<host ip address>:80"
}

resource "yb_customer_resource" "customer" {
    // use unauthenticcated provider to create customer
    provider   = yb.unauthenticated
    code       = "<code>"
    email      = "<email-id>"
    name       = "<customer-name>"
}