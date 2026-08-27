data "yba_pa_collector" "embedded" {}

# Collected and stored locally, with metrics also remote-written into YBA's
# own Prometheus.
resource "yba_universe_perf_advisor_registration" "advanced" {
  universe_uuid     = yba_universe.example.id
  pa_collector_uuid = data.yba_pa_collector.embedded.uuid
  mode              = "ADVANCED"
}

# Online mode: the local collector scrapes the universe and forwards
# everything to an external Perf Advisor, keeping nothing here. The endpoint is
# pushed to the collector before the universe is registered, so a destination
# YBA cannot reach fails this apply.
resource "yba_universe_perf_advisor_registration" "online" {
  universe_uuid              = yba_universe.example.id
  pa_collector_uuid          = data.yba_pa_collector.embedded.uuid
  mode                       = "ONLINE"
  perf_advisor_endpoint_uuid = yba_perf_advisor_endpoint.byoc.id
}
