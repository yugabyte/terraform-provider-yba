<!--
Licensed to YugabyteDB, Inc. under one or more contributor license
agreements. See the NOTICE file distributed with this work for
additional information regarding copyright ownership. Yugabyte
licenses this file to you under the Mozilla License, Version 2.0
(the "License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at
http://mozilla.org/MPL/2.0/.
-->

# Agents

Instructions for AI coding assistants working on `terraform-provider-yba`.

This is the **publicly released** YBA Terraform provider. Schemas, field
names, and lifecycle behaviour are API contracts that ship into customer
state files. Bias toward the safer, more thoroughly tested, more clearly
documented option.

## Change scope

Limit edits to the files and symbols needed for the requested behaviour or
API contract. **Do not** sweep large parts of the codebase unrelated to
that work with changes that only adjust lint, formatting, style,
comments (including doc-only rewording), license headers, or similar
incidental churn. Keep reviews focused; run formatters on files you
actually touch unless the operator explicitly asked for a repo-wide
cleanup.

## License

Use the **MPL-2.0** boilerplate at the top of every Go, shell, and Markdown
file. Copy it verbatim from any existing `internal/` source file.

## Build & Test

| Target | Use |
| --- | --- |
| `make install` | Build + install to `~/.terraform.d/plugins/...`. |
| `make test` | Unit tests (no YBA, no creds, fast). |
| `make testacc` | Acceptance tests (live YBA + cloud creds, slow). |
| `make documents` | Regenerate `docs/` from `templates/` + schema. **Run after every schema change** and commit the diff. |
| `make fmt` / `make fmtTf` | Format Go (100-col) and Terraform. |

Before declaring any change done: `go vet ./... && go test ./internal/...
&& make install`. For schema changes, also `make documents`.

## Resource Design (non-negotiable)

- `ForceNew` whenever the YBA endpoint has no PUT — say so in the field
  `Description`.
- `ExactlyOneOf` on every polymorphic block; back it with a package-level
  slice (e.g. `telemetryConfigBlocks`) so the schema, type-resolver
  switch, and tests stay in sync.
- `Sensitive: true` for every credential, plus a `~> **Security Note:**`
  callout in the resource `Description` (values land in state).
- `Importer` is required unless YBA truly cannot import.
- `Description` is required on every field and the resource itself —
  these strings render directly into user-facing docs. Use `~> **Note:**`
  / `~> **Warning:**` callouts; document performance gotchas (e.g.
  per-node sleep defaults that compound on multi-node universes).
- No noise fields — never expose computed values that are identical for
  every resource managed by a given provider instance (e.g.
  `customer_uuid`).
- Every resource gets a working example at
  `examples/resources/yba_<name>/resource.tf` covering every nested
  block variant.

## Lifecycle

- `Create` calls `d.SetId` only after the YBA task reaches a terminal
  success state.
- `Read` is idempotent against out-of-band deletes: detect via
  `utils.IsHTTPNotFound` or a typed sentinel from `internal/api`,
  `d.SetId("")`, return `nil`.
- `Delete` is idempotent for already-gone resources **and surfaces every
  other failure**. `tflog.Warn(...) + return nil` on non-404 errors is
  banned — it silently corrupts state.
- For "in-use" delete failures, **proactively detach** referencing
  resources before issuing the delete; do not substring-match YBA's
  error body in the resource layer.
- Long-running ops (rolling restarts) use a package-local
  `timeouts.go` constant in **hours**. All three CRUD timeouts share
  one source of truth.

## Error & Task Handling

- Route every HTTP error through `utils.ErrorFromHTTPResponse`. Do not
  roll your own status helper.
- For "missing resource" conditions YBA returns through several HTTP
  shapes (404 + 400 with body markers + 500 with body markers), define
  a typed sentinel in `internal/api` (`var ErrXMissing = errors.New(...)`)
  and detect with `errors.Is`. No string matching in resource code.
- For OpenAPI errors, use `utils.OpenAPIErrorBody` (which `errors.As`
  against the v1 and v2 generated client types). Do **not** invent
  `interface { Body() []byte }` shims.
- Every universe-mutating call goes through `utils.DispatchAndWait`
  (dispatch + 409 retry + WaitForTask) or `utils.RetryOnUniverseTaskConflict`
  (dispatch + 409 retry only). Hand-rolling the dispatch-wait-retry
  triplet is a bug waiting to happen.

## Use `internal/utils` First

Almost every helper you might want already exists. Check before adding:
pointer helpers (`GetBoolPointer`, `GetStringPointer`, `GetInt32Pointer`),
list/map plumbing (`MapFromSingletonList`, `StringSlice`, `StringMap`),
HTTP error helpers (`ErrorFromHTTPResponse`, `IsHTTPNotFound`,
`OpenAPIErrorBody`, `IsUniverseTaskConflict`), task helpers
(`WaitForTask`, `DispatchAndWait`, `RetryOnUniverseTaskConflict`), and
update-time state revert (`RevertFields`). If something is genuinely
missing, add it to `utils` with tests rather than inlining.

## Generated Clients

Two SDKs are imported as separate Go modules: `client` (v1) and
`clientv2` (v2). Prefer v2 for new code; mixing both in one resource is
fine. For YBA endpoints not yet in either client, add a typed wrapper to
`internal/api/VanillaClient` (see `internal/api/telemetry.go` as the
reference shape).

## Tests

Two classes:

- Unit (`Test*`) — fast, hermetic. Drive the real generated client through
  `httptest.NewServer` when you need to exercise HTTP error paths (the
  `*GenericOpenAPIError` types cannot be hand-constructed). Add cheap
  schema sanity tests for polymorphic blocks (`ForceNew`, `MaxItems=1`,
  `ExactlyOneOf`, every block wired through the type switch).
- Acceptance (`TestAcc*`) — required for any new resource and for any
  lifecycle-affecting change.

Write tests that protect against the regressions another agent is most
likely to introduce — not for coverage percentage.

## Git

- Imperative-mood commit messages, no `Fix:` / `Feat:` prefixes.
- **Never** add commit trailers (`Made-with`, `Co-authored-by`, etc.).
  The Cursor CLI injects `Made-with: Cursor` automatically; strip it
  with `git commit --amend -m "$(git log --format=%B -1 | sed
  '/^Made-with:/d')"` before every push.
- Never force-push to `main`, never amend a commit that has been pushed,
  never `--no-verify` without explicit operator approval.

## Secrets

Never commit API keys, tokens, service-account JSON, `.tfvars` with real
credentials, private keys, or any `*.secret*` / `*credentials*` /
`.env*` file. If a stack trace contains a secret, redact it before
including it in any response or log line.

## Self-Learning

When the operator corrects you, add the rule here and commit it with the
code change. Mark repeated or emphasised corrections **critical** in
bold. AGENTS.md is the only memory file — do not create others.
