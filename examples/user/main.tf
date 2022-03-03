terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "yb" {
  // these can be set as environment variables
  apikey = "***REMOVED***"
  host = "localhost:9000"
}

resource "yb_user" "user" {
  email = "sdu@yugabyte.com"
  password = "Password1@#"
  role = "ReadOnly"
}