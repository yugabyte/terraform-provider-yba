# Allow OTLP-typed telemetry providers globally. Required for OTLP and
# Dynatrace export configurations to be usable in YBA.
resource "yba_runtime_config" "allow_otlp" {
  key   = "yb.telemetry.allow_otlp"
  value = "true"
}

# Enable per-universe metrics export (required to use the metrics block on
# yba_universe_telemetry_config).
resource "yba_runtime_config" "metrics_export_enabled" {
  key   = "yb.universe.metrics_export_enabled"
  value = "true"
}
