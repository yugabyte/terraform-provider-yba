# Read a key from the global scope, which is the default.
data "yba_runtime_config" "allow_s3" {
  key = "yb.telemetry.allow_s3"
}

# The value is a string. Convert it with tobool, tonumber or jsondecode to use
# it as another type.
output "s3_telemetry_allowed" {
  value = tobool(data.yba_runtime_config.allow_s3.value)
}

# Read a key from the scope of one universe.
data "yba_runtime_config" "under_replicated_check" {
  scope = yba_universe.main.id
  key   = "yb.checks.under_replicated_tablets.enabled"
}
