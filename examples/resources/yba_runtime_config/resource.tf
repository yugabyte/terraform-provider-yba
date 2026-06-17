# Allow the S3 telemetry exporter globally. Required before an S3-typed
# telemetry provider can be used in YBA.
resource "yba_runtime_config" "allow_s3" {
  key   = "yb.telemetry.allow_s3"
  value = "true"
}

# Enable per-universe metrics export (required to use the metrics block on
# yba_universe_telemetry_config).
resource "yba_runtime_config" "metrics_export_enabled" {
  key   = "yb.universe.metrics_export_enabled"
  value = "true"
}

# Every value is sent and read back as a plain string regardless of the key's
# YBA data type, so booleans, numbers, durations, and lists are all written as
# strings. YBA validates the string against the key's type and stores it
# verbatim, so the value round-trips with no drift.
resource "yba_runtime_config" "task_gc_interval" {
  key   = "yb.taskGC.gc_check_interval" # a Duration-typed key
  value = "3 hours"
}
