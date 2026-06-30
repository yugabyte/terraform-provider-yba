# Repeat the `exporter` block to fan a single pipeline out to multiple telemetry
# destinations. Each block becomes one entry in the API's `exporters` array, so
# the metrics below are shipped to BOTH Prometheus and Datadog.
resource "yba_universe_telemetry_config" "fanout" {
  universe_uuid = yba_universe.main.id

  metrics {
    exporter {
      exporter_uuid  = yba_telemetry_provider.prometheus.id
      metrics_prefix = "ybdb."
    }
    exporter {
      exporter_uuid  = yba_telemetry_provider.datadog.id
      metrics_prefix = "ddog."
    }
  }
}
