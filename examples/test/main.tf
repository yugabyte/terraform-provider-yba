terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

data "yb_customer_data" "customer" {
  api_token = "***REMOVED***"
}

provider "yb" {
  // these can be set as environment variables
  host = "portal.dev.yugabyte.com"
}

resource "yb_cloud_provider" "gcp" {
  connection_info {
    cuuid     = data.yb_customer_data.customer.cuuid
    api_token = data.yb_customer_data.customer.api_token
  }

  code = "gcp"
  config = merge(
    { YB_FIREWALL_TAGS = "cluster-server" },
    jsondecode(file("/Users/stevendu/.yugabyte/yugabyte-gce.json"))
  )
  dest_vpc_id = "yugabyte-network"
  name        = "sdu-test-gcp-provider"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 54422
  air_gap_install = false
}