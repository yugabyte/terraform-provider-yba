# Review guide

Judgement calls for reviewing PRs in this repository. `make lint` owns
formatting, import order, line length and spelling. `AGENTS.md` owns the hard
rules (ForceNew, ExactlyOneOf, Sensitive, Importer, Description,
DispatchAndWait, utils-first). Apply both from their source; do not repeat
their findings here.

Verify every claim about YBA server behaviour against the YBA Java source
(`managed/src/main/java` in yugabyte-db, locally at `~/code/yugabyte-db`:
controllers, `*Task.java`, `*Helper.java`). A doc sentence or code comment
that states a YBA rule without a traceable source is a finding.

## How to review

- Flag at MEDIUM severity and above. Suppress LOW nits unless they reveal a
  correctness problem.
- Do not flag formatting, import order, naming that matches the surrounding
  module, or a missing test where an existing test already exercises the
  changed behaviour. Flag a missing test only when the path has no coverage.
- Flag anything in the diff that contradicts a comment, a `Description`, a
  doc page or the commit message in the same diff.
- With no actionable finding, reply `LGTM` in one line, with at most a
  one-line caveat. Do not recap the diff.
- Do not re-flag across review rounds. Skip a resolved thread, a thread whose
  reply explains why the suggestion was declined or applied differently, and
  a hunk that is byte-identical to the one you already reviewed.

## Upgrade safety

**A provider upgrade must never change what happens to a resource that is
already managed.** Customers upgrade without reading the changelog or the
docs. A YBA that goes down, gets reinstalled or is replaced because
`terraform apply` now does something it did not do before is the most severe
failure this project can ship.

- The first question for any lifecycle change: what do the first plan and
  apply after the upgrade do to existing state? Any answer but "nothing" is
  a finding. New behaviour is opt-in through a new argument whose default is
  the old behaviour, never a changed default.
- `Read` never gains the power to drop a resource from state on a verdict
  that can be wrong, and never gains a dependency that `terraform plan` did
  not have before. A wrong verdict recreates something live; a new
  dependency stops pipelines that only had it at apply time.
- `ForceNew`, `Computed`, defaults and `DiffSuppressFunc` on an existing
  attribute are contract. Changing them replaces or rewrites customer
  resources on the next apply.
- A doc note, a `~> **Warning:**` or an upgrade guide does not make a change
  safe. If the change is only safe for an operator who read something first,
  it is not safe.
- When a fix for a real gap cannot meet this, the gap stays open and is
  written up as a limitation. Shipping the fix anyway is the finding.
- When the diff adds, removes or renames a schema field, changes `ForceNew`,
  `Default` or a type, or changes what `Read` or `Delete` does, the PR
  description states what the first plan after upgrade shows on existing
  state. Flag the missing statement only then.

## Change shape

- One logical change per PR. Mechanical preparation (a client bump, a key
  refactor, a version bump) ships first as its own PR.
- A feature branch is rebased on `main`, never merged with it.
- Design decisions go in the PR description. A new tracked design doc is a
  finding.
- A defect found in review is removed in the same change or reported with a
  concrete fix plan for the operator. A gated-off path left in the tree (a
  CI step reverted so a test can never run, a trigger disabled around broken
  code) is a finding: it misleads later readers and re-arms when someone
  re-wires the trigger.

## Schema and naming

- Resource and attribute names follow the public feature name and `yba-cli`,
  never the Java model.
- A family of related configs ships as one resource per type on a shared
  spec factory. A new polymorphic resource with a type switch is a finding.
- `Sensitive` marks secrets only. A username, region, endpoint or key id
  stays visible so a typo shows in `terraform plan`.
- When one field's default follows another, the code tells "set in config"
  from "echoed from state" with `d.GetRawConfig()`. `d.Get` treats a state
  echo as an explicit pin.
- An `Optional + Computed` block that models a server-side toggle carries an
  explicit `enabled` field, and removing the block changes nothing.
- A `DiffSuppressFunc` on a value the server re-encodes (PEM, JSON, YAML)
  compares the decoded form, not trimmed text. Trimmed text on a `ForceNew`
  attribute diffs forever.
- A trigger or counter attribute documents its adoption path: adding it to an
  already-managed resource counts as a change and fires on the next apply.
- `terraform import` must give a clean plan. When a `Required + ForceNew`
  attribute cannot be read back, the Import section states the consequence
  and the escape (`ignore_changes` or recreate).

## Read, delete and drift

- `Read` sets every attribute the resource owns, nested blocks included, so
  an out-of-band edit shows as drift, even when every config field is
  `ForceNew`.
