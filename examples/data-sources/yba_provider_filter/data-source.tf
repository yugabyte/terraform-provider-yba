data "yba_provider_filter" "all_providers" {
}

data "yba_provider_filter" "filter_code" {
  codes = ["<provider-code>"]
}

data "yba_provider_filter" "filter_name" {
  name = "<provider-name-substring>"
}

data "yba_provider_filter" "filter_name_and_code" {
  codes = ["<provider-code>"]
  name  = "<provider-name-substring>"
}

data "yba_provider_filter" "filter_regions" {
  codes   = ["<provider-code>"]
  regions = ["<region1>", "<region2>"]
}

data "yba_provider_filter" "filter_zones" {
  codes   = ["<provider-code>"]
  regions = ["<region1>"]
  zones   = ["<zone1>", "<zone2>"]
}
