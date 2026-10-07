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

~> **Warning:** Read replica (ASYNC cluster) support is not fully documented in this provider. Configuration options for ASYNC clusters may be incomplete or subject to change. Use read replicas with caution and refer to the YugabyteDB Anywhere UI or API documentation for the full set of supported options.

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
| Restricted entries in `communication_ports` (see [Update Communication Ports](#update-communication-ports)) | YSQL, YCQL, YEDIS, and YB-Controller ports. |

## Overview of Supported Actions

| Action | Trigger | Task name |
|---|---|---|
| [DB Version Upgrade](#db-version-upgrade) | `yb_software_version` changes | Upgrading Software |
| [Finalize Upgrade](#finalize-upgrade) | `db_version_upgrade_options.finalize = true` | Finalizing Upgrade |
| [Rollback Upgrade](#rollback-upgrade) | `db_version_upgrade_options.rollback = true` | Rolling back upgrade |
| [GFlags Upgrade](#gflags-upgrade) | `specific_gflags` changes (or legacy `master_gflags` / `tserver_gflags`) | Upgrading GFlags |
| [TLS Toggle](#tls-toggle) | `enable_node_to_node_encrypt` or `enable_client_to_node_encrypt` changes | Toggling TLS |
| [Systemd Upgrade](#systemd-upgrade) | `use_systemd` changes from `false` to `true` | Upgrading to Systemd |
| [VM Image Upgrade](#vm-image-upgrade) | `image_bundle_uuid` changes | Upgrading VM Image |
| [Resize Nodes](#resize-nodes) | `volume_size` increases with no instance type change | Resizing Node |
| [Edit Cluster Parameters](#edit-cluster-parameters) | Instance type, node count, volume count, volume size decrease, storage type, instance tags, or zone placement changes | Updating Universe |
| [Update Communication Ports](#update-communication-ports) | Mutable fields in `communication_ports` change without cluster changes | Updating Universe |
| [Certificate Rotation](#certificate-rotation) | `root_ca` or `client_root_ca` changes, or a `cert_rotation` trigger changes | Updating Certificates |
| [Encryption at Rest](#encryption-at-rest) | `encryption_at_rest` block added with `enabled = true`, or its `kms_config_uuid`, `universe_key_rotation_trigger` or `enabled` changes | Enabling encryption at rest, Rotating encryption at rest, or Disabling encryption at rest |
| [Delete Read Replica](#delete-read-replica) | ASYNC cluster removed from `clusters` list | Deleting Read Replica |
| [Delete Universe](#delete-universe) | `terraform destroy` | Deleting Universe |

You can batch multiple actions in a single `terraform apply`. When more than one field
changes at the same time, the provider runs the corresponding tasks sequentially, in the
order that [Action Ordering and Sequencing](#action-ordering-and-sequencing) shows.

---

## DB Version Upgrade

**Trigger:** `clusters[*].user_intent.yb_software_version` changes on the PRIMARY cluster.

**Task name:** Upgrading Software

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.upgrade_option` | Restart strategy: `Rolling` (default), `Non-Rolling`, or `Non-Restart`. |
| `node_restart_settings.sleep_after_master_restart_millis` | Milliseconds to pause after each master restart (default 180000). |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Milliseconds to pause after each TServer restart (default 180000). |
| `db_version_upgrade_options.finalize` | When `true`, automatically finalizes the upgrade after the software upgrade completes. |
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

**Task name:** Finalizing Upgrade

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

**Task name:** Rolling back upgrade

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

**Trigger:** changes to `clusters[*].user_intent.specific_gflags`. The deprecated
`master_gflags` / `tserver_gflags` maps also trigger this upgrade but are subject to the
limitations noted below.

**Task name:** Upgrading GFlags

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.upgrade_option` | `Rolling` (default), `Non-Rolling`, or `Non-Restart`. |
| `node_restart_settings.sleep_after_master_restart_millis` | Pause duration after each master restart. |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Pause duration after each TServer restart. |

**Behavior:** Applies new GFlag values to master and TServer processes. The restart strategy
controls whether nodes are restarted one at a time (`Rolling`), all at once (`Non-Rolling`),
or not at all (`Non-Restart` -- config pushed without restart, supported only for certain
flags). `Non-Restart` rejects flag deletions and `gflag_groups` changes.

**Example:**

```terraform
user_intent {
  # ... other fields ...
  specific_gflags {
    per_process {
      master_gflags = {
        "log_min_duration_statement" = "1000"
      }
      tserver_gflags = {
        "ysql_log_min_duration_statement" = "1000"
      }
    }
  }
}
```

`specific_gflags` exposes three knobs the flat maps cannot:

- `gflag_groups` -- atomically apply a YBA-managed bundle (e.g.
  `ENHANCED_POSTGRES_COMPATIBILITY`). Names are case-insensitive in HCL; the provider
  normalizes to the upper-case form YBA stores.
- `per_az` -- override flags for specific availability zones; per-AZ values overlay the
  cluster-level `per_process` map.
- Read Replica divergence -- the ASYNC cluster can carry its own `per_process` and
  `per_az` settings, or set `inherit_from_primary = true` to pick up the Primary's flags
  at runtime. `gflag_groups` is universe-wide: YBA overwrites the Read Replica's
  `gflag_groups` with the Primary's on every apply, so the provider rejects HCL where
  Primary and Read Replica declare different groups. Per-cluster `per_process` /
  `per_az` edits are bundled into one GFlag upgrade that applies the deltas across
  all clusters together.

The deprecated flat path is universe-wide: `master_gflags` / `tserver_gflags` must match
between Primary and Read Replica, and edits to the Read Replica's flat maps alone are
ignored. Setting both the flat maps and `specific_gflags` in the same cluster is rejected
at plan time as mutually exclusive. See the [v1.0.0 upgrade
guide](upgrading-to-v1.0.0#migrating-universe-gflags-to-specific_gflags) for the migration
recipe.

### Removing GFlags or groups <a id="removing-gflags-or-groups"></a>

The fields inside `specific_gflags` are `Optional + Computed`, which means commenting a
block out or omitting a field is interpreted as "use the value YBA has", not "remove it".
To actually clear a setting, declare it explicitly empty in HCL.

| To remove | Write |
|---|---|
| All `master_gflags` from `per_process` | `master_gflags = {}` |
| All `tserver_gflags` from `per_process` | `tserver_gflags = {}` |
| All `gflag_groups` | `gflag_groups = []` |
| Read Replica inheritance | `inherit_from_primary = false` |
| A specific `per_az` override | Drop that `per_az` block from HCL; `per_az` is not Computed at the entry level. |

For a partial removal of individual flag keys, edit the map content (e.g. omit the key
from `master_gflags`); the provider sends the new map to YBA and the missing key is
treated as a deletion. The empty-map / empty-list pattern above is only required when you
want to clear the entire field.

Example -- drop all TServer flags while keeping the Master flags and the
`ENHANCED_POSTGRES_COMPATIBILITY` group active:

```hcl
specific_gflags {
  gflag_groups = ["ENHANCED_POSTGRES_COMPATIBILITY"]
  per_process {
    tserver_gflags = {}
    master_gflags = {
      "timestamp_history_retention_interval_sec" = "14400"
    }
  }
}
```

`terraform plan` will show the TServer map going to `{}`; the resulting GFlag upgrade
clears those flags on every TServer.

---

## TLS Toggle

**Trigger:** `clusters[*].user_intent.enable_node_to_node_encrypt` or
`enable_client_to_node_encrypt` changes on the PRIMARY cluster.

**Task name:** Toggling TLS

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

## Certificate Rotation

**Trigger:** `root_ca` or `client_root_ca` changes to a different certificate UUID, or a
`cert_rotation` trigger (`server_cert_trigger` or `client_cert_trigger`) changes to a new
non-empty value.

**Task name:** Updating Certificates

**Controlling fields:**

| Field | Purpose |
|---|---|
| `root_ca` | Root certificate for node-to-node encryption. A change rotates the universe to the new certificate. |
| `client_root_ca` | Root certificate for client-to-node encryption. A change rotates client-to-node encryption to the new certificate. |
| `cert_rotation.server_cert_trigger` | A change re-issues the node-to-node server certificates from the current `root_ca`, which must be a self-signed certificate. |
| `cert_rotation.client_cert_trigger` | A change re-issues the client-to-node server certificates from the current `client_root_ca`, which must be a self-signed certificate. |
| `node_restart_settings.*` | Restart strategy and sleep times after each node restart. |

**Behavior:** There are two kinds of rotation. The YugabyteDB Anywhere UI calls them
**Rotate root certificate** and **Rotate server certificate**.

- **Root certificate rotation:** the root certificate changes (`root_ca` or `client_root_ca`
  edits). YugabyteDB Anywhere issues new server certificates from the new root certificate.
  For a new node-to-node root certificate with the `Rolling` or `Non-Restart` strategy, the
  task runs in three phases, so that the nodes can keep talking to each other: it adds the
  new root certificate to the trust store of every node, issues the new server
  certificates, and then removes the old root certificate. With `Rolling`, every node
  restarts once in each phase, so three times in all. Update everything that trusts only
  the old root certificate, for example client trust bundles, to the new certificate.
- **Server certificate rotation:** the root certificate does not change. YugabyteDB Anywhere
  issues new server certificates for each node, signed with the key of the self-signed root
  certificate. By default these server certificates expire after 1 year, so this is the
  rotation that you do regularly. It changes nothing that Terraform can read from the
  universe, so a trigger value starts it: the first value set on a trigger starts a
  rotation, each change starts another one, and removing the trigger does nothing. When one
  root certificate serves node-to-node and client-to-node encryption, a change of either
  trigger re-issues the server certificates for both in the same task. Use a
  [`time_rotating`](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/rotating)
  value as the trigger to rotate on a schedule.

When a root certificate change and a trigger change are in the same apply, the provider runs
the root certificate rotation first, waits for it to finish, and then runs the server
certificate rotation. YugabyteDB Anywhere cannot do both in one task. Do not combine them: a
root certificate rotation already issues new server certificates for every node, so the
trigger rotation only adds a second restart of every node. Change a trigger only in an apply
that does not change `root_ca` or `client_root_ca`.

~> **Note:** If the universe's node-to-node certificates have expired, set
`node_restart_settings.upgrade_option = "Non-Rolling"` for the rotation. With expired
certificates, YugabyteDB Anywhere rejects `Non-Restart`, and rejects `Rolling` for every
rotation except one that re-issues only the client-to-node server certificates. YugabyteDB
Anywhere finds expired certificates through the universe's health checks, so this rule
applies after a health check has reported the expiry.

~> **Note:** A `Non-Restart` rotation reloads the certificates without a restart. It has
these requirements: YugabyteDB Anywhere 2025.2.0.0 or later; DB version 2.14.0.0 or later;
the global runtime configuration flag `yb.features.cert_reload.enabled` set to `true` (the
default); node-to-node certificates that have not expired; and a universe that is configured
for certificate reload. YugabyteDB Anywhere configures each universe that it creates. A
universe that an older YugabyteDB Anywhere release created is configured during its first
`Rolling` or `Non-Rolling` certificate rotation, so do one of those before the first
`Non-Restart` rotation. A universe with only client-to-node encryption also needs DB version
2025.2.1.0 or later.

~> **Note:** A rotation of the node-to-node root certificate also changes the certificate
that the other universes of this universe's xCluster configurations must trust. On
YugabyteDB Anywhere 2025.1.0.0 or later, the rotation copies the new root certificate to
the target universes of the configurations where this universe is the source, and to the
source universes of the db-scoped configurations where it is the target, and reloads the
certificates there without a restart. This works only on a universe that meets the flag,
DB version and configuration requirements of a `Non-Restart` rotation in the note above.
On any other universe, and with earlier YugabyteDB Anywhere releases, the replication
stops. Restart the xCluster configuration after the rotation.

**Example -- rotate to a new client-to-node certificate:**

Change the certificate files and the label of the `yba_custom_server_certificate`. With
`create_before_destroy`, Terraform creates the new certificate, rotates the universe to it,
and then deletes the old certificate.

```terraform
resource "yba_custom_server_certificate" "c2n" {
  label              = "prod-c2n-2027"
  root_certificate   = file("org-ca.crt")
  server_certificate = file("server-2027.crt")
  server_key         = file("server-2027.key")

  lifecycle {
    create_before_destroy = true
  }
}

resource "yba_universe" "example" {
  root_ca        = yba_self_signed_certificate.n2n.uuid
  client_root_ca = yba_custom_server_certificate.c2n.uuid
  # ... other fields ...
}
```

**Example -- re-issue the node-to-node server certificates, in a later apply:**

```terraform
resource "yba_universe" "example" {
  root_ca        = yba_self_signed_certificate.n2n.uuid
  client_root_ca = yba_custom_server_certificate.c2n.uuid

  cert_rotation {
    server_cert_trigger = "2027-01" # change to re-issue node-to-node server certificates
  }
  # ... other fields ...
}
```

---

## Encryption at Rest

**Trigger:** the `encryption_at_rest` block is added with `enabled = true`,
`encryption_at_rest.kms_config_uuid` changes on an enabled universe,
`encryption_at_rest.universe_key_rotation_trigger` changes to a new non-empty value, or
`encryption_at_rest.enabled` changes.

**Task name:** Enabling encryption at rest, Rotating encryption at rest (master key or
universe key), or Disabling encryption at rest

**Controlling fields:**

| Field | Purpose |
|---|---|
| `encryption_at_rest.enabled` | Whether the universe encrypts data at rest. Required in the block: `true` enables encryption, `false` disables it in place. |
| `encryption_at_rest.kms_config_uuid` | The encryption-at-rest configuration (`yba_gcp_ear_config`, or one that `yba_ear_config` finds) whose master key wraps the universe keys. A change rotates the master key. It can change only when `enabled` is true. |
| `encryption_at_rest.universe_key_rotation_trigger` | A change to a new non-empty value rotates the universe key under the current master key. |

**Behavior:** YugabyteDB encrypts each data file with its own data key, and wraps the data
keys with a universe key. YugabyteDB Anywhere wraps the universe key with the master key in
the key management service that the configuration names. None of the operations below
restart nodes. Each operation is one YugabyteDB Anywhere task that updates the keys on the
masters.

- **Enable:** YugabyteDB Anywhere generates a universe key, wraps it with the master key of
  the configuration, sends it to every master, and turns encryption on. Data written after
  that is encrypted. Existing files are encrypted when compactions write them again. A
  block set at universe creation encrypts from the first write.
- **Master key rotation:** a different `kms_config_uuid` on an enabled universe. YugabyteDB
  Anywhere unwraps the universe keys with the old configuration and wraps them again with
  the new one. No data is written again and no new universe key is generated. The old
  configuration keeps the universe's key history, and you cannot delete it while the
  universe exists.
- **Universe key rotation:** a changed `universe_key_rotation_trigger`. YugabyteDB Anywhere
  generates a new universe key under the current master key and makes it active. New files
  use it, and the earlier keys stay available for the files that they encrypted.
  YugabyteDB Anywhere does not store the trigger, so its value has no meaning: the first
  value set starts a rotation, each change starts another one, and removing the trigger
  does nothing. Use a
  [`time_rotating`](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/rotating)
  value as the trigger to rotate on a schedule. A trigger change in the apply that enables
  encryption does not run a separate rotation, because the enable already generates a new
  key. The exception is an enable with a different configuration from the one last used:
  that enable keeps the earlier universe key, so the provider runs the universe key rotation
  after it.
- **Disable:** `enabled = false`. YugabyteDB Anywhere turns encryption off on the masters
  and keeps every key, so files encrypted earlier stay readable. You can enable encryption
  again later. With the same configuration, the enable generates a new universe key. With
  another configuration, it turns encryption on again with the earlier universe key, and
  wraps the universe keys with the new master key, as a master key rotation does. After a
  disable, YugabyteDB Anywhere still reports the last configuration in `kms_config_uuid`.

When `kms_config_uuid` and the trigger change in the same apply, the provider runs the
master key rotation first and the universe key rotation after it, as two tasks. A trigger
change in an apply that disables encryption is an error, and so is a `kms_config_uuid`
change while `enabled` is false.

When the block is omitted, Terraform reads it from the universe, as it does for `root_ca`.
So removing the block from the configuration changes nothing, and a universe imported into
Terraform shows its current encryption state. To disable encryption, always set
`enabled = false`.

~> **Note:** A configuration that a universe has used keeps the universe's key history
until the universe is deleted, also after a disable or a master key rotation to another
configuration. YugabyteDB Anywhere does not delete the configuration before then. To retire
a configuration, create its replacement, change `kms_config_uuid` on every universe, and
keep the old resource (or remove it from state) until the universes are deleted. Give the
replacement `depends_on` on the old configuration. A universe that moved to the replacement
no longer depends on the old configuration, so without `depends_on`, `terraform destroy`
deletes the old configuration before the universe, and that delete fails.

**Example -- enable at creation, later rotate the universe key:**

```terraform
resource "yba_gcp_ear_config" "kms" {
  name          = "gcp-kms-prod"
  credentials   = file("kms-service-account.json")
  location_id   = "us-east1"
  key_ring_id   = "yugabyte-ring"
  crypto_key_id = "yugabyte-master-key"
}

resource "yba_universe" "example" {
  encryption_at_rest {
    enabled                       = true
    kms_config_uuid               = yba_gcp_ear_config.kms.uuid
    universe_key_rotation_trigger = "2026-Q3" # change to rotate the universe key
  }
  # ... other fields ...
}
```

**Example -- enable on an existing universe:**

For a universe that Terraform already manages, add the block and change nothing else. Import
a universe that was created in the YugabyteDB Anywhere UI first. After the import, its state
holds the current encryption settings (`enabled = false`, no configuration), and the next
apply adds the block. The configuration can be a `yba_gcp_ear_config` resource or, as here,
a configuration created in the UI and found by name.

```sh
terraform import yba_universe.existing <universe-uuid>
```

```terraform
data "yba_ear_config" "kms" {
  name = "gcp-kms-prod"
}

resource "yba_universe" "existing" {
  encryption_at_rest {
    enabled         = true
    kms_config_uuid = data.yba_ear_config.kms.uuid
  }
  # ... the universe's existing fields, unchanged ...
}
```

The plan shows only the new block. The apply runs one task, Enabling encryption at rest,
with no node restarts. Data written after that is encrypted, and existing files are
encrypted when compactions write them again. A `universe_key_rotation_trigger` added in the
same apply does not run a separate rotation; set it later to rotate the key.

---

## Systemd Upgrade

**Trigger:** `clusters[*].user_intent.use_systemd` changes from `false` to `true` on the
PRIMARY cluster.

**Task name:** Upgrading to Systemd

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

**Task name:** Upgrading VM Image

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

**Task name:** Resizing Node

**Controlling fields:**

| Field | Purpose |
|---|---|
| `node_restart_settings.sleep_after_master_restart_millis` | Pause duration after each master restart. |
| `node_restart_settings.sleep_after_tserver_restart_millis` | Pause duration after each TServer restart. |

**Behavior:** Expands the volume on each node in place without a full move. The restart strategy
is always `Rolling`. This is the most efficient path for volume size increases: nodes are not
reprovisioned and no data migration is required.

**Volume shrink:** Decreasing `volume_size` cannot use the in-place resize path. The shrink is
instead handled as a full move via the cluster edit (see
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

**Task name:** Updating Universe

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

**Task name:** Updating Universe

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
into the cluster edit request and no separate port-only task is dispatched.

---

## Delete Read Replica

**Trigger:** An ASYNC cluster entry is removed from the `clusters` list in the Terraform
configuration.

**Task name:** Deleting Read Replica

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

**Task name:** Deleting Universe

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
   1. **DB Version Upgrade** + optional auto-finalize *(PRIMARY only)*
   2. **GFlags Upgrade** -- only on the legacy flat path, dispatched from the PRIMARY
      iteration with the universe-wide map. ASYNC iteration logs the change and skips.
      Configs on the `specific_gflags` path skip this step and dispatch in step 6 below.
   3. **TLS Toggle** *(PRIMARY only)*
   4. **Systemd Upgrade** *(PRIMARY only)*
   5. **Resize Nodes** (volume grow, same instance type)
   6. **Edit Cluster Parameters**
6. **GFlags Upgrade** -- `specific_gflags` path only. One GFlag upgrade dispatched
   after the per-cluster loop that carries the per-cluster deltas (Primary, Read
   Replica, or both).
7. **VM Image Upgrade** (after cluster edit, if not already run before scale-out)
8. **Update Communication Ports** (if only ports changed with no cluster changes)
9. **Certificate Rotation**: root certificate rotation first (if `root_ca` or
   `client_root_ca` changed and an earlier step did not already apply the change), then
   server certificate rotation (if a `cert_rotation` trigger changed), as two tasks. When
   both run, the second task re-issues the server certificates that the first one already
   issued, with another restart of every node. Do not change a trigger in the same apply as
   `root_ca` or `client_root_ca`.
10. **Encryption at Rest**: enable, master key rotation, or disable first, then universe
    key rotation (if `universe_key_rotation_trigger` changed), as separate tasks. None of
    them restart nodes.

Each task in the sequence completes (or fails fast) before the next is dispatched. A failure
in any step causes `terraform apply` to return an error; partial changes already applied to
the universe are preserved in Terraform state.

---

## Restart Strategy Reference

The `node_restart_settings.upgrade_option` field applies to most upgrade tasks:

| Strategy | Behavior | Applies to |
|---|---|---|
| `Rolling` | Nodes are restarted one at a time; the universe stays available throughout. | DB version, GFlags, Systemd, Rollback, Finalize, Certificate Rotation |
| `Non-Rolling` | All nodes are restarted simultaneously; brief downtime during restart. | DB version, GFlags, Systemd, Certificate Rotation |
| `Non-Restart` | Changes are pushed to running processes without restarting. GFlags: hot-reload flags only. Certificate Rotation: certificate reload on universes that meet the requirements in [Certificate Rotation](#certificate-rotation). | GFlags, Certificate Rotation |

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