- When the resource ID is a parent's UUID (one config per parent), two
  resource blocks for one parent oscillate forever. The description names
  the constraint and the code or docs defend against it.
- A "gone" sentinel must not fire on a 404 for an unknown route. Play
  answers an unmatched route with a plain-text 404, so an older YBA that
  lacks the route looks like a deleted resource. Check the body or route
  shape before clearing state.
- A delete or disable that posts a replace-not-merge spec clears every
  server-side section, including ones another writer added. Rebuild the
  spec from live state and clear only what this resource wrote, or say so in
  the resource description.

## YBA behaviour claims

- Every stated gate lists all of its conditions: version floor, runtime
  flag, prior configuration step, platform (VM or Kubernetes), both
  directions of a rule. "On universes whose DB version supports it" is a
  finding.
- A rule checked on VM universes is also checked on Kubernetes, and a rule
  checked in one direction (node-to-node) is checked in the other
  (client-to-node).
- A new YBA route or field carries a version callout verified on both
  `master` and the current stable branch. The acceptance fixture runs a
  preview build; a long-tier pass proves nothing about stable.
- Two tasks in one apply where the second repeats the first's work get a doc
  warning about the second rolling restart.

## Errors and logging

- An error message adds the variable fact and stops:
  `<what> (<VAR>=<value>): <underlying error>`. Remediation hints and
  topology assumptions belong in a README.
- A log line never carries a value that may be a secret (runtime config
  values, credentials, tokens).
- A `Required` field that is empty at plan time returns a diagnostic. A
  `continue` or silent skip defers the failure to apply with a worse
  message.
- A value the server quotes or re-encodes is normalized before `d.Set`.

## Tests

- Keep only tests that exercise a decision table, a merge rule, a fallback
  path or a guard. Delete field-to-key plumbing, setters, error-string
  echoes, mocked library classes and exact message prefixes. One test per
  rule.
- A fixture pins values the schema accepts. `TestResourceDataRaw` bypasses
  `ValidateFunc`, so an invalid fixture passes and pins behaviour users
  cannot reach.
- Every substring marker that detects a server condition has a unit test, so
  a YBA wording change fails a test instead of silently clearing state.
- The test covers the claim in the commit message. "Reconciles drift" needs
  the case where state holds A and the server returns B, not only
  populate-from-empty and clear-on-empty.
- A known platform bug is a `t.Skip` with the ticket in a short comment, next
  to the test it blocks.
- Acceptance resource names come from `acctest.RandomName` only; a raw
  `RandString` skips the branch prefix and the 40-character cap.
  `resource.ParallelTest` targets a per-cloud YBA; `resource.Test` targets
  the shared YBA.

## Public docs

- Nothing published from this repo (`Description` strings, `docs/`,
  `templates/`, `examples/`) names an internal ticket or warns that a feature
  is broken. Docs describe supported behaviour.
- Explanations use the identifiers from the YBA source, never imported
  vocabulary. An external term may appear once, as an alias.
- Template front matter takes the first paragraph only:
  `index (split (trimspace .Description) "\n\n") 0`. A full `.Description`
  there puts every callout into the registry summary.

## Match the neighbours

The codebase splits on some styles. Where it does, match the form named here;
a third form is a finding.

- A preview or experimental YBA API gets an admonition constant prefixed to
  the resource `Description`, plus a `diag.Warning` on Create and Update. A
  bare `~> **Note:**` is a finding.
- Create and Update end with `return resource<X>Read(ctx, d, meta)`. A Create
  that returns `nil` after `d.SetId` leaves computed fields unset.
- An Update that writes credentials uses a named `diags` return, a deferred
  Read, and `utils.RevertFields` before every error return.
- Every resource has one `Test<X>Guardrails` test: importer, every
  `Description`, no `customer_uuid`, the timeout constant. An importable
  resource's acceptance test has an import step with `ImportStateVerify`.

## Where to look harder, and what to skip

- `internal/utils/`: every resource's Read and Delete route through it. A
  change to `IsHTTPNotFound`, `ErrorFromHTTPResponse` or the task helpers
  changes every resource.
- `internal/universe/`: every mutation is a YBA task and most are rolling
  restarts. Check task ordering, the 409 retry, and what a failed step leaves
  in state.
- `internal/installation/`: `yba_installer` acts on a live host over SSH. A
  wrong verdict in `Read` reinstalls or upgrades YBA.
- Skip `docs/`: `make documents` generates it from `templates/` and the
  schema. Review the template and the `Description` instead. Skip `go.sum`.
