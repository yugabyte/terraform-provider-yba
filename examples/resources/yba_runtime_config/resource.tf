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
