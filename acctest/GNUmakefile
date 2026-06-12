# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Per-cloud acceptance-test fixtures, run from this directory:
#   make apply-gcp     # apply the <cloud> fixture (VPC, IAM, YBA VM + install)
#   make env           # write the test env from fixture state
#   make destroy-gcp   # destroy the fixture
# The repo-root `make acctest` runs the test suite against these.

# Build and install the dev yba provider (delegates to the repo-root Makefile).
# The fixtures install YBA through it, so terraform needs it on PATH first.
.PHONY: install
install:
	@$(MAKE) -C .. install

# Log in to GCP and Azure for the byoc-dev project (idempotent).
.PHONY: auth
auth:
	@./auth.sh

# Keep terraform's cached state/lock around between runs (don't auto-delete).
.PRECIOUS: %/.terraform %/.terraform/terraform.tfstate

# Create the per-cloud terraform working directory (0700, it holds state cache).
%/.terraform:
	install -d -m 700 $@

# Run `terraform init` for a cloud, cached: re-runs only when providers.tf
# changes (not the lock file, which init itself rewrites).
%/.terraform/terraform.tfstate: %/providers.tf | %/.terraform install
	@echo "Initializing $* fixture"
	terraform -chdir=$* init -input=false -reconfigure

# Convenience alias: `make init-gcp` ensures the gcp working dir is initialized.
init-%: %/.terraform/terraform.tfstate ;

# Apply a cloud fixture: VPC, IAM, the YBA VM and its install. Run once, then
# reuse across many test runs. State lives in GCS (gs://tf-acctest-tfstate).
apply-%: init-%
	@echo "Applying $* fixture (creates real cloud + YBA resources)"
	terraform -chdir=$* apply -auto-approve

# Tear down a cloud fixture and everything it created.
destroy-%: init-%
	@echo "Destroying $* fixture"
	terraform -chdir=$* destroy # -auto-approve

# Write the test env (TF_VAR_*, plus YBA_HOST/YBA_API_KEY for the shared client)
# into the gitignored, 0600 `env` file. Concatenates each fixture's test_env
# output: gcp supplies the standing YBA endpoint and TF_VAR_GCP_*; azure supplies
# TF_VAR_AZURE_*. The whole suite runs against the single gcp-hosted YBA.
env: auth init-gcp init-azure
	@echo "Generating '$@' file from the Terraform state"
	@umask 077; : > $@; \
	for c in gcp azure; do \
		terraform -chdir=$$c output -raw test_env >> $@; \
		printf '\n' >> $@; \
	done
	@echo "Generated '$@' file"

# Publish the env to a single GitHub Actions secret (ACCTEST_ENV) for CI.
.PHONY: push-github-secrets
push-github-secrets: env
	@echo "Pushing env to GitHub secret ACCTEST_ENV"
	gh secret set ACCTEST_ENV < env
