#!/usr/bin/env bash
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Authenticate the shell for acceptance tests against the byoc-dev project.
# Idempotent: interactive (browser/SSO) only on first run, a no-op once
# authenticated. Run by the operator — the agent cannot do the logins.
set -euo pipefail

GCP_PROJECT="byoc-dev"

echo 'Authenticating to GCP'

echo "==> GCP (project: $GCP_PROJECT)"
active="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null | head -1 || true)"
if printf '%s' "$active" | grep -q '@yugabyte.com'; then
	echo "    signed in as $active"
else
	echo "    no active @yugabyte.com account — opening browser login"
	gcloud auth login --quiet
fi
if [ "$(gcloud config get-value project 2>/dev/null)" != "$GCP_PROJECT" ]; then
	echo "    setting default project to $GCP_PROJECT"
	gcloud config set project "$GCP_PROJECT" >/dev/null
fi
adc_file="$HOME/.config/gcloud/application_default_credentials.json"
if [ ! -e "$adc_file" ]; then
	echo "    no Application Default Credentials — opening ADC login (terraform uses these)"
	gcloud auth application-default login
fi
# Align the ADC quota project with the active project (silences gcloud's
# "quota project does not match" warning).
if ! grep -q "\"quota_project_id\": \"$GCP_PROJECT\"" "$adc_file" 2>/dev/null; then
	echo "    aligning ADC quota project to $GCP_PROJECT"
	gcloud auth application-default set-quota-project "$GCP_PROJECT" >/dev/null
fi
echo "    OK: account=$(gcloud config get-value account 2>/dev/null), project=$(gcloud config get-value project 2>/dev/null)"

echo "GCP authenticated"
