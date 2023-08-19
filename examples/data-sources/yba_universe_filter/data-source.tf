data "yba_universe_filter" "all_universes" {
}

data "yba_universe_filter" "filter_code" {
  code = "<universe-code>"
}

data "yba_universe_filter" "filter_name" {
  name      = "<universe-name-substring>"
  num_nodes = 3
}
