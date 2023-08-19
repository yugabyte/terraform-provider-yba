data "yba_provider_filter" "all_providers" {
}

data "yba_provider_filter" "filter_code" {
  code = "<provider-code>"
}

data "yba_provider_filter" "filter_name" {
  name = "<provider-name-substring>"
}

data "yba_provider_filter" "filter_name_and_code" {
  code = "<provider-code>"
  name = "<provider-name-substring>"
}

data "yba_provider_filter" "filter_regions" {
  code   = "<provider-code>"
  region = ["region1", "region2"]
}

data "yba_provider_filter" "filter_zones" {
  code   = "<provider-code>"
  region = ["region1"]
  zones  = ["zone1", "zone2"]
}
