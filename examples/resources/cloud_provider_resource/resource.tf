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
  host = "http://portal.dev.yugabyte.com"
}

data "yb_customer" "customer" {}

resource "yb_cloud_provider" "gcp" {
  customer_id = data.yb_customer.customer.id
  code = "gcp"
  config = {
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    ***REMOVED***
    YB_FIREWALL_TAGS = "cluster-server"
  }
  dest_vpc_id = "***REMOVED***"
  name = "my-gcp-provider"
  regions {
    code = "us-central1"
    name = "us-central1"
  }
  ssh_port = 54422
  air_gap_install = false
}

output "provider" {
  value = yb_cloud_provider.gcp
  sensitive = true
}
