#!/bin/sh
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# with-yba-tunnel.sh [-s] <command...> — run <command> with a tunnel to the
# standing YBA up (127.0.0.1:9443 -> VM:443; the VM accepts no direct
# ingress). The single owner of the tunnel constants and logic:
#
#   - `make acctest` / `make acctest-long` (CI and local, identically): opens
#     the API tunnel, waits for it, runs the tests, tears down.
#   - `make -C acctest apply-gcp` / `destroy-gcp` pass -s, which adds
#     127.0.0.1:2222 -> VM:22 (the yba_installer's SSH). Ops on an existing
#     stand hit the YBA API within seconds (terraform configures the bootstrap
#     provider during refresh, a one-shot connection), so -s still waits for
#     the API leg — but bounded, warning and proceeding on timeout: on a
#     first-ever apply nothing can answer until terraform has created the VM
#     and installed YBA, and that flow only reaches the API after the install.
#
# The API leg has two shapes. With the fixture SSH key in the env
# (YBA_SSH_PRIVATE_KEY_B64, set for test runs), it is an `ssh -L` forward
# carried over ONE IAP WebSocket (start-iap-tunnel --listen-on-stdin). The
# test suite spawns a fresh provider process per terraform command, each
# dialing new TCP connections; a per-connection IAP forward does a full relay
# handshake for every one, and under the burst when test packages rotate some
# handshakes fail — the client sees resets or, worse, black holes that eat
# the whole 20m go-test timeout. SSH multiplexes each connection as a channel
# on the established transport, so new connections never touch the relay.
# Without the key (fixture ops — on a first apply the keypair does not exist
# yet), the leg falls back to the per-connection forward, which the one-shot
# fixture flows never stress.
#
# Auth: the fixture service account when TF_VAR_GCP_CREDENTIALS is set (test
# runs source acctest/env), inside an isolated CLOUDSDK_CONFIG so the caller's
# gcloud config and active account are never touched and no personal IAP grant
# is needed. Otherwise the ambient gcloud login (fixture ops, where the SA key
# may not exist yet), which needs roles/iap.tunnelResourceAccessor or owner.
#
# Each leg retries in a loop: survives IAP disconnects mid-run and comes up on
# its own once its target exists. If 127.0.0.1:9443 already answers, the test
# path execs straight through; duplicate legs elsewhere just fail to bind and
# idle harmlessly in their loop.
set -eu

# Constants match acctest/gcp/terraform.tfvars (prefix, gcp_project_id,
# gcp_region + "-a").
VM=tf-acctest-yba
PROJECT=byoc-dev
ZONE=us-west1-a

ssh_leg=
if [ "${1:-}" = "-s" ]; then
  ssh_leg=1
  shift
fi

ready() { curl -sk -o /dev/null --max-time 2 https://127.0.0.1:9443/; }

if [ -z "$ssh_leg" ] && ready; then
  exec "$@"
fi

umask 077
tmp=$(mktemp -d)
loop_pids=
cleanup() {
  # Kill each retry loop first (stops respawning), then the tunnel process it
  # recorded (ssh takes its ProxyCommand gcloud down with it).
  for p in $loop_pids; do kill "$p" 2>/dev/null || true; done
  for f in "$tmp"/leg-*.pid; do
    [ -f "$f" ] && kill "$(cat "$f")" 2>/dev/null || true
  done
  rm -rf "$tmp"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

if [ -n "${TF_VAR_GCP_CREDENTIALS:-}" ]; then
  export CLOUDSDK_CONFIG="$tmp/gcloud"
  printf '%s' "$TF_VAR_GCP_CREDENTIALS" >"$tmp/sa.json"
  gcloud auth activate-service-account --key-file="$tmp/sa.json" --quiet
elif ! gcloud auth print-access-token >/dev/null 2>&1; then
  # Ambient path: fail fast with a remedy — an expired login otherwise
  # surfaces later as an opaque connection-refused from the tunnel.
  echo "with-yba-tunnel: no usable gcloud login - run: make -C acctest auth" >&2
  exit 1
fi

leg() { # leg <vm-port> <local-port>
  ( while :; do
      gcloud compute start-iap-tunnel "$VM" "$1" \
        --project="$PROJECT" --zone="$ZONE" \
        --local-host-port=localhost:"$2" >>"$tmp/leg-$1.log" 2>&1 &
      echo $! >"$tmp/leg-$1.pid"
      wait $! || true
      sleep 5
    done ) &
  loop_pids="$loop_pids $!"
}

# The multiplexed API leg (see header). Host-key checking is off: the key file
# is written fresh each run, IAP already authenticates the endpoint by IAM,
# and the API traffic inside is TLS anyway. ServerAlive* makes ssh exit when
# the relay goes quiet, so the retry loop replaces it within seconds;
# ExitOnForwardFailure covers the duplicate-listener case the same way a
# failed gcloud bind does.
ssh_mux_leg() {
  printf '%s' "$YBA_SSH_PRIVATE_KEY_B64" | base64 -d >"$tmp/ssh-key"
  ( while :; do
      ssh -N \
        -i "$tmp/ssh-key" \
        -o ProxyCommand="gcloud compute start-iap-tunnel $VM 22 --listen-on-stdin --project=$PROJECT --zone=$ZONE" \
        -o BatchMode=yes -o IdentitiesOnly=yes -o ConnectTimeout=30 \
        -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
        -o ExitOnForwardFailure=yes \
        -o ServerAliveInterval=15 -o ServerAliveCountMax=3 \
        -L 127.0.0.1:9443:127.0.0.1:443 \
        "${YBA_SSH_USER:-yugabyte}@$VM" >>"$tmp/leg-443.log" 2>&1 &
      echo $! >"$tmp/leg-443.pid"
      wait $! || true
      sleep 5
    done ) &
  loop_pids="$loop_pids $!"
}

if [ -n "${YBA_SSH_PRIVATE_KEY_B64:-}" ]; then
  ssh_mux_leg
else
  leg 443 9443
fi
if [ -n "$ssh_leg" ]; then
  leg 22 2222
  for _ in $(seq 1 12); do
    if ready; then break; fi
    sleep 5
  done
  if ! ready; then
    echo "with-yba-tunnel: 127.0.0.1:9443 not answering (fresh bootstrap?); proceeding" >&2
  fi
  rc=0
  "$@" || rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "with-yba-tunnel: command failed (rc=$rc); tunnel logs:" >&2
    cat "$tmp"/leg-*.log >&2 || true
  fi
  exit "$rc"
fi

for _ in $(seq 1 30); do
  if ready; then
    rc=0
    "$@" || rc=$?
    if [ "$rc" -ne 0 ]; then
      echo "with-yba-tunnel: command failed (rc=$rc); tunnel logs:" >&2
      cat "$tmp"/leg-*.log >&2 || true
    fi
    exit "$rc"
  fi
  sleep 2
done

echo "tunnel to the standing YBA did not become ready:" >&2
cat "$tmp"/leg-*.log >&2
exit 1
