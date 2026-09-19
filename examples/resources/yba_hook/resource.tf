# Custom hooks must be enabled on the YBA instance before hooks can be
# managed; use_sudo additionally requires the enable_sudo flag.
resource "yba_runtime_config" "enable_custom_hooks" {
  key   = "yb.security.custom_hooks.enable_custom_hooks"
  value = "true"
}

resource "yba_runtime_config" "enable_sudo_hooks" {
  key   = "yb.security.custom_hooks.enable_sudo"
  value = "true"
}

# A Bash hook that runs on every node provision, on nodes backed by one cloud
# provider. Hooks that fire on the same trigger run in natural sort order of
# their names, so a numeric prefix pins the execution order. runtime_args are
# exposed to the script when it runs.
resource "yba_hook" "mount_volume" {
  name           = "10-mount-volume.sh"
  execution_lang = "Bash"
  hook_text      = <<-EOT
    #!/bin/bash
    set -euo pipefail
    mount "$DEVICE" /data
  EOT
  use_sudo       = true
  runtime_args = {
    DEVICE = "/dev/sdb"
  }

  trigger_type  = "PreNodeProvision"
  provider_uuid = yba_azure_provider.azure.id

  depends_on = [
    yba_runtime_config.enable_custom_hooks,
    yba_runtime_config.enable_sudo_hooks,
  ]
}

# A Python hook loaded from a file next to the Terraform configuration,
# applying to every universe (no universe_uuid or provider_uuid): it runs
# after every rolling restart, anywhere.
resource "yba_hook" "collect_diagnostics" {
  name           = "20-collect-diagnostics.py"
  execution_lang = "Python"
  hook_text      = file("${path.module}/hooks/collect_diagnostics.py")
  use_sudo       = false

  trigger_type = "PostRestartUniverse"

  depends_on = [yba_runtime_config.enable_custom_hooks]
}

# A hook bound to a single cluster of one universe; cluster_uuid requires
# universe_uuid. ApiTriggered hooks only run when explicitly invoked through
# the YBA run-hooks API.
resource "yba_hook" "rotate_credentials" {
  name           = "30-rotate-credentials.sh"
  execution_lang = "Bash"
  hook_text      = file("${path.module}/hooks/rotate_credentials.sh")

  trigger_type  = "ApiTriggered"
  universe_uuid = yba_universe.gcp.id
  cluster_uuid  = "5f8aa2a2-3bce-45c7-9cf2-31ba14b7ab61"

  depends_on = [yba_runtime_config.enable_custom_hooks]
}
