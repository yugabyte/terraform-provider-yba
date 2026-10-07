data "yba_pa_collector" "embedded" {}

# ADVANCED mode: YBA stores the data locally and also writes the metrics into
# its own Prometheus.
resource "yba_universe_perf_advisor_registration" "advanced" {
  universe_uuid     = yba_universe.analytics.id
  pa_collector_uuid = data.yba_pa_collector.embedded.uuid
  mode              = "ADVANCED"
}

# ONLINE mode: YBA sends all the data to an external Perf Advisor and keeps no
# copy. The endpoint comes from the yba_perf_advisor_endpoint example. A
# universe has one registration, so each resource names a different universe.
resource "yba_universe_perf_advisor_registration" "online" {
  universe_uuid              = yba_universe.orders.id
  pa_collector_uuid          = data.yba_pa_collector.embedded.uuid
  mode                       = "ONLINE"
  perf_advisor_endpoint_uuid = yba_perf_advisor_endpoint.byoc.id
}
