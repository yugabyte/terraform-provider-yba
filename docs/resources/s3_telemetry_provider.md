---
page_title: "yba_s3_telemetry_provider Resource - YugabyteDB Anywhere"
description: |-
  ~> Experimental: This resource wraps a YugabyteDB Anywhere telemetry export API that is still experimental and may change in backward-incompatible ways across YBA releases. Pin your provider version and review release notes before upgrading.
  Amazon S3 Telemetry Provider resource. Defines a reusable S3 destination that universes can use to export audit logs and query logs — useful for long-term archival.
  ~> Note: YBA does not allow editing a telemetry provider in place. Any change to a field forces Terraform to destroy and recreate the resource. YBA also refuses to delete a provider that is still referenced by a universe's telemetry config, so the destroy step first enumerates every universe whose audit / query / metrics exporter list references this provider and rewrites that list with the provider removed (via a rolling-upgrade task on each universe). Once every detach task reaches a terminal state, the provider itself is deleted. The universes themselves are never destroyed — only their OpenTelemetry collector configuration is updated.
  ~> Drift Note: Read refreshes only name and tags. The Amazon S3 connection fields are not reconciled against the server, because YBA masks credentials in its responses and every field is ForceNew anyway. A field edited out-of-band in the YBA UI is therefore not detected as drift — re-apply from Terraform to restore the intended configuration.
  ~> Import Note: Import verifies the provider's type: importing a provider that is not a Amazon S3 destination fails with the actual type, so it can be imported with the matching yba_*_telemetry_provider resource instead.
  ~> Security Note: Credentials such as API keys, tokens, and secret access keys are stored in the Terraform state file (marked sensitive). Use a secure backend and restrict access to your state files.
---

# yba_s3_telemetry_provider (Resource)

~> **Experimental:** This resource wraps a YugabyteDB Anywhere telemetry export API that is still experimental and may change in backward-incompatible ways across YBA releases. Pin your provider version and review release notes before upgrading.

Amazon S3 Telemetry Provider resource. Defines a reusable S3 destination that universes can use to export audit logs and query logs — useful for long-term archival.

~> **Note:** YBA does not allow editing a telemetry provider in place. Any change to a field forces Terraform to destroy and recreate the resource. YBA also refuses to delete a provider that is still referenced by a universe's telemetry config, so the destroy step first enumerates every universe whose audit / query / metrics exporter list references this provider and rewrites that list with the provider removed (via a rolling-upgrade task on each universe). Once every detach task reaches a terminal state, the provider itself is deleted. The universes themselves are never destroyed — only their OpenTelemetry collector configuration is updated.

~> **Drift Note:** Read refreshes only `name` and `tags`. The Amazon S3 connection fields are **not** reconciled against the server, because YBA masks credentials in its responses and every field is `ForceNew` anyway. A field edited out-of-band in the YBA UI is therefore not detected as drift — re-apply from Terraform to restore the intended configuration.

~> **Import Note:** Import verifies the provider's type: importing a provider that is not a Amazon S3 destination fails with the actual type, so it can be imported with the matching `yba_*_telemetry_provider` resource instead.

~> **Security Note:** Credentials such as API keys, tokens, and secret access keys are stored in the Terraform state file (marked sensitive). Use a secure backend and restrict access to your state files.

## Example Usage

```terraform
# S3 archival destination (long-term audit/query log storage).
resource "yba_s3_telemetry_provider" "audit_archive" {
  name = "audit-archive"

  bucket           = "yba-audit-logs"
  region           = "us-west-2"
  access_key       = var.aws_access_key
  secret_key       = var.aws_secret_key
  directory_prefix = "yb-logs"
  file_prefix      = "audit-"

  # Optional: assume a role for the bucket writes.
  role_arn = "arn:aws:iam::111111111111:role/yba-s3-archive"

  include_universe_and_node_in_prefix = true

  # Optional tags, upserted as attributes onto every exported record.
  tags = {
    env = "prod"
  }
}

# S3-compatible store (e.g. MinIO) with path-style addressing and an
# hourly directory layout.
resource "yba_s3_telemetry_provider" "minio" {
  name = "minio-archive"

  bucket     = "yba-logs"
  region     = "us-east-1"
  access_key = var.minio_access_key
  secret_key = var.minio_secret_key

  endpoint         = "http://minio.internal:9000"
  disable_ssl      = true
  force_path_style = true
  partition        = "hour"

  # Serialization format: OTLP_JSON (YBA default) or SUMO_IC (logs only).
  marshaler = "OTLP_JSON"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `access_key` (String, Sensitive) AWS access key with bucket write permissions.
- `bucket` (String) S3 bucket name.
- `name` (String) Name of the telemetry provider configuration.
- `region` (String) AWS region of the bucket.
- `secret_key` (String, Sensitive) AWS secret key for the access key.

### Optional

- `directory_prefix` (String) S3 prefix (root directory inside the bucket) to write objects under.
- `disable_ssl` (Boolean) Disable SSL when talking to the S3 endpoint.
- `endpoint` (String) Optional override endpoint URL (e.g. for VPC endpoints or S3-compatible stores).
- `file_prefix` (String) Optional file-name prefix prepended to every object.
- `force_path_style` (Boolean) Force path-style addressing instead of the default virtual-hosted style.
- `include_universe_and_node_in_prefix` (Boolean) Append `<universe-uuid>/<node-name>` to the directory prefix when writing objects.
- `marshaler` (String) Optional marshaler used to serialize records (defaults to YBA's choice).
- `partition` (String) Time granularity of the S3 object directory layout. One of `hour` or `minute` (YBA default: `minute`).
- `role_arn` (String) Optional IAM role ARN to assume.
- `tags` (Map of String) Optional string tags associated with the configuration.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `read` (String)

## Replacing an in-use provider

YBA does not support editing a telemetry provider — any change to a
field forces Terraform to destroy-and-recreate the resource. YBA also
rejects delete requests for a provider that is still referenced by a
universe's telemetry configuration:

```
Cannot delete Telemetry Provider '...', as it is in use.
```

The destroy step handles this proactively: before issuing the YBA delete
it enumerates every universe whose telemetry config references the
provider and rewrites each universe's config with the provider filtered
out of the audit/query/metrics exporter lists (through the unified
`/api/v2/customers/{c}/universes/{u}/export-telemetry-configs` endpoint).
It waits for every resulting rolling-upgrade task to reach a terminal
state, and only then issues the YBA delete. The detach step is therefore
the canonical "detach, then mutate" workflow — destroy-and-recreate
plans, plain `terraform destroy`, and any `yba_universe_telemetry_config`
update planned in the same `terraform apply` (which is then applied with
the new provider UUID) all go through it.

The universes themselves are never destroyed — only their OpenTelemetry
collector configuration is updated.

## Import

Telemetry providers can be imported using their UUID. Import verifies
the provider's type: importing a provider of a different sink type fails
with a message naming the matching resource.

```sh
terraform import yba_s3_telemetry_provider.audit_archive <telemetry-provider-uuid>
```
