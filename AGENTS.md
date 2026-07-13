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

This is the **publicly released** YBA Terraform provider (v1.0). Schemas, field
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

The build system is `GNUmakefile`. Key targets:

| Target | Use |
| --- | --- |
| `make install` | Build + install to `~/.terraform.d/plugins/...` and write a `dev_overrides` `.terraformrc`. |
| `make test` | Unit tests via `gotestsum` (`./... -skip '^TestAcc'`) — no YBA, no creds, fast. |
| `make acctest` | Acceptance tests (`TF_ACC=1`, live YBA + cloud creds, slow). Use `make acctest-long` for the ~15-min universe tests. |
| `make documents` (`make docs`) | Regenerate `docs/` from `templates/` + schema via `tfplugindocs`. **Run after every schema change** and commit the diff. |
| `make fmt` | Format / auto-fix Go (`golines`, 100-col); alias of `lint-fix`. |
| `make fmtTf` | `terraform fmt -recursive`. |
| `make lint` | `golangci-lint` (`lint-go`) + docs lint (`lint-docs`). |

Before declaring any change done: `go vet ./... && make lint && make test &&
make install`. For schema changes, also `make documents`.

**critical: never commit or push code that fails `make lint`.** CI runs
`golangci-lint run ./...` on every PR and a lint failure blocks the merge — it
is trivial to catch locally first, so always run `make lint` (or at minimum
`golangci-lint run ./...`) before committing. This applies to **every** Go file
the change touches, including pre-existing lint issues in files you edit:
`golines` (100-col) and `staticcheck` are the usual offenders. Auto-fix
formatting with `golangci-lint fmt ./...` (or `make fmt`); for an unavoidable
deprecated-API use, add a narrowly-scoped `//nolint:<linter> // <reason>` rather
than leaving the build red.

## Resource Design (non-negotiable)

- `ForceNew` whenever the YBA endpoint has no PUT — say so in the field
  `Description`.
- `ExactlyOneOf` on every polymorphic block; back it with a package-level
  slice so the schema, type-resolver switch, and tests stay in sync.
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
  `examples/resources/yba_<name>/resource.tf` showing **every field**:
  plain attributes (string/bool/map arguments such as `tags`) as well
  as every nested block variant. Fields that are mutually exclusive
  (e.g. per-`auth_type` credentials) get separate example resources in
  the same file.
- Telemetry sinks ship as per-sink resources
  (`yba_<sink>_telemetry_provider`) built on the `sinkSpec` factory in
  `internal/telemetry/sink.go` — add a new sink as a new spec + resource,
  not as a block on a shared polymorphic resource.

## Lifecycle

- `Create` calls `d.SetId` only after the YBA task reaches a terminal
  success state.
- `Read` is idempotent against out-of-band deletes: detect via
  `utils.IsHTTPNotFound` (or a typed sentinel — see Error & Task Handling),
  `d.SetId("")`, return `nil`.
- `Delete` is idempotent for already-gone resources **and surfaces every
  other failure**. `tflog.Warn(...) + return nil` on non-404 errors is
  banned — it silently corrupts state.
- For "in-use" delete failures, **proactively detach** referencing
  resources before issuing the delete; do not substring-match YBA's
  error body in the resource layer.
- Long-running ops (e.g. rolling restarts): define timeout constants in
  **hours** in one place per package so all three CRUD timeouts share one
  source of truth.

## Error & Task Handling

- Route every HTTP error through `utils.ErrorFromHTTPResponse`. Do not
  roll your own status helper.
- Detect out-of-band deletes / missing resources with
  `utils.IsHTTPNotFound`. For "missing resource" conditions YBA returns
  through non-404 shapes (400/500 with body markers), prefer typed
  detection (a sentinel error + `errors.Is`) over substring-matching
  YBA's error body in resource code.
- Every universe-mutating call goes through `utils.DispatchAndWait`
  (dispatch + 409 retry + `WaitForTask`) or
  `utils.RetryOnUniverseTaskConflict` (dispatch + 409 retry only).
  Hand-rolling the dispatch-wait-retry triplet is a bug waiting to happen.

## Use `internal/utils` First

Almost every helper you might want already exists in `internal/utils`.
Check before adding: pointer helpers (`GetBoolPointer`, `GetStringPointer`,
`GetInt32Pointer`), list/map plumbing (`MapFromSingletonList`,
`StringSlice`, `StringMap`), HTTP error helpers (`ErrorFromHTTPResponse`,
`IsHTTPNotFound`, `IsUniverseTaskConflict`), task helpers (`WaitForTask`,
`DispatchAndWait`, `RetryOnUniverseTaskConflict`), and update-time state
revert (`RevertFields`). If something is genuinely missing, add it to
`utils` with tests rather than inlining.

## Generated Client

The provider depends on a single generated SDK: `client` =
`github.com/yugabyte/platform-go-client` (v1), imported under the `client`
alias. There is no v2 client wired in yet — the `make updatev2client`
target scaffolds a future `github.com/yugabyte/platform-go-client/v2`, but
it is not a `go.mod` dependency and is not used in code.

For YBA endpoints not yet in the generated client, add a typed wrapper to
`internal/api` on the `VanillaClient` type (`internal/api/api.go`) — see
`internal/api/releases.go` / `internal/api/update_options.go` for the
reference shape.

## Tests

Two classes:

- Unit (`Test*`) — fast, hermetic. Drive the real generated client through
  `httptest.NewServer` when you need to exercise HTTP error paths (the
  `*GenericOpenAPIError` types cannot be hand-constructed). Add cheap
  schema sanity tests for polymorphic blocks (`ForceNew`, `MaxItems=1`,
  `ExactlyOneOf`, every block wired through the type switch).
- Acceptance (`TestAcc*`) — required for any new resource and for any
  lifecycle-affecting change. Run with `make acctest`.

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
bold. `AGENTS.md` is the tracked source of truth for provider conventions
(a local, gitignored `CLAUDE.md` may `@AGENTS.md` to load it into Claude
Code; durable cross-session memory lives in the Meko datapack below) — do
not add other tracked convention files.

## Meko (memory & knowledge)

When using Meko MCP tools in this repository, **default to the
`terraform-provider-yba` datapack**:

- `datapack_id`: `c6ffd40e-a627-4381-ad61-92cacad888a2`
- `agent_id`: `claude_code:terraform-provider-yba` (set by the SessionStart hook)

Use this datapack for all datapack-scoped operations — `knowledgebase_search`
and any memory/knowledge reads or writes that accept a `datapack_id`. Only
fall back to the common `meko_agent` bucket for genuinely cross-project facts
(user identity, global preferences).
