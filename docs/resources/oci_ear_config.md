---
page_title: "yba_oci_ear_config Resource - YugabyteDB Anywhere"
description: |-
  Manages a YugabyteDB Anywhere encryption-at-rest configuration that uses an Oracle Cloud Infrastructure Vault key as the master key. YugabyteDB Anywhere uses the key to wrap and unwrap the universe keys of each universe that uses the configuration.
---

# yba_oci_ear_config (Resource)

Manages a YugabyteDB Anywhere encryption-at-rest configuration that uses an Oracle Cloud Infrastructure Vault key as the master key. YugabyteDB Anywhere uses the key to wrap and unwrap the universe keys of each universe that uses the configuration.

~> **Note:** Requires YugabyteDB Anywhere 2026.1.2.0 or later. `terraform plan` fails on an older YBA.

Point the configuration at an existing AES key with `key_name`, or let YugabyteDB Anywhere create an AES-256 key with that display name. Terraform does not manage or delete a key that YugabyteDB Anywhere creates. An existing key must be enabled and use the AES algorithm. The display name must match one key in the compartment.

There are two authentication modes. Set `tenancy_id`, `user_id`, `fingerprint` and `private_key_content` for an API signing key, or set `use_instance_principal = true` to authenticate as the YugabyteDB Anywhere host. The host must be an OCI compute instance in a dynamic group. Either identity needs policies to use the vault and to use keys in `compartment_id`, and to manage keys when YugabyteDB Anywhere creates the key. YugabyteDB Anywhere checks them when it creates or edits the configuration.

An apply fails while the global runtime config `yb.kms.allow_oci` is false. YugabyteDB Anywhere allows OCI KMS when that key is unset or true.

~> **Security Note:** `private_key_content` is stored in the Terraform state file and marked sensitive. Use a secure state backend and restrict access to state files, or use `use_instance_principal`, which keeps the key out of Terraform.

~> **Note:** Only the arguments whose description says so can change in place, and Terraform ignores a change after creation to the arguments whose description says that. A change to any other argument forces replacement. YugabyteDB Anywhere does not delete a configuration that a universe has used: it keeps the universe's key history, also after encryption is disabled or the universe moves to another configuration, until the universe is deleted. A destroy of such a configuration fails, and the error names the universes. To move universes to a new configuration, create it, change `encryption_at_rest.kms_config_uuid` on each universe, and keep the old configuration (or remove it from state) until its universes are deleted. Give the new configuration `depends_on` on the old one, so that `terraform destroy` deletes the universe before the old configuration.

~> **Note:** Terraform refreshes `in_use` and the non-secret settings. YugabyteDB Anywhere masks credentials, so a credential changed outside Terraform, for example in the YugabyteDB Anywhere UI, does not show as drift. An apply sends a credential again only when the credential or the authentication mode changes.

Use the configuration in a universe through its `encryption_at_rest` block. The
[Encryption at Rest](../guides/universe-edit-actions.md#encryption-at-rest) section of the
Universe Edit Actions guide describes how to enable encryption, rotate the master key or the
universe key, and disable encryption. For the YugabyteDB Anywhere side, see
[Enable encryption at rest](https://docs.yugabyte.com/stable/yugabyte-platform/security/enable-encryption-at-rest/).

## Example Usage

```terraform
# Encryption-at-rest configuration on an existing OCI Vault key, with an API
# signing key for authentication.
resource "yba_oci_ear_config" "api_key" {
  name                = "oci-kms-prod"
  tenancy_id          = "ocid1.tenancy.oc1..aaaa"
  user_id             = "ocid1.user.oc1..aaaa"
  fingerprint         = "12:34:56:78:9a:bc:de"
  private_key_content = file("${path.module}/oci_api_key.pem")
  compartment_id      = "ocid1.compartment.oc1..aaaa"
  vault_id            = "ocid1.vault.oc1.iad.aaaa"
  region              = "us-ashburn-1"
  key_name            = "yugabyte-master-key"
}

# Authentication as the YugabyteDB Anywhere host (instance principal): no API
# signing key in Terraform or in state. The host must be an OCI compute
# instance in a dynamic group that can use the vault and the key. YugabyteDB
# Anywhere creates an AES-256 key when key_name is not already in the
# compartment. Requires YugabyteDB Anywhere 2026.1.2.0 or later.
resource "yba_oci_ear_config" "host_identity" {
  name                   = "oci-kms-central"
  use_instance_principal = true
  compartment_id         = "ocid1.compartment.oc1..aaaa"
  vault_id               = "ocid1.vault.oc1.iad.bbbb"
  region                 = "us-ashburn-1"
  key_name               = "yugabyte-master-key"
}

# A universe uses a configuration through its encryption_at_rest block:
#
#   encryption_at_rest {
#     enabled         = true
#     kms_config_uuid = yba_oci_ear_config.api_key.uuid
#   }
#
# The yba_universe example shows a complete universe with the block.
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `compartment_id` (String) OCID of the compartment that holds the vault and the key. Can change in place.
- `key_name` (String) Display name of the master key in the vault. YugabyteDB Anywhere uses an existing AES key with this name, or creates an AES-256 key when the name is absent and the identity can create keys. A change forces replacement.
- `name` (String) Name of the configuration. Must be unique per customer. YugabyteDB Anywhere cannot rename a configuration, so a change forces replacement.
- `region` (String) OCI region of the vault, for example `us-ashburn-1`. A change forces replacement.
- `vault_id` (String) OCID of the vault that holds the master key. A change forces replacement.

### Optional

- `fingerprint` (String) Fingerprint of the API signing key. Required when `use_instance_principal` is false. Can change in place.
- `private_key_content` (String, Sensitive) PEM contents of the API signing key, inline or from `file(...)`. Required when `use_instance_principal` is false. Can change in place: YugabyteDB Anywhere checks the credentials, the vault and the key before it stores the change.
- `tenancy_id` (String) OCID of the tenancy that owns the API signing key. Required, with `user_id`, `fingerprint` and `private_key_content`, when `use_instance_principal` is false. Can change in place.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `use_instance_principal` (Boolean) Authenticate as the YugabyteDB Anywhere host with an OCI instance principal, instead of an API signing key. Requires YugabyteDB Anywhere 2026.1.2.0 or later. A change forces replacement.
- `user_id` (String) OCID of the user that owns the API signing key. Required when `use_instance_principal` is false. Can change in place.

### Read-Only

- `id` (String) The ID of this resource.
- `in_use` (Boolean) Whether a universe holds key history for this configuration. A configuration in use cannot be deleted.
- `key_id` (String) OCID of the master key YugabyteDB Anywhere resolved from `key_name`.
- `uuid` (String) UUID of the configuration.

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `update` (String)

## Import

Import an OCI KMS configuration with its UUID. The `yba_ear_config` data source returns the
UUID for a configuration name:

```sh
terraform import yba_oci_ear_config.api_key <config-uuid>
```

Import fails for a configuration of another KMS provider, and the error names that provider.
Import reads every argument except `private_key_content`. YugabyteDB Anywhere masks the
private key when it lists a configuration, so import leaves `private_key_content` empty.
When the configuration sets `private_key_content`, the first plan after the import shows
one in-place update, and the apply sends the key from your configuration. When the
configuration uses `use_instance_principal`, there is no private key to restore, and the
next plan is empty.
