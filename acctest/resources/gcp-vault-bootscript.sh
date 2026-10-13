#!/bin/bash
# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# GCP startup script of the Vault VM (acctest/gcp/vault.tf), rendered with
# templatefile: a dev-mode Vault server for the yba_hashicorp_vault_ear_config
# acceptance test. Dev mode keeps all data in memory, so vault-configure runs
# after every start of the server and creates the same transit key, policy,
# token and AppRole again.

set -euxo pipefail

if [ ! -x /usr/local/bin/vault ]; then
  apt-get update
  apt-get install -y curl unzip
  curl -fsSL -o /tmp/vault.zip \
    "https://releases.hashicorp.com/vault/${vault_version}/vault_${vault_version}_linux_amd64.zip"
  unzip -o /tmp/vault.zip -d /usr/local/bin
fi

# The policy covers the transit mount, as the sample policy in the YugabyteDB
# Anywhere docs does. YBA also checks the credentials with a transit key named
# AES (HashicorpEARService.createAuthConfigWithService passes the algorithm as
# the key name), so a policy scoped to the configured key fails. The default
# policy, which tokens and AppRole logins also get, allows the token lookup
# and renewal.
cat > /usr/local/bin/vault-configure <<'EOF'
#!/bin/bash
set -euo pipefail
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN='${root_token}'
until vault status >/dev/null 2>&1; do sleep 1; done

vault secrets enable -path='${mount_path}' transit
vault write -f '${mount_path}keys/${key_name}'
vault policy write yba - <<'POLICY'
path "${mount_path}*" {
  capabilities = ["create", "read", "update", "list"]
}
POLICY

vault token create -id='${token}' -policy=yba -period=768h
vault auth enable approle
vault write auth/approle/role/yba token_policies=yba secret_id_num_uses=0 secret_id_ttl=0
vault write auth/approle/role/yba/role-id role_id='${role_id}'
vault write auth/approle/role/yba/custom-secret-id secret_id='${secret_id}'
EOF
chmod 0700 /usr/local/bin/vault-configure

cat > /etc/systemd/system/vault-dev.service <<'EOF'
[Unit]
Description=Vault dev server for the YBA acceptance tests
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/vault server -dev -dev-listen-address=0.0.0.0:8200 -dev-root-token-id=${root_token}
ExecStartPost=/usr/local/bin/vault-configure
Restart=always

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable vault-dev
systemctl restart vault-dev
