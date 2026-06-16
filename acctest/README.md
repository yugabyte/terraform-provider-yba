<!--
Copyright 2026 YugabyteDB, Inc.
SPDX-License-Identifier: MPL-2.0
-->

# Acceptance tests

The acceptance tests (`TestAcc*`) run against a **standing** YugabyteDB Anywhere
(YBA) and real cloud resources — the "fixture". The fixture is applied once and
left running. Tests create and destroy resources through YBA against it.

You almost never touch the fixture. The normal job is just: **log in, run tests.**

## Quick start (run the tests)

From the repo root:

```bash
make -C acctest auth   # 1. log in to GCP, Azure and AWS (browser, first time only)
make acctest           # 2. build the provider, fetch the env, run the suite
```

That's it. Step 2 automatically writes `acctest/env` from the standing fixture's
Terraform state the first time, then runs every `TestAcc*` (skipping the long
tier, `TestAccLong*`). Re-run `make acctest` as often as you like. To run the
long tier (multi-node universe deploys, ~15 min each): `make acctest-long`.

If you see `acctest/env doesn't exist - extracting from TF state`, that is normal
on the first run.

## Prerequisites

Install these CLIs and be a member of the `byoc-dev` GCP access groups:

- `terraform`, `go`, `make`
- `gcloud` (GCP login, done by `make -C acctest auth`)
- `gh` (only needed for `push-github-secrets`)

`make -C acctest auth` is **idempotent** — running it again when you are already
logged in does nothing. The agent cannot run these logins for you (they are
interactive), so run it yourself.

## Troubleshooting

- **Auth error** (`could not find default credentials`, `403`): re-run
  `make -C acctest auth`.
- **Every test is SKIPPED**: the env was not loaded. Run `make -C acctest env`,
  confirm `acctest/env` exists, then `make acctest`.
- **Env looks stale** (after re-applying a fixture, or rotated keys): delete it
  and let it regenerate — `rm acctest/env && make acctest`.
- **Run a single test**:
  ```bash
  set -a && . ./acctest/env && set +a
  TF_ACC=1 go test -v -run TestAccGCPProvider_WithCredentials ./internal/provider/gcp/
  ```

## Managing the fixture (rare)

Only needed to first create the fixture, or to recreate/destroy it. It is slow
and creates real billable resources.

```bash
make -C acctest apply-gcp     # create or update a fixture (gcp / azure / aws)
make -C acctest destroy-gcp   # tear it down
```

Each cloud has its own fixture target (`apply-gcp`/`apply-azure`/`apply-aws` and
the matching `destroy-*`). The AWS fixture authenticates with the `byoc-dev` AWS
profile by default (override with the `aws_profile` var); the GCP and Azure ones
use the active `gcloud`/`az` login from `make -C acctest auth`.

Applying installs YBA via `yba_installer`, which needs a **YBA license file** at
the repo root named `yugabyte_anywhere.lic` (gitignored — it is a secret, kept
out of the repo). Obtain it out of band and drop it there before `apply-gcp`;
`terraform` validates the path at plan time and fails fast if it is missing.
This file is only needed to deploy YBA — CI never uses it, since the suite runs
against the already-installed standing YBA.

## CI

CI has no cloud access. It reads the env from the `ACCTEST_ENV` GitHub Actions
secret (written to `acctest/env`, then sourced like a local run). Publish/refresh
that secret after re-applying a fixture:

```bash
make -C acctest push-github-secrets
```

## Layout

| Path                 | What                                                          |
| -------------------- | ------------------------------------------------------------- |
| `gcp/`               | GCP fixture: VPC, IAM, a YBA VM + install, a backups bucket.   |
| `azure/`             | Azure fixture: RG, VNet, service principal, a YBA VM + install, a backups account. |
| `aws/`               | AWS fixture: VPC, IAM (key user + instance role), a YBA VM + install, a backups bucket. |
| `resources/`         | Shared install assets (`yba-ctl.yml`, VM startup scripts).    |
| `auth.sh`            | Logs in to GCP, Azure and AWS for byoc-dev.                   |
| `GNUmakefile`        | The fixture and env targets.                                  |

## Resource naming

Tests name resources `<prefix>-<kind>-<random>` so parallel runs do not collide.
The prefix is `TF_ACCTEST_PREFIX` if set, otherwise `acc-<branch>` using the
branch's last path segment (`user/feature` becomes `feature`). The branch comes
from `GITHUB_HEAD_REF` / `GITHUB_REF_NAME` in CI, else the local git branch.
