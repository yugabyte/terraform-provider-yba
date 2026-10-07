# Allow the S3 exporter for telemetry providers. The global scope is the
# default.
resource "yba_runtime_config" "allow_s3" {
  key   = "yb.telemetry.allow_s3"
  value = "true"
}

# Turn on metrics export, so that you can create telemetry providers and
# export universe metrics to them.
resource "yba_runtime_config" "metrics_export_enabled" {
  key   = "yb.universe.metrics_export_enabled"
  value = "true"
}

# Every value is a string, whatever the type of the key. Write booleans,
# numbers, durations and lists as strings. YBA checks the value against the
# type of the key and stores it as written.
resource "yba_runtime_config" "task_gc_interval" {
  key   = "yb.taskGC.gc_check_interval" # a duration key
  value = "3 hours"
}

# A key on the scope of one universe: the protocol of the load balancer
# health checks for that universe.
resource "yba_runtime_config" "lb_health_check_protocol" {
  scope = yba_universe.main.id
  key   = "yb.universe.network_load_balancer.custom_health_check_protocol"
  value = "TCP"
}
