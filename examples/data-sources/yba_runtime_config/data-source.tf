# Read a runtime config key from the global scope. The value is always a string.
data "yba_runtime_config" "allow_s3" {
  key = "yb.telemetry.allow_s3"
}

# Because value is a string, convert it to the type you need with the matching
# Terraform function: tobool(...) for booleans, tonumber(...) for numbers, or
# jsondecode(...) for lists/objects.
output "s3_telemetry_allowed" {
  value = tobool(data.yba_runtime_config.allow_s3.value)
}

# Read a key from a non-global scope by passing its scope UUID.
data "yba_runtime_config" "universe_metrics" {
  scope = "00000000-0000-0000-0000-000000000000"
  key   = "yb.universe.metrics_export_enabled"
}
