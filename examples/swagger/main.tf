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
  host = "portal.dev.yugabyte.com"
}

resource "yb_cloud_provider" "gcp" {
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
  dest_vpc_id = "yugabyte-network"
  name = "sdu-test-gcp-provider"
  regions {
    code = "us-central1"
    name = "us-central1"
  }
  ssh_port = 54422
  air_gap_install = false
}

data "yb_provider_key" "gcp-key" {
  provider_id = yb_cloud_provider.gcp.id
}