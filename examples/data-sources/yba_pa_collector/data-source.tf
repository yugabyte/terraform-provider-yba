# The embedded Perf Advisor collector. YBA creates and owns it, so it is looked
# up rather than declared; with a single collector configured no filter is
# needed.
data "yba_pa_collector" "embedded" {}

# A specific collector, when more than one is configured.
data "yba_pa_collector" "selected" {
  uuid = var.pa_collector_uuid
}

output "pa_collector_in_use" {
  value = data.yba_pa_collector.embedded.in_use_status
}
