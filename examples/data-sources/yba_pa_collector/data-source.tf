# The only collector. YBA creates and manages the embedded collector, so
# Terraform looks it up and does not declare it.
data "yba_pa_collector" "embedded" {}

# One collector by UUID, when there is more than one.
data "yba_pa_collector" "selected" {
  uuid = var.pa_collector_uuid
}

output "pa_collector_in_use" {
  value = data.yba_pa_collector.embedded.in_use_status
}
