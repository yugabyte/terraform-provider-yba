---
page_title: "yba_hook Resource - YugabyteDB Anywhere"
description: |-
  YBA Hook Resource. Manages a custom hook — a Bash or Python script that YugabyteDB Anywhere runs on universe nodes when the configured trigger fires (for example node provisioning, a rolling restart, or a software upgrade) — together with where it applies: every universe (the default), one provider, one universe, or one cluster.
  Behind the API, YBA binds hooks to triggers through hook scope objects shared by every hook with the same trigger and target. The resource manages those scopes automatically: it reuses an existing scope or creates one on demand, and deletes a scope when its last hook is removed.
  ~> Note: Custom hooks must be enabled on the YBA instance: set the global runtime config key yb.security.custom_hooks.enable_custom_hooks to true (for example with the yba_runtime_config resource). All custom hook operations require a Super Admin API token (an Admin token when YBA runs in cloud mode).
  ~> Note: All hooks that fire on the same trigger run in natural sort order of their names. Prefix names with a number (10-mount.sh, 20-tune.sh) to control execution order.
  ~> Warning: Deleting a hook scope in YBA cascade-deletes every hook attached to it. This resource only deletes a scope it is about to leave empty, but a hook attached to the same trigger and target outside Terraform at that same moment can be lost to the cascade. Avoid mixing out-of-band hook management with Terraform-managed hooks on the same trigger and target.
---

# yba_hook (Resource)

YBA Hook Resource. Manages a custom hook — a Bash or Python script that YugabyteDB Anywhere runs on universe nodes when the configured trigger fires (for example node provisioning, a rolling restart, or a software upgrade) — together with where it applies: every universe (the default), one provider, one universe, or one cluster.

Behind the API, YBA binds hooks to triggers through hook scope objects shared by every hook with the same trigger and target. The resource manages those scopes automatically: it reuses an existing scope or creates one on demand, and deletes a scope when its last hook is removed.

~> **Note:** Custom hooks must be enabled on the YBA instance: set the global runtime config key `yb.security.custom_hooks.enable_custom_hooks` to `true` (for example with the `yba_runtime_config` resource). All custom hook operations require a Super Admin API token (an Admin token when YBA runs in cloud mode).

~> **Note:** All hooks that fire on the same trigger run in natural sort order of their names. Prefix names with a number (`10-mount.sh`, `20-tune.sh`) to control execution order.

~> **Warning:** Deleting a hook scope in YBA cascade-deletes every hook attached to it. This resource only deletes a scope it is about to leave empty, but a hook attached to the same trigger and target outside Terraform at that same moment can be lost to the cascade. Avoid mixing out-of-band hook management with Terraform-managed hooks on the same trigger and target.

## Example Usage

```terraform
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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `execution_lang` (String) Language the hook is written in. Allowed values: `Bash`, `Python`.
- `hook_text` (String) Full contents of the hook script. Use the Terraform `file()` function to load it from disk.
- `name` (String) Name of the hook, unique per customer. The name also determines execution order: hooks firing on the same trigger run in natural sort order of their names.
- `trigger_type` (String) Trigger the hook runs on. Node lifecycle triggers are `PreNodeProvision` and `PostNodeProvision`; `ApiTriggered` hooks run only when explicitly invoked through the YBA run-hooks API. Upgrade-task triggers follow the pattern `Pre<Task>`/`Post<Task>` (around the whole task) and `Pre<Task>NodeUpgrade`/`Post<Task>NodeUpgrade` (around each node), for example `PreRestartUniverse` or `PostSoftwareUpgradeNodeUpgrade`. The full set depends on the YBA version; YBA rejects unknown values.

### Optional

- `cluster_uuid` (String) UUID of the cluster within `universe_uuid` the hook applies to; requires `universe_uuid`.
- `provider_uuid` (String) UUID of the cloud provider the hook applies to. Cannot be combined with `universe_uuid`; leave both unset to apply the hook to every universe.
- `runtime_args` (Map of String) Optional string arguments exposed to the hook at runtime.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `universe_uuid` (String) UUID of the universe the hook applies to. Cannot be combined with `provider_uuid`; leave both unset to apply the hook to every universe.
- `use_sudo` (Boolean) Run the hook with superuser privileges. Requires the global runtime config key `yb.security.custom_hooks.enable_sudo` to be `true`. False by default.

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `read` (String)
- `update` (String)

## Import

Hooks can be imported using the hook UUID:

```sh
terraform import yba_hook.mount_volume 4f1f66a1-6b7f-4c5e-89f2-1b9f2f31e2c3
```
