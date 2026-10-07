---
page_title: "yba_gcp_ear_config Resource - YugabyteDB Anywhere"
description: |-
  Manages a YugabyteDB Anywhere encryption-at-rest configuration that uses a Google Cloud KMS crypto key as the master key. YugabyteDB Anywhere uses the crypto key to wrap and unwrap the universe keys of each universe that uses the configuration.
---

# yba_gcp_ear_config (Resource)

Manages a YugabyteDB Anywhere encryption-at-rest configuration that uses a Google Cloud KMS crypto key as the master key. YugabyteDB Anywhere uses the crypto key to wrap and unwrap the universe keys of each universe that uses the configuration.

Point the configuration at an existing key ring and crypto key, or let YugabyteDB Anywhere create them. To create them, the identity needs `cloudkms.keyRings.create` and `cloudkms.cryptoKeys.create`. An existing crypto key must have the purpose `ENCRYPT_DECRYPT`, manual rotation (no rotation period), and an enabled primary version.

There are two authentication modes. Set `credentials` to a service-account key, or set `use_gcp_iam = true` to authenticate as the YugabyteDB Anywhere host: the attached service account on Compute Engine, workload identity on GKE, or the key file at `GOOGLE_APPLICATION_CREDENTIALS`. Either identity needs these permissions on the key ring's project: `cloudkms.keyRings.get`, `cloudkms.cryptoKeys.get`, `cloudkms.cryptoKeyVersions.useToEncrypt`, `cloudkms.cryptoKeyVersions.useToDecrypt` and `cloudkms.locations.generateRandomBytes`. YugabyteDB Anywhere checks them when it creates the configuration.

~> **Note:** `use_gcp_iam` and `project_id` need a YugabyteDB Anywhere release later than 2026.1. YugabyteDB Anywhere 2026.1 and earlier reject a configuration without `credentials`, and store `project_id` but do not use it.

~> **Security Note:** `credentials` is stored in the Terraform state file and marked sensitive. Use a secure state backend and restrict access to state files, or use `use_gcp_iam`, which keeps the key out of Terraform.

~> **Note:** Only the credential arguments can change in place. A change to any other argument forces replacement. YugabyteDB Anywhere does not delete a configuration that a universe has used: it keeps the universe's key history, also after encryption is disabled or the universe moves to another configuration, until the universe is deleted. A destroy of such a configuration fails, and the error names the universes. To move universes to a new configuration, create it, change `encryption_at_rest.kms_config_uuid` on each universe, and keep the old configuration (or remove it from state) until its universes are deleted. Give the new configuration `depends_on` on the old one, so that `terraform destroy` deletes the universe before the old configuration.

~> **Drift Note:** Terraform refreshes `in_use` and the non-secret settings. YugabyteDB Anywhere masks credentials, so a credential changed outside Terraform, for example in the YugabyteDB Anywhere UI, does not show as drift. An apply without a change to the credential in the configuration does not send it again.

~> **Import Note:** Import checks the KMS provider: importing a configuration that is not a GCP KMS configuration fails, and the error names its KMS provider. YugabyteDB Anywhere never returns credentials, so they are empty after import, and the first apply sends them again.

Use the configuration in a universe through its `encryption_at_rest` block. The
[Encryption at Rest](../guides/universe-edit-actions.md#encryption-at-rest) section of the
Universe Edit Actions guide describes how to enable encryption, rotate the master key or the
universe key, and disable encryption. For the YugabyteDB Anywhere side, see
[Enable encryption at rest](https://docs.yugabyte.com/stable/yugabyte-platform/security/enable-encryption-at-rest/).

## Example Usage

```terraform
# Encryption-at-rest configuration on an existing Cloud KMS crypto key, with a
# service-account key for authentication. The project_id in the key file names
# the key ring's project.
resource "yba_gcp_ear_config" "service_account" {
  name          = "gcp-kms-prod"
  credentials   = file("${path.module}/kms-service-account.json")
  location_id   = "us-east1"
  key_ring_id   = "yugabyte-ring"
  crypto_key_id = "yugabyte-master-key"
}

# The same kind of key ring, reached with the identity of the YugabyteDB
# Anywhere host (attached service account, workload identity, or
# GOOGLE_APPLICATION_CREDENTIALS): no key file in Terraform or in state. The key
# ring is in another project, so project_id names it. use_gcp_iam and project_id
# need a YugabyteDB Anywhere release later than 2026.1.
resource "yba_gcp_ear_config" "host_identity" {
  name          = "gcp-kms-central"
  use_gcp_iam   = true
  project_id    = "security-kms-project"
  location_id   = "global"
  key_ring_id   = "central-ring"
  crypto_key_id = "yugabyte-master-key"

  # Optional: the protection level of a crypto key that YugabyteDB Anywhere
  # creates, and a custom Cloud KMS endpoint (Private Service Connect or a
  # restricted VIP).
  protection_level = "HSM"
  kms_endpoint     = "kms.example.internal:443"
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_gcp_ear_config.service_account.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `crypto_key_id` (String) ID of the crypto key in the key ring. This key is the master key. YugabyteDB Anywhere creates it as a symmetric key when it does not exist and the identity can create crypto keys. A change forces replacement.
- `key_ring_id` (String) ID of the key ring, which is the last part of its resource name. YugabyteDB Anywhere creates the key ring when it does not exist and the identity can create key rings. A change forces replacement.
- `location_id` (String) Cloud KMS location of the key ring, for example `global`, `us-east1` or `europe`. A change forces replacement.
- `name` (String) Name of the configuration. Must be unique per customer. YugabyteDB Anywhere cannot rename a configuration, so a change forces replacement.

### Optional

- `credentials` (String, Sensitive) Service-account key JSON, inline or from `file(...)`. Required unless `use_gcp_iam` is true. The key's `project_id` is the key ring's project unless `project_id` is set. Can change in place: YugabyteDB Anywhere first checks that the new key can unwrap the active universe key of each universe that uses the configuration.
- `kms_endpoint` (String) Custom Cloud KMS endpoint, for Private Service Connect or a restricted VIP. A change forces replacement.
- `project_id` (String) GCP project that owns the key ring. Defaults to the project of the service-account key, or to the host's project with `use_gcp_iam`. Set it when the key ring is in another project. Not supported on YugabyteDB Anywhere 2026.1 or earlier. A change forces replacement.
- `protection_level` (String) `SOFTWARE` or `HSM`: the protection level of a crypto key that YugabyteDB Anywhere creates, `SOFTWARE` when not set. For an existing key, YugabyteDB Anywhere records the key's actual level, so leave this unset or set it to that level. A change forces replacement.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `use_gcp_iam` (Boolean) Authenticate as the YugabyteDB Anywhere host instead of with a key file: the attached service account on Compute Engine, workload identity on GKE, or the key file at `GOOGLE_APPLICATION_CREDENTIALS`. Not supported on YugabyteDB Anywhere 2026.1 or earlier. Can change in place; YugabyteDB Anywhere deletes the stored key when the configuration changes to the host identity.

### Read-Only

- `id` (String) The ID of this resource.
- `in_use` (Boolean) Whether a universe holds key history for this configuration. A configuration in use cannot be deleted.
- `uuid` (String) UUID of the configuration.

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `update` (String)

## Import

Import a GCP KMS configuration with its UUID. The `yba_ear_config` data source returns the
UUID for a configuration name:

```sh
terraform import yba_gcp_ear_config.service_account <config-uuid>
```

YugabyteDB Anywhere never returns `credentials`, so it stays empty after import. The first
apply sends the configured key again.
