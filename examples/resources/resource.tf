terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "yb" {
  apikey = "e007de9f-835c-46f0-9e0d-9e8422c93c89"
  host = "portal.dev.yugabyte.com"
}

resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  custom_host_cidrs = []
  config = {
    type = "service_account"
    project_id = "yugabyte"
    private_key_id = "8f57b0753f55960b1f3276ee0db9e8cbcf10d0bd"
    private_key = "-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDYHyUo/KVlJ7PK\n6rHLS69IxaSQAMrQCBWMBP5DY8TdWC41pghpSsu3/GRTDoZ4hw7WsX73bMfVdttK\nEjW8bTfeitMnHCBINXqVLlnd8yrAJ5ZCSiI1dceQ7x/NCUpiH1D6f2mXkJn//wlF\nKcGGYU+qj7B6XlpZZxN8hHLk5tq561qami5kMYSATHfYQI0Z2kgbrsjWazkcm5tu\n0x6TWJ8c8dfc4NgfKH0TfsOXfqQSDYbwe4g5a88MXD2+qTZUvTGi6p9Rf4SDoELC\ngk4G4/te99w27bS0IBQq15l8L3ppSYBuxWM3vPj9xooxaZKNlpN7PfEswUj/9wyS\nXG/aVQePAgMBAAECggEAISZx2D4cjo4O9XaXa/QBgHuUeOQuN8etqmsPpz2T8lG2\n0NLVYnUvF1sW9mh5dt5ch9D1BTXB1zviehOd+3eTRMbtiYe2ae0ODvjrnvBQI+ZO\nlX9yjNmykUgkjBo7Nx7PmITXqQBspsgzX1D+1sJxaluc+cAkQqddZVGZoAPLFA4+\nD48GsGiufTLkj85IY6M+/ziOeex8Ah03F8tPOkEmSsxd1QvRpW6boh+YMpAk1fii\noberPLTG9cb8URNu/KSB/KP2vXPakTt3p4rXzqRX+jfyZK7h2Nd8fWv3mnHjwicc\n8ZPbiFgtPkA4licGmn/EygNUIPDrwSce+HWvA310xQKBgQDtO+3nlJoOrWO4O4f/\nSwwvEg16Xp+4dfmZLMBfnnQBmOJh5cOzX4rpFTsRW8jca5oNfUDRysq/V3SbAx0g\nwr9wXY/QNhvWbCxO+h0IEmhuzeAKsmVy/GxoBssFyM6GDLXkJeqMKvKLJJbzFV5O\nIuw6p5XqLECXsDmQ1jVjn6NU5QKBgQDpN66Mrepum3JS0+xPKapW9bOMUDinIMN3\nXVmhfY+iEBu4MVAvk/j5mQ44t346S+tKbiqoQ2mXbYy0spBfrN79BhkB00Q0BEaj\nS9pLdMPolnFhbKnOy5HexSmr6+A63S9IlbEV0bqjRAsdKtmM7msoYfjHAtGh3UJz\n1p7uWlY3YwKBgQCpJfnTDNlrbaWUTp4BIPlm9nA1uBIZ68QzuvzPMKN2IBQJyVFo\nK89XsZOUJOVqhC4rQAtfikBVfX3eqLG0Eid9briDtJDUqfxNs3fPsZBUsOX1uo0r\nF2AULAPF9A+M9LMcIQzDNDwLieM3Hx1GiQ/2Ild5yGOlxDjHVHRsu/4xIQKBgDoJ\nXkmp+f3+dwu/qz3j+3zadg0D5aVJlPr+YxC6A2VsJsnGk9LTOxE6EnzwxNvTCsGh\n+sGWzQ8e9vX8vcrhZTiILO70WTOsoLuAY9mFPD+EOMDq3rMUm79ZR05+S3W6l0qz\n3ba1U4HPrAhdInhc2JPbFaLIw8xJGIFlNnXQS0ZLAoGBAJ33nae9WHUbUIUoHYKg\nMAhOIiERkkNNdSoNjMt+ik8mMvZjbKVGrq5Wl1kEf1PoSkHVl48nV1Y3u2BIt9XC\nxCNaXrphVkWZTBEnsTyxVZ6nukolaiLSXfKIBXFh/hPhE02Xey9MetHXgt5Z/YBo\n8xFlQngiAwbr1aYggQHo96lj\n-----END PRIVATE KEY-----\n"
    client_email = "811619402015-compute@developer.gserviceaccount.com"
    client_id = "107942268808569032883"
    auth_uri = "https://accounts.google.com/o/oauth2/auth"
    token_uri = "https://accounts.google.com/o/oauth2/token"
    auth_provider_x509_cert_url = "https://www.googleapis.com/oauth2/v1/certs"
    client_x509_cert_url = "https://www.googleapis.com/robot/v1/metadata/x509/811619402015-compute%40developer.gserviceaccount.com"
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

locals {
  region_list = yb_cloud_provider.gcp.regions[*].uuid
  provider_id = yb_cloud_provider.gcp.id
  provider_key = data.yb_provider_key.gcp-key.id
}

output "provider" {
  value = yb_cloud_provider.gcp
}

#resource "yb_universe" "gcp_universe" {
#  depends_on = [yb_cloud_provider.gcp]
#  clusters {
#    cluster_type = "PRIMARY"
#    user_intent {
#      universe_name = "sdu-test-gcp-universe"
#      provider_type = "gcp"
#      provider = local.provider_id
#      region_list = local.region_list
#      num_nodes = 3
#      replication_factor = 3
#      instance_type = "n1-standard-1"
#      device_info {
#        num_volumes = 1
#        volume_size = 375
#        storage_type = "Persistent"
#      }
#      assign_public_ip = true
#      use_time_sync = true
#      enable_ysql = true
#      enable_node_to_node_encrypt = true
#      enable_client_to_node_encrypt = true
#      yb_software_version = "2.7.3.0-b80"
#      access_key_code = local.provider_key
#    }
#  }
#}


