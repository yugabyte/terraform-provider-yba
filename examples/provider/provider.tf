provider "yba" {
  // unauthenticated - to use provider for installation and customer creation  
  alias = "unauthenticated"
  host  = "<host-ip-address>:80"
}

provider "yba" {
  // after customer creation, use authenticated provider
  host      = "<host-ip-address>:80"
  api_token = "<customer-api-token>"
}
