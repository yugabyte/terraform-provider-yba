---
page_title: "yba_hook Resource - YugabyteDB Anywhere"
description: |-
  Manages a custom hook in YugabyteDB Anywhere: a Bash or Python script that YBA runs on universe nodes when a trigger fires, such as node provisioning, a rolling restart or a software upgrade. A hook applies to every universe, or to one provider, one universe or one cluster.
---

# yba_hook (Resource)

Manages a custom hook in YugabyteDB Anywhere: a Bash or Python script that YBA runs on universe nodes when a trigger fires, such as node provisioning, a rolling restart or a software upgrade. A hook applies to every universe, or to one provider, one universe or one cluster.

YBA binds each hook to its trigger and target through a hook scope, which all hooks with the same trigger and target share. The resource creates the hook scope when it does not exist, and deletes the hook scope when its last hook is removed.

~> **Note:** Custom hooks must be enabled. Set the global runtime config key `yb.security.custom_hooks.enable_custom_hooks` to `true`, for example with the `yba_runtime_config` resource. Every custom hook operation requires the API token of a Super Admin user.

~> **Note:** YBA runs hooks only on VM universes (cloud and on-premises providers). Kubernetes universe tasks run no hooks.

~> **Note:** Hooks that fire on the same trigger run in natural sort order of their names. Prefix the names with a number (`10-mount.sh`, `20-tune.sh`) to set the order.

~> **Warning:** When YBA deletes a hook scope, it also deletes every hook attached to it. This resource deletes a hook scope only after it reads the scope again and finds no hooks. Another client, such as a script that calls the YBA API or a second Terraform state, can attach a hook between that read and the delete, and YBA then deletes that hook too. Keep all hooks for one trigger and target in one Terraform state, and do not manage hooks for that trigger and target outside Terraform.

## Triggers

Set `trigger_type` to one of these values:

| Trigger | When the hook runs |
| --- | --- |
| `PreNodeProvision`, `PostNodeProvision` | Before and after YugabyteDB Anywhere provisions a node. |
| `Pre<Task>`, `Post<Task>` | Once before and once after the whole task. |
| `Pre<Task>NodeUpgrade`, `Post<Task>NodeUpgrade` | Before and after the task acts on each node. |
| `ApiTriggered` | Only when you start it through the YugabyteDB Anywhere API. |

`<Task>` is one of `RestartUniverse`, `SoftwareUpgrade`, `RebootUniverse`,
`ThirdpartySoftwareUpgrade` or `ConfigureDBApis`. For example,
`PreRestartUniverse` runs before a universe restart, and
`PostSoftwareUpgradeNodeUpgrade` runs after the software upgrade of each node.

The `ConfigureDBApis` triggers require YugabyteDB Anywhere 2025.2.0.0 or later.
To run `ApiTriggered` hooks, set the global runtime config key
`yb.security.custom_hooks.enable_api_triggered_hooks` to `true`.

## Example Usage

```terraform
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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `execution_lang` (String) Language of the hook script. Allowed values: `Bash`, `Python`.
- `hook_text` (String) Contents of the hook script. Use the Terraform `file()` function to load it from a file.
- `name` (String) Name of the hook. It must be unique for the customer and at most 100 characters. Hooks that fire on the same trigger run in natural sort order of their names.
- `trigger_type` (String) Trigger that runs the hook, for example `PreNodeProvision` or `PostRestartUniverse`. The Triggers section lists every value. YBA rejects unknown values.

### Optional

- `cluster_uuid` (String) UUID of the cluster in `universe_uuid` that the hook applies to. Requires `universe_uuid`.
- `provider_uuid` (String) UUID of the provider that the hook applies to. The hook runs on the universe nodes of that provider. Conflicts with `universe_uuid`. Leave both unset to apply the hook to every universe.
- `runtime_args` (Map of String) String arguments for the hook. YBA passes each entry to the script as a `--KEY VALUE` command-line flag, after its own `--parent_task <task>` and `--trigger <trigger>` flags.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `universe_uuid` (String) UUID of the universe that the hook applies to. Conflicts with `provider_uuid`. Leave both unset to apply the hook to every universe.
- `use_sudo` (Boolean) Run the hook with superuser privileges. Requires the global runtime config key `yb.security.custom_hooks.enable_sudo` set to `true`. YBA skips the hook when the key is `false` at the time the trigger fires. Defaults to `false`.

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
terraform import yba_hook.mount_volume <hook-uuid>
```
