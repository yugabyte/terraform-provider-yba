---
page_title: "yba_gcp_ear_config Resource - YugabyteDB Anywhere"
description: |-
  Encryption-at-rest configuration backed by a Google Cloud KMS crypto key. YugabyteDB Anywhere wraps each universe's universe key with the crypto key and unwraps it whenever a node needs it. Point the configuration at an existing key ring and crypto key, or let YugabyteDB Anywhere create them when the identity holds cloudkms.keyRings.create and cloudkms.cryptoKeys.create. An existing crypto key must have purpose ENCRYPT_DECRYPT, manual rotation (no rotation period), and an enabled primary version.
---

# yba_gcp_ear_config (Resource)

Encryption-at-rest configuration backed by a Google Cloud KMS crypto key. YugabyteDB Anywhere wraps each universe's universe key with the crypto key and unwraps it whenever a node needs it. Point the configuration at an existing key ring and crypto key, or let YugabyteDB Anywhere create them when the identity holds `cloudkms.keyRings.create` and `cloudkms.cryptoKeys.create`. An existing crypto key must have purpose `ENCRYPT_DECRYPT`, manual rotation (no rotation period), and an enabled primary version.

Two authentication modes are supported. Set `credentials` to a service-account key, or set `use_gcp_iam = true` to authenticate as the YugabyteDB Anywhere host: the attached service account on Compute Engine, workload identity on GKE, or the key at `GOOGLE_APPLICATION_CREDENTIALS`. Either identity needs `cloudkms.keyRings.get`, `cloudkms.cryptoKeys.get`, `cloudkms.cryptoKeyVersions.useToEncrypt`, `cloudkms.cryptoKeyVersions.useToDecrypt` and `cloudkms.locations.generateRandomBytes` on the key ring's project; YugabyteDB Anywhere checks them at create time.

~> **Note:** `use_gcp_iam` and `project_id` need a YugabyteDB Anywhere build with host-identity support for GCP KMS. Older builds reject `use_gcp_iam` at create time and ignore `project_id`.

~> **Security Note:** `credentials` is stored in the Terraform state file (marked sensitive). Use a secure backend and restrict access to your state files, or use `use_gcp_iam` so that no key leaves the host.

~> **Note:** Only the credential arguments can change in place. Every other argument is fixed by YugabyteDB Anywhere and forces replacement, and a configuration that any universe has used cannot be deleted: YugabyteDB Anywhere keeps the universe's key history after encryption is disabled and after the universe moves to another configuration, and only deleting the universe clears it. To move universes off a configuration, create the new one, change each universe's `encryption_at_rest.kms_config_uuid`, and keep the old configuration (or remove it from state) until its universes are gone.

~> **Drift Note:** Read refreshes `in_use` and the non-secret settings. Credentials are never read back, because YugabyteDB Anywhere masks them: a credential changed in the YugabyteDB Anywhere UI is not detected as drift. Re-apply from Terraform to restore the intended value.

~> **Import Note:** Import verifies the provider: importing a configuration that is not a GCP KMS configuration fails with the actual provider, so it can be imported with the matching `yba_*_ear_config` resource instead. Credentials cannot be recovered through the API and stay empty after import; the first apply submits them again.

Attach the configuration to a universe through its `encryption_at_rest` block. Enabling,
master key rotation, universe key rotation and disabling are documented in the
[Universe Edit Actions](../guides/universe-edit-actions.md#encryption-at-rest) guide. For the
YugabyteDB Anywhere side, see [Enable encryption at rest](https://docs.yugabyte.com/stable/yugabyte-platform/security/enable-encryption-at-rest/).

## Example Usage

```terraform
# Encryption-at-rest configuration on an existing Cloud KMS crypto key,
# authenticating with a service-account key. The key file's project_id names
# the key ring's project.
resource "yba_gcp_ear_config" "service_account" {
  name          = "gcp-kms-prod"
  credentials   = file("~/.gcp/kms-service-account.json")
  location_id   = "us-east1"
  key_ring_id   = "yugabyte-ring"
  crypto_key_id = "yugabyte-master-key"
}

# The same key ring reached with the YugabyteDB Anywhere host's own identity
# (attached service account, workload identity, or GOOGLE_APPLICATION_CREDENTIALS):
# no key file in Terraform or in state. The key ring lives in another project,
# so project_id names it explicitly. Needs a YugabyteDB Anywhere build with
# host-identity support for GCP KMS.
resource "yba_gcp_ear_config" "host_identity" {
  name          = "gcp-kms-central"
  use_gcp_iam   = true
  project_id    = "security-kms-project"
  location_id   = "global"
  key_ring_id   = "central-ring"
  crypto_key_id = "yugabyte-master-key"

  # Optional: protection level for a key YugabyteDB Anywhere creates, and a
  # custom Cloud KMS endpoint (Private Service Connect / restricted VIP).
  protection_level = "HSM"
  kms_endpoint     = "kms.example.internal:443"
}

# Attach a configuration to a universe through its encryption_at_rest block.
resource "yba_universe" "encrypted" {
  encryption_at_rest {
    kms_config_uuid = yba_gcp_ear_config.service_account.uuid
  }
  # ... clusters, communication_ports, ...
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `crypto_key_id` (String) Crypto key ID inside the key ring. This is the master key. Created as a symmetric key when missing and the identity may create crypto keys. Fixed after creation.
- `key_ring_id` (String) Key ring ID (the last segment of its resource name). Created when missing and the identity may create key rings. Fixed after creation.
- `location_id` (String) Cloud KMS location of the key ring, for example `global`, `us-east1` or `europe`. Fixed after creation.
- `name` (String) Name of the configuration, unique per customer. YugabyteDB Anywhere does not allow renaming, so a change forces replacement.

### Optional

- `credentials` (String, Sensitive) Service-account key JSON, inline or via `file(...)`. Required unless `use_gcp_iam` is true. The key's `project_id` names the key ring's project unless `project_id` is set. Can be changed in place: YugabyteDB Anywhere checks that the new key can still unwrap every universe key before storing it.
- `kms_endpoint` (String) Custom Cloud KMS endpoint, for Private Service Connect or a restricted VIP. Fixed after creation.
- `project_id` (String) GCP project that owns the key ring. Defaults to the service-account key's project, or to the host's project with `use_gcp_iam`. Set it when the key ring lives in another project. Fixed after creation.
- `protection_level` (String) `SOFTWARE` or `HSM`. Used when YugabyteDB Anywhere creates the crypto key (`SOFTWARE` when omitted). For an existing key YugabyteDB Anywhere records the key's actual level, so leave this unset or set it to the level the key has. Fixed after creation.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `use_gcp_iam` (Boolean) Authenticate as the YugabyteDB Anywhere host instead of with a key file: the attached service account on Compute Engine, workload identity on GKE, or the key at `GOOGLE_APPLICATION_CREDENTIALS`. Can be changed in place; switching from `credentials` drops the stored key.

### Read-Only

- `id` (String) The ID of this resource.
- `in_use` (Boolean) True while any universe holds key history for this configuration. Such a configuration cannot be deleted.
- `uuid` (String) UUID of the configuration.

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `update` (String)

## Import

GCP encryption-at-rest configurations can be imported using the configuration UUID. The
`yba_ear_config` data source returns it for a configuration name:

```sh
terraform import yba_gcp_ear_config.example <config-uuid>
```

`credentials` cannot be recovered through the API and stays empty after import; the first
apply submits the configured key again.
