---
subcategory: ""
page_title: "Universe Edit Actions - YugabyteDB Anywhere Terraform Provider"
description: |-
  Reference guide for all edit operations supported on the yba_universe resource.
---

# Universe Edit Actions

The `yba_universe` resource supports a range of in-place edit operations that are triggered
automatically when you change specific fields and run `terraform apply`. Each operation maps
to a distinct YBA API task that runs asynchronously on the YugabyteDB Anywhere platform.

This guide describes every supported action: what triggers it, which fields control its
behavior, and any ordering or constraint rules that apply.

## Fields that cannot be changed via Terraform after creation

The provider rejects edits to the following `yba_universe` fields at plan time. Changing any of
them on an existing universe results in a `cannot be changed after universe creation` error and
no task is dispatched. To change one of these, destroy and recreate the universe.

| Field | Notes |
|---|---|
| `clusters[*].user_intent.universe_name` | |
| `clusters[*].user_intent.provider` | Cross-cloud or cross-provider moves are not supported. |
| `clusters[*].user_intent.access_key_code` | Key rotation is not supported. |
| `clusters[*].user_intent.enable_ysql`, `enable_ycql`, `enable_yedis` | |
| `clusters[*].user_intent.enable_ysql_auth`, `enable_ycql_auth` | |
| `clusters[*].user_intent.ysql_password`, `ycql_password` | Password rotation is not supported. |
| `clusters[*].user_intent.assign_public_ip`, `assign_static_ip`, `enable_ipv6` | |
| `clusters[*].user_intent.use_host_name`, `use_time_sync` | |
| `clusters[*].user_intent.aws_arn_string` | |
| `root_ca`, `client_root_ca` | Rotation is not supported. |
| Restricted entries in `communication_ports` (see [Update Communication Ports](#update-communication-ports)) | YSQL, YCQL, YEDIS, and YB-Controller ports. |

## Overview of Supported Actions

| Action | Trigger | YBA Task |
|---|---|---|
| [DB Version Upgrade](#db-version-upgrade) | `yb_software_version` changes | `UpgradeDBVersion` |
| [Finalize Upgrade](#finalize-upgrade) | `db_version_upgrade_options.finalize = true` | `FinalizeUpgrade` |
| [Rollback Upgrade](#rollback-upgrade) | `db_version_upgrade_options.rollback = true` | `RollbackUpgrade` |
| [GFlags Upgrade](#gflags-upgrade) | `master_gflags` or `tserver_gflags` changes | `UpgradeGFlags` |
| [TLS Toggle](#tls-toggle) | `enable_node_to_node_encrypt` or `enable_client_to_node_encrypt` changes | `UpgradeTls` |
| [Systemd Upgrade](#systemd-upgrade) | `use_systemd` changes from `false` to `true` | `UpgradeSystemd` |
| [VM Image Upgrade](#vm-image-upgrade) | `image_bundle_uuid` changes | `UpgradeVMImage` |
| [Resize Nodes](#resize-nodes) | `volume_size` increases with no instance type change | `ResizeNode` |
| [Edit Cluster Parameters](#edit-cluster-parameters) | Instance type, node count, volume count, volume size decrease, storage type, instance tags, or zone placement changes | `UpdatePrimaryCluster` / `UpdateReadOnlyCluster` |
| [Update Communication Ports](#update-communication-ports) | Mutable fields in `communication_ports` change without cluster changes | `UpdatePrimaryCluster` |
| [Delete Read Replica](#delete-read-replica) | ASYNC cluster removed from `clusters` list | `DeleteReadonlyCluster` |
| [Delete Universe](#delete-universe) | `terraform destroy` | `DestroyUniverse` |

You can batch multiple actions in a single `terraform apply`. When more than one field
changes at the same time, the provider runs the corresponding tasks sequentially in the
order shown above.

---

## DB Version Upgrade

**Trigger:** `clusters[*].user_intent.yb_software_version` changes on the PRIMARY cluster.

**YBA task:** `UpgradeDBVersion`

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.upgrade_option` | Restart strategy: `Rolling` (default), `Non-Rolling`, or `Non-Restart`. |
| `node_restart_settings.sleep_after_master_restart_millis` | Milliseconds to pause after each master restart (default 180000). |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Milliseconds to pause after each TServer restart (default 180000). |
| `db_version_upgrade_options.finalize` | When `true`, automatically calls FinalizeUpgrade after the upgrade task completes. |
| `db_version_upgrade_state` (read-only) | Current upgrade state reported by YBA. |

**Behavior:** The upgrade task rolls out the new software version across nodes according to the
chosen restart strategy. Once the task completes, the universe enters `PreFinalize` state by
default, which pauses the upgrade so you can verify the new version before committing. Set
`finalize = true` to commit immediately after the upgrade completes, or leave it `false` to
pause and act separately.

Software upgrade changes on ASYNC (read replica) clusters are ignored -- the upgrade is
applied globally to all clusters through the PRIMARY cluster change.

**Example -- pause at PreFinalize for a monitoring window:**

```terraform
resource "yba_universe" "example" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      yb_software_version = "2.20.2.0-b10"
      # ... other fields ...
    }
  }

  db_version_upgrade_options {
    finalize = false
    rollback = false
  }

  node_restart_settings {
    upgrade_option                   = "Rolling"
    sleep_after_master_restart_millis  = 60000
    sleep_after_tserver_restart_millis = 60000
  }
}
```

---

## Finalize Upgrade

**Trigger:** `db_version_upgrade_options.finalize` flips from `false` to `true` while the
universe is in `PreFinalize` state.

**YBA task:** `FinalizeUpgrade`

**Controlling fields:** Same `node_restart_settings` as the upgrade itself.

**Behavior:** Commits the pending DB version upgrade. After a successful finalize, the universe
returns to `Ready` state. Reset `finalize = false` (and leave `rollback = false`) in your
configuration once done to reach a stable Terraform steady state.

**Example -- commit after a monitoring window:**

```terraform
db_version_upgrade_options {
  finalize = true
  rollback = false
}
```

**State machine for DB version upgrades:**

| `db_version_upgrade_state` | Recommended next action |
|---|---|
| `Ready` | Change `yb_software_version` to start an upgrade. |
| `Upgrading` | Wait -- upgrade task is running. |
| `UpgradeFailed` | Investigate via the YBA UI; retry or rollback. |
| `PreFinalize` | Set `finalize = true` to commit, or `rollback = true` to revert. |
| `Finalizing` | Wait -- finalize task is running. |
| `FinalizeFailed` | Retry by applying with `finalize = true` again. |
| `RollingBack` | Wait -- rollback task is running. |
| `RollbackFailed` | Retry by applying with `rollback = true` again. |

---

## Rollback Upgrade

**Trigger:** `db_version_upgrade_options.rollback` is set to `true` while the universe is in
`PreFinalize` state.

**YBA task:** `RollbackUpgrade`

**Controlling fields:** Same `node_restart_settings` as the upgrade itself.

**Behavior:** Reverts the universe to the previous DB version. The provider automatically resets
`rollback` to `false` in Terraform state after a successful rollback so that the next plan
shows a diff, reminding you to update your configuration. After rollback completes, set
`rollback = false` and restore `yb_software_version` to the previous version in your config
to reach a stable steady state.

Rollback runs before any cluster-level edits in the same apply, so you can combine a rollback
with other cluster parameter changes in a single `terraform apply`.

**Example -- revert a pending upgrade:**

```terraform
db_version_upgrade_options {
  finalize = false
  rollback = true
}
```

---

## GFlags Upgrade

**Trigger:** `clusters[*].user_intent.master_gflags` or `tserver_gflags` changes on the
PRIMARY cluster.

**YBA task:** `UpgradeGFlags`

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.upgrade_option` | `Rolling` (default), `Non-Rolling`, or `Non-Restart`. |
| `node_restart_settings.sleep_after_master_restart_millis` | Pause duration after each master restart. |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Pause duration after each TServer restart. |

**Behavior:** Applies new GFlag values to master and TServer processes. The restart strategy
controls whether nodes are restarted one at a time (`Rolling`), all at once (`Non-Rolling`),
or not at all (`Non-Restart` -- config pushed without restart, supported only for certain
flags).

GFlag changes on ASYNC clusters are ignored -- GFlags are applied universe-wide through the
PRIMARY cluster change.

**Example:**

```terraform
user_intent {
  master_gflags = {
    "log_min_duration_statement" = "1000"
  }
  tserver_gflags = {
    "ysql_log_min_duration_statement" = "1000"
  }
  # ... other fields ...
}
```

---

## TLS Toggle

**Trigger:** `clusters[*].user_intent.enable_node_to_node_encrypt` or
`enable_client_to_node_encrypt` changes on the PRIMARY cluster.

**YBA task:** `UpgradeTls`

**Controlling fields:**

| Field | Purpose |
|---|---|
| `root_ca` | Root CA UUID for node-to-node TLS. Reused on re-enable if previously auto-generated. |
| `client_root_ca` | Separate root CA for client-to-node TLS. When different from `root_ca`, separate certificates are used. |
| `node_restart_settings.sleep_after_master_restart_millis` | Pause duration after each master restart. |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Pause duration after each TServer restart. |

**Behavior:** Enables or disables encryption-in-transit between nodes or between clients and
nodes. The upgrade strategy is always `Non-Rolling` regardless of `node_restart_settings.upgrade_option`.
All nodes are restarted simultaneously.

When re-enabling TLS after it was previously disabled, the provider reuses the certificate
UUIDs that were active at the time TLS was disabled (stored in Terraform state), so YBA does
not generate new certificates.

TLS toggle changes on ASYNC clusters are silently ignored -- the toggle applies to all
clusters through the PRIMARY cluster change.

**Example -- enable TLS on an existing universe:**

```terraform
resource "yba_universe" "example" {
  root_ca = "<root-ca-uuid>"

  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      # ... other fields ...
    }
  }
}
```

---

## Systemd Upgrade

**Trigger:** `clusters[*].user_intent.use_systemd` changes from `false` to `true` on the
PRIMARY cluster.

**YBA task:** `UpgradeSystemd`

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.upgrade_option` | `Rolling` (default), `Non-Rolling`, or `Non-Restart`. |
| `node_restart_settings.sleep_after_master_restart_millis` | Pause duration after each master restart. |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Pause duration after each TServer restart. |

**Behavior:** Migrates node process management from cron-based to systemd. This is a one-way
operation -- the provider rejects `use_systemd = false` on an existing universe that already
uses systemd at apply time.

Systemd changes on ASYNC clusters are ignored -- the upgrade applies universe-wide through
the PRIMARY cluster change.

---

## VM Image Upgrade

**Trigger:** `clusters[*].user_intent.image_bundle_uuid` changes on any cluster.

**YBA task:** `UpgradeVMImage`

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.sleep_after_master_restart_millis` | Pause duration after each master restart. |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Pause duration after each TServer restart. |

**Behavior:** Replaces the OS image on all cluster nodes. The restart strategy is always
`Rolling` regardless of `node_restart_settings.upgrade_option`. Nodes are upgraded one at a
time to maintain availability.

**Ordering with scale-out:** When `num_nodes` increases at the same time as `image_bundle_uuid`
changes, the VM image upgrade runs *before* the scale-out so that newly provisioned nodes
start with the new image directly. In all other cases (scale-in or no scale change), the VM
image upgrade runs *after* the cluster edit so that nodes being removed are not upgraded
unnecessarily.

**Example -- upgrade the OS image bundle:**

```terraform
user_intent {
  image_bundle_uuid = data.yba_provider_image_bundles.al2023.bundles[0].uuid
  # ... other fields ...
}
```

---

## Resize Nodes

**Trigger:** `clusters[*].user_intent.device_info.volume_size` increases while the instance
type remains unchanged.

**YBA task:** `ResizeNode`

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.sleep_after_master_restart_millis` | Pause duration after each master restart. |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Pause duration after each TServer restart. |

**Behavior:** Expands the volume on each node in place without a full move. The restart strategy
is always `Rolling`. This is the most efficient path for volume size increases: nodes are not
reprovisioned and no data migration is required.

**Volume shrink:** Decreasing `volume_size` cannot use the in-place resize path. The shrink is
instead handled by `UpdatePrimaryCluster` / `UpdateReadOnlyCluster` as a full move (see
[Edit Cluster Parameters](#edit-cluster-parameters)). You must set `full_move { allow = true }`
to authorize a volume shrink.

**Applies to both PRIMARY and ASYNC clusters** independently: each cluster dispatches its own
`ResizeNode` task when its volume size grows while its instance type stays the same.

---

## Edit Cluster Parameters

**Trigger:** One or more of the following fields change on a PRIMARY or ASYNC cluster:

| Field | When it triggers this path |
|---|---|
| `instance_type` | Any change. May also carry a volume size or count change. |
| `num_nodes` | Scale-out or scale-in. |
| `device_info.num_volumes` | When `instance_type` also changes, or when `full_move { allow = true }`. |
| `device_info.volume_size` | Decrease only. Increase with the same instance type uses ResizeNode. |
| `device_info.storage_type` | Requires `full_move { allow = true }`. |
| `instance_tags` | Any change. |
| `cloud_list` | Per-zone placement changes. |

**YBA task:** `UpdatePrimaryCluster` (PRIMARY) or `UpdateReadOnlyCluster` (ASYNC)

**Controlling field:**

| Field | Purpose |
|---|---|
| `full_move.allow` | Must be `true` to authorize operations that trigger a full move: `volume_size` decrease (any instance type), `num_volumes` change with the same instance type, or `storage_type` change. |
| `full_move.force` | Optional. When `true`, route the edit through a full move even when YBA reports that smart resize (in-place rolling update) is also available -- typically used to force a full rebuild on an `instance_type` change. Requires `allow = true`. Has no effect when YBA does not return `FULL_MOVE` as an option for the planned edit. |

**Full move operations:** Certain changes require provisioning new nodes with the new
configuration, migrating all data from the old nodes, then decommissioning the old nodes. This
temporarily requires 2x node capacity and is significantly slower than in-place operations:

| Change | Full move required? |
|---|---|
| `instance_type` change (with or without volume changes) | No -- YBA defaults to smart resize. Set `full_move { allow = true, force = true }` to force a full rebuild instead. |
| `volume_size` increase with instance type change | No -- bundled into the cluster edit. Set `full_move { allow = true, force = true }` to force a full rebuild instead. |
| `volume_size` decrease (any instance type) | Yes -- set `full_move { allow = true }`. |
| `num_volumes` change with same instance type | Yes -- set `full_move { allow = true }`. |
| `num_volumes` change with instance type change | No -- bundled into the cluster edit. Set `full_move { allow = true, force = true }` to force a full rebuild instead. |
| `storage_type` change | Yes -- set `full_move { allow = true }`. |
| `num_nodes` change | No -- nodes are added or removed. |
| `instance_tags` change | No -- applied without restart. |
| `cloud_list` zone placement change | No -- nodes removed from old zones, added to new zones. |

**Zone placement:** Changing `cloud_list` updates zone placement. The provider resolves AZ
UUIDs by name from the live API state before computing the diff to avoid index-shifting
confusion with Terraform's positional list comparison. Moving all nodes out of a zone and
into a new zone is supported.

~> **Note:** Moving data to a different availability zone within the same region is supported
through `cloud_list` changes. Moving to a different *region* requires adding the new region to
the provider configuration first.

**Example -- scale out from 3 to 6 nodes:**

```terraform
user_intent {
  num_nodes = 6
  # ... other fields ...
}
```

**Example -- change instance type (triggers EditUniverse / full cluster reconfigure):**

```terraform
user_intent {
  instance_type = "c5.2xlarge"
  # ... other fields ...
}
```

**Example -- shrink volume size (requires `full_move { allow = true }`):**

```terraform
resource "yba_universe" "example" {
  full_move {
    allow = true
  }

  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      device_info {
        volume_size = 100   # decreased from 250
        # ... other fields ...
      }
    }
  }
}
```

**Example -- force a full move during an instance type change:**

By default, YBA uses smart resize (an in-place rolling update) for an instance type change.
Set `full_move { allow = true, force = true }` to route the edit through a full move
instead -- new nodes are provisioned with the new instance type, data is migrated, and the
old nodes are decommissioned. Revert to `force = false` after the targeted edit so
subsequent eligible edits do not silently route through a full move.

```terraform
resource "yba_universe" "example" {
  full_move {
    allow = true
    force = true
  }

  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      instance_type = "c5.2xlarge"   # changed from c5.large
      # ... other fields ...
    }
  }
}
```

---

## Update Communication Ports

**Trigger:** One or more mutable fields in the `communication_ports` block change without any
cluster changes in the same apply.

**YBA task:** `UpdatePrimaryCluster` (via `UniverseConfigureTaskParams` carrying the new ports)

**Mutable ports:**

| Port | Default |
|---|---|
| `master_http_port` | 7000 |
| `master_rpc_port` | 7100 |
| `tserver_http_port` | 9000 |
| `tserver_rpc_port` | 9100 |
| `node_exporter_port` | 9300 |

**Immutable ports (cannot be changed after universe creation):**

| Port |
|---|
| `yql_server_http_port` |
| `yql_server_rpc_port` |
| `ysql_server_http_port` |
| `ysql_server_rpc_port` |
| `redis_server_http_port` |
| `redis_server_rpc_port` |
| `yb_controller_rpc_port` |

The provider validates that immutable ports have not changed at plan time and returns an error
before dispatching any task if they have.

When cluster changes and port changes occur in the same apply, the updated ports are bundled
into the `UpdatePrimaryCluster` request for the cluster edit and no separate port-only task
is dispatched.

---

## Delete Read Replica

**Trigger:** An ASYNC cluster entry is removed from the `clusters` list in the Terraform
configuration.

**YBA task:** `DeleteReadonlyCluster`

**Controlling field:**

| Field | Purpose |
|---|---|
| `delete_options.force_delete` | When `true`, forces deletion even if the cluster has errors. |

**Behavior:** Removes the read replica cluster from the universe. The PRIMARY cluster is
unaffected. Adding a read replica after the universe has been created is not currently
supported -- read replicas may only be specified at universe creation time.

---

## Delete Universe

**Trigger:** `terraform destroy` or removing the `yba_universe` resource block from
configuration.

**YBA task:** `DestroyUniverse`

**Controlling fields:**

| Field | Default | Purpose |
|---|---|---|
| `delete_options.delete_backups` | `false` | Also delete all YBA-managed backups for this universe. |
| `delete_options.delete_certs` | `false` | Also delete the TLS certificates associated with this universe. |
| `delete_options.force_delete` | `false` | Force deletion even when the universe has errors or a stuck task. |

**Behavior:** Decommissions all nodes, removes the universe record from YBA, and optionally
cleans up associated resources (backups, certificates). When the universe is stuck after a
failed create or update task, the provider automatically escalates to force-delete so it can
clean up the resource.

**Example -- delete and clean up backups:**

```terraform
resource "yba_universe" "example" {
  delete_options {
    delete_backups = true
    delete_certs   = false
    force_delete   = false
  }
  # ... other fields ...
}
```

---

## Action Ordering and Sequencing

When multiple fields change in a single apply, the provider dispatches tasks in the following
fixed order:

1. **Rollback** (if `rollback = true` and universe is `PreFinalize`)
2. **Explicit finalize** (if `finalize` flips to `true` and universe is already `PreFinalize`)
3. **Delete Read Replica** (if ASYNC cluster removed)
4. **VM Image Upgrade** (before scale-out, if `image_bundle_uuid` and `num_nodes` both change)
5. Per-cluster loop (PRIMARY first, then ASYNC):
   1. **DB Version Upgrade** + optional auto-finalize
   2. **GFlags Upgrade**
   3. **TLS Toggle**
   4. **Systemd Upgrade**
   5. **Resize Nodes** (volume grow, same instance type)
   6. **Edit Cluster Parameters** (`UpdatePrimaryCluster` / `UpdateReadOnlyCluster`)
6. **VM Image Upgrade** (after cluster edit, if not already run before scale-out)
7. **Update Communication Ports** (if only ports changed with no cluster changes)

Each task in the sequence completes (or fails fast) before the next is dispatched. A failure
in any step causes `terraform apply` to return an error; partial changes already applied to
the universe are preserved in Terraform state.

---

## Restart Strategy Reference

The `node_restart_settings.upgrade_option` field applies to most upgrade tasks:

| Strategy | Behavior | Applies to |
|---|---|---|
| `Rolling` | Nodes are restarted one at a time; the universe stays available throughout. | DB version, GFlags, Systemd, Rollback, Finalize |
| `Non-Rolling` | All nodes are restarted simultaneously; brief downtime during restart. | DB version, GFlags, Systemd |
| `Non-Restart` | GFlag values are pushed to running processes without restarting. Only compatible with hot-reload flags. | GFlags only |

**Fixed strategies (not affected by `upgrade_option`):**

| Task | Fixed strategy |
|---|---|
| TLS Toggle | `Non-Rolling` always |
| Resize Nodes | `Rolling` always |
| VM Image Upgrade | `Rolling` always |

---

## Related Resources

- [`yba_universe` resource reference](../resources/universe)
- [YugabyteDB Anywhere: Manage universe deployments](https://docs.yugabyte.com/stable/yugabyte-platform/manage-deployments/)
- [YugabyteDB Anywhere: Upgrade universes](https://docs.yugabyte.com/stable/yugabyte-platform/manage-deployments/upgrade-software/)
