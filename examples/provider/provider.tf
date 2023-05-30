provider "yba" {
  // unauthenticated - to use provider for installation of YugabyteDB Anywhere and customer creation  
  alias = "unauthenticated"
  host  = "<host-ip-address>"
}

provider "yba" {
  // after customer creation, use authenticated provider
  host      = "<host-ip-address>"
  api_token = "<customer-api-token>"
}

provider "yba" {
  // For HTTP based YugabyteDB Anywhere applications
  enable_https = false
  host         = "<host-ip-address>"
  api_token    = "<customer-api-token>"
}
