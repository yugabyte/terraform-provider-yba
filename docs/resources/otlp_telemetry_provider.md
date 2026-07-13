---
page_title: "yba_otlp_telemetry_provider Resource - YugabyteDB Anywhere"
description: |-
  ~> Experimental: This resource wraps a YugabyteDB Anywhere telemetry export API that is still experimental and may change in backward-incompatible ways across YBA releases. Pin your provider version and review release notes before upgrading.
  OTLP Telemetry Provider resource. Defines a reusable OpenTelemetry Protocol destination that universes can use to export audit logs, query logs, and metrics.
  ~> Note: YBA does not allow editing a telemetry provider in place. Any change to a field forces Terraform to destroy and recreate the resource. YBA also refuses to delete a provider that is still referenced by a universe's telemetry config, so the destroy step first enumerates every universe whose audit / query / metrics exporter list references this provider and rewrites that list with the provider removed (via a rolling-upgrade task on each universe). Once every detach task reaches a terminal state, the provider itself is deleted. The universes themselves are never destroyed — only their OpenTelemetry collector configuration is updated.
  ~> Drift Note: Read refreshes only name and tags. The OTLP connection fields are not reconciled against the server, because YBA masks credentials in its responses and every field is ForceNew anyway. A field edited out-of-band in the YBA UI is therefore not detected as drift — re-apply from Terraform to restore the intended configuration.
  ~> Import Note: Import verifies the provider's type: importing a provider that is not a OTLP destination fails with the actual type, so it can be imported with the matching yba_*_telemetry_provider resource instead.
  ~> Security Note: Credentials such as API keys, tokens, and secret access keys are stored in the Terraform state file (marked sensitive). Use a secure backend and restrict access to your state files.
---

# yba_otlp_telemetry_provider (Resource)

~> **Experimental:** This resource wraps a YugabyteDB Anywhere telemetry export API that is still experimental and may change in backward-incompatible ways across YBA releases. Pin your provider version and review release notes before upgrading.

OTLP Telemetry Provider resource. Defines a reusable OpenTelemetry Protocol destination that universes can use to export audit logs, query logs, and metrics.

~> **Note:** YBA does not allow editing a telemetry provider in place. Any change to a field forces Terraform to destroy and recreate the resource. YBA also refuses to delete a provider that is still referenced by a universe's telemetry config, so the destroy step first enumerates every universe whose audit / query / metrics exporter list references this provider and rewrites that list with the provider removed (via a rolling-upgrade task on each universe). Once every detach task reaches a terminal state, the provider itself is deleted. The universes themselves are never destroyed — only their OpenTelemetry collector configuration is updated.

~> **Drift Note:** Read refreshes only `name` and `tags`. The OTLP connection fields are **not** reconciled against the server, because YBA masks credentials in its responses and every field is `ForceNew` anyway. A field edited out-of-band in the YBA UI is therefore not detected as drift — re-apply from Terraform to restore the intended configuration.

~> **Import Note:** Import verifies the provider's type: importing a provider that is not a OTLP destination fails with the actual type, so it can be imported with the matching `yba_*_telemetry_provider` resource instead.

~> **Security Note:** Credentials such as API keys, tokens, and secret access keys are stored in the Terraform state file (marked sensitive). Use a secure backend and restrict access to your state files.

## Example Usage

```terraform
# Generic OTLP destination (e.g. Prometheus with the OTLP receiver).
#
# When this resource is replaced (any field change forces a recreate),
# Terraform first rewrites every universe whose telemetry config
# references this provider to drop the exporter (rolling upgrade), then
# deletes the old provider and creates the replacement. The universe
# itself is never destroyed.
resource "yba_otlp_telemetry_provider" "prometheus" {
  name = "prometheus"

  endpoint        = "http://10.242.32.5:9091/api/v1/otlp/v1/metrics"
  auth_type       = "NoAuth"
  protocol        = "HTTP"
  compression     = "gzip"
  timeout_seconds = 5

  # Optional tags, upserted as attributes onto every exported record.
  tags = {
    env = "prod"
  }
}

# OTLP collector behind basic auth, with per-signal endpoint overrides
# (HTTP protocol only) and extra headers.
resource "yba_otlp_telemetry_provider" "collector" {
  name = "otel-collector"

  endpoint            = "https://collector.example.com:4318"
  protocol            = "HTTP"
  auth_type           = "BasicAuth"
  basic_auth_username = var.otlp_username
  basic_auth_password = var.otlp_password

  logs_endpoint    = "https://collector.example.com:4318/v1/logs"
  metrics_endpoint = "https://collector.example.com:4318/v1/metrics"

  headers = {
    "X-Scope-OrgID" = "yba"
  }
}

# OTLP endpoint authenticated with a bearer token (gRPC transport).
resource "yba_otlp_telemetry_provider" "bearer" {
  name = "otel-bearer"

  endpoint     = "https://otlp.example.com:4317"
  auth_type    = "BearerToken"
  bearer_token = var.otlp_bearer_token
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `endpoint` (String) OTLP endpoint URL.
- `name` (String) Name of the telemetry provider configuration.

### Optional

- `auth_type` (String) Authentication type. One of NoAuth, BasicAuth, BearerToken.
- `basic_auth_password` (String, Sensitive) BasicAuth password (only used when auth_type=BasicAuth).
- `basic_auth_username` (String) BasicAuth username (only used when auth_type=BasicAuth).
- `bearer_token` (String, Sensitive) Bearer token (only used when auth_type=BearerToken).
- `compression` (String) Compression for the OTLP exporter. One of gzip, none, snappy, zstd.
- `headers` (Map of String) Additional headers to send on every OTLP request.
- `logs_endpoint` (String) Override endpoint for log export (HTTP protocol only). When set, the value of `endpoint` is ignored for logs.
- `metrics_endpoint` (String) Override endpoint for metric export (HTTP protocol only). When set, the value of `endpoint` is ignored for metrics.
- `protocol` (String) Transport protocol. One of gRPC, HTTP.
- `tags` (Map of String) Optional string tags associated with the configuration.
- `timeout_seconds` (Number) Timeout in seconds for the OTLP exporter. Must be positive.
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
terraform import yba_otlp_telemetry_provider.prometheus <telemetry-provider-uuid>
```
