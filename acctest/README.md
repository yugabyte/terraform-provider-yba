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

The standing YBA VM has **no direct internet ingress** — every connection
(UI, API, SSH) goes through an [IAP TCP forwarding](https://cloud.google.com/iap/docs/using-tcp-forwarding)
tunnel, which authenticates callers with IAM before any packet reaches the VM.
The env's YBA endpoints are therefore `127.0.0.1:9443`, and `make acctest`
opens the tunnel itself (`acctest/with-yba-tunnel.sh`, the same path CI and
the fixture targets use) as the fixture service account — no personal IAP
grant or extra terminal needed. To browse the UI, hold a tunnel open with:

```bash
acctest/with-yba-tunnel.sh sleep 86400   # then open https://127.0.0.1:9443
```

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

- **Auth error** (`could not find default credentials`, `Reauthentication
  failed`, `no usable gcloud login`, `403`): re-run `make -C acctest auth` —
  it detects expired sessions, not just missing ones.
- **`IAP tunnel ... did not become ready`** (or `connection refused` to
  `127.0.0.1:9443`): check `gcloud` is installed and the env is fresh — the
  tunnel authenticates as the fixture SA from `TF_VAR_GCP_CREDENTIALS`, so a
  rotated key means a stale env (see below).
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

For **gcp**, `apply-gcp` and `destroy-gcp` open their own IAP tunnels (API plus
an SSH leg for the YBA install and destroy-time `yba-ctl clean` — the VM
accepts nothing else). This works even on a first-ever apply: the tunnel legs
retry until the VM exists and the installer waits for SSH, so one `apply-gcp`
bootstraps end to end. Since the fixture SA key may not exist yet, these run
as *your* gcloud login, which needs `roles/iap.tunnelResourceAccessor` (or
project owner) on `byoc-dev`.

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

CI reads the env from the `ACCTEST_ENV` GitHub Actions secret (written to
`acctest/env`, then sourced like a local run). There are no CI-specific tunnel
steps: `make acctest` opens the IAP tunnel the same way it does locally, as the
fixture service account inside that env. Publish/refresh the secret after
re-applying a fixture:

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
