# Custom hooks must be enabled before Terraform can manage hooks. A hook with
# use_sudo = true also needs the enable_sudo key.
resource "yba_runtime_config" "enable_custom_hooks" {
  key   = "yb.security.custom_hooks.enable_custom_hooks"
  value = "true"
}

resource "yba_runtime_config" "enable_sudo_hooks" {
  key   = "yb.security.custom_hooks.enable_sudo"
  value = "true"
}

# A Bash hook that runs before YBA provisions a node of one provider. Hooks
# that fire on the same trigger run in natural sort order of their names, so a
# number prefix sets the order. YBA passes each runtime_args entry to the
# script as a "--KEY VALUE" flag, after its own "--parent_task" and "--trigger"
# flags.
resource "yba_hook" "mount_volume" {
  name           = "10-mount-volume.sh"
  execution_lang = "Bash"
  hook_text      = <<-EOT
    #!/bin/bash
    set -euo pipefail
    while [ $# -gt 0 ]; do
      case "$1" in
        --DEVICE) DEVICE="$2"; shift 2 ;;
        *) shift ;;
      esac
    done
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

# A Python hook, loaded from a file next to the configuration, that runs after
# every universe restart. With no universe_uuid or provider_uuid, it applies to
# every universe.
resource "yba_hook" "collect_diagnostics" {
  name           = "20-collect-diagnostics.py"
  execution_lang = "Python"
  hook_text      = file("${path.module}/hooks/collect_diagnostics.py")
  use_sudo       = false

  trigger_type = "PostRestartUniverse"

  depends_on = [yba_runtime_config.enable_custom_hooks]
}

# ApiTriggered hooks run only when you start them through the YBA API, and
# only while this key is true.
resource "yba_runtime_config" "enable_api_triggered_hooks" {
  key   = "yb.security.custom_hooks.enable_api_triggered_hooks"
  value = "true"
}

# An ApiTriggered hook for one cluster of one universe. cluster_uuid requires
# universe_uuid.
resource "yba_hook" "rotate_credentials" {
  name           = "30-rotate-credentials.sh"
  execution_lang = "Bash"
  hook_text      = file("${path.module}/hooks/rotate_credentials.sh")

  trigger_type  = "ApiTriggered"
  universe_uuid = yba_universe.gcp.id
  cluster_uuid  = yba_universe.gcp.clusters[0].uuid

  depends_on = [
    yba_runtime_config.enable_custom_hooks,
    yba_runtime_config.enable_api_triggered_hooks,
  ]
}
