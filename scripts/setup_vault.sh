#!/usr/bin/env bash
#
# Registers clickvault with a running dev Vault, configures a connection to
# ClickHouse, and creates a dynamic + a static role.
#
# Required env vars:
#   VAULT_ADDR, VAULT_TOKEN                 - as consumed by the vault CLI
#   CLICKHOUSE_VAULT_ADMIN_PASSWORD         - password for the Vault admin user in ClickHouse
#
# Optional env vars:
#   CLICKHOUSE_CONNECTION_URL   (default: clickhouse://clickhouse:9000)
#   CLICKHOUSE_VAULT_ADMIN_USER (default: vault_admin)
#   CLICKVAULT_DB_NAME          (default: pos-clickhouse)
#   CLICKVAULT_PLUGIN_DIR       (default: $(vault plugin list -format=json 2>/dev/null | ... ) - see `vault plugin register` docs)

set -euo pipefail

: "${VAULT_ADDR:?VAULT_ADDR must be set}"
: "${VAULT_TOKEN:?VAULT_TOKEN must be set}"
: "${CLICKHOUSE_VAULT_ADMIN_PASSWORD:?CLICKHOUSE_VAULT_ADMIN_PASSWORD must be set}"

CLICKHOUSE_CONNECTION_URL="${CLICKHOUSE_CONNECTION_URL:-clickhouse://clickhouse:9000}"
CLICKHOUSE_VAULT_ADMIN_USER="${CLICKHOUSE_VAULT_ADMIN_USER:-vault_admin}"
CLICKVAULT_DB_NAME="${CLICKVAULT_DB_NAME:-pos-clickhouse}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "==> Building clickvault"
make -C "${REPO_ROOT}" build-linux-amd64

SHA256="$(sha256sum "${REPO_ROOT}/bin/clickvault-linux-amd64" | awk '{print $1}')"
echo "==> sha256: ${SHA256}"

echo "==> Enabling the database secrets engine (no-op if already enabled)"
vault secrets enable database 2>/dev/null || true

echo "==> Registering clickvault plugin"
vault plugin register \
  -sha256="${SHA256}" \
  database clickvault

echo "==> Creating clickhouse-password-policy"
vault write sys/policies/password/clickhouse-password-policy policy=-<<'EOF'
length = 20
rule "charset" {
  charset = "abcdefghijklmnopqrstuvwxyz"
}
rule "charset" {
  charset    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
  min-chars  = 1
}
rule "charset" {
  charset    = "0123456789"
  min-chars  = 1
}
EOF

echo "==> Configuring connection ${CLICKVAULT_DB_NAME}"
vault write "database/config/${CLICKVAULT_DB_NAME}" \
  plugin_name=clickvault \
  connection_url="${CLICKHOUSE_CONNECTION_URL}" \
  username="${CLICKHOUSE_VAULT_ADMIN_USER}" \
  password="${CLICKHOUSE_VAULT_ADMIN_PASSWORD}" \
  cluster=""

echo "==> Creating dynamic role pos-analytics-dynamic"
vault write "database/roles/pos-analytics-dynamic" \
  db_name="${CLICKVAULT_DB_NAME}" \
  creation_statements='CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '"'"'{{password}}'"'"'; GRANT analytics ON default.* TO "{{username}}";' \
  default_ttl="24h" \
  max_ttl="48h" \
  password_policy="clickhouse-password-policy"

echo "==> Creating static role pos-service-account"
vault write "database/static-roles/pos-service-account" \
  db_name="${CLICKVAULT_DB_NAME}" \
  username="pos_service" \
  rotation_period="72h" \
  password_policy="clickhouse-password-policy"

echo "==> Done"
