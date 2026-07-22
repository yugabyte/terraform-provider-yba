#!/usr/bin/env bash
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Authenticate the shell for acceptance tests against the byoc-dev project.
# Idempotent: interactive (browser/SSO) only on first run, a no-op once
# authenticated. Run by the operator — the agent cannot do the logins.
set -euo pipefail

GCP_PROJECT="byoc-dev"
AZURE_SUBSCRIPTION="byoc-dev"
AWS_PROFILE_NAME="${AWS_PROFILE:-byoc-dev}"

echo 'Authenticating in clouds'

echo "==> GCP (project: $GCP_PROJECT)"
# print-access-token proves the login is USABLE — a listed account whose
# session has expired under org reauth policy still shows ACTIVE in
# `gcloud auth list` but cannot mint tokens.
active="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null | head -1 || true)"
if printf '%s' "$active" | grep -q '@yugabyte.com' &&
	gcloud auth print-access-token >/dev/null 2>&1; then
	echo "    signed in as $active"
else
	echo "    no usable @yugabyte.com login (absent or expired) — opening browser login"
	gcloud auth login --quiet
fi
if [ "$(gcloud config get-value project 2>/dev/null)" != "$GCP_PROJECT" ]; then
	echo "    setting default project to $GCP_PROJECT"
	gcloud config set project "$GCP_PROJECT" >/dev/null
fi
adc_file="$HOME/.config/gcloud/application_default_credentials.json"
if [ ! -e "$adc_file" ] ||
	! gcloud auth application-default print-access-token >/dev/null 2>&1; then
	echo "    no usable Application Default Credentials — opening ADC login (terraform uses these)"
	gcloud auth application-default login
fi
# Align the ADC quota project with the active project (silences gcloud's
# "quota project does not match" warning).
if ! grep -q "\"quota_project_id\": \"$GCP_PROJECT\"" "$adc_file" 2>/dev/null; then
	echo "    aligning ADC quota project to $GCP_PROJECT"
	gcloud auth application-default set-quota-project "$GCP_PROJECT" >/dev/null
fi
echo "    OK: account=$(gcloud config get-value account 2>/dev/null), project=$(gcloud config get-value project 2>/dev/null)"

echo "==> Azure (subscription: $AZURE_SUBSCRIPTION)"
if az account show >/dev/null 2>&1; then
	echo "    signed in"
else
	echo "    not signed in — opening az login"
	az login >/dev/null
fi
if [ "$(az account show --query name -o tsv 2>/dev/null)" != "$AZURE_SUBSCRIPTION" ]; then
	echo "    switching active subscription to $AZURE_SUBSCRIPTION"
	az account set --subscription "$AZURE_SUBSCRIPTION"
fi
echo "    OK: subscription=$(az account show --query name -o tsv 2>/dev/null) ($(az account show --query id -o tsv 2>/dev/null))"

echo "==> AWS (profile: $AWS_PROFILE_NAME)"
export AWS_PROFILE="$AWS_PROFILE_NAME"
if aws sts get-caller-identity >/dev/null 2>&1; then
	echo "    signed in"
else
	echo "    not signed in — opening aws sso login"
	aws sso login >/dev/null
fi
echo "    OK: account=$(aws sts get-caller-identity --query Account -o text 2>/dev/null), arn=$(aws sts get-caller-identity --query Arn -o text 2>/dev/null)"

echo "All clouds authenticated"
