#!/usr/bin/env bash
# Validate replication service configuration before a systemd cutover.
# This script never prints secret values.
set -euo pipefail

mode=${1:?usage: replication-config-check.sh <staged|reaper> ENV_FILE}
env_file=${2:?usage: replication-config-check.sh <staged|reaper> ENV_FILE}

case "$mode" in
  staged|reaper) ;;
  *) echo "mode must be staged or reaper" >&2; exit 2 ;;
esac
test -f "$env_file" || { echo "missing environment file: $env_file" >&2; exit 1; }

value() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key { value = substr($0, index($0, "=") + 1); count++ } END { if (count != 1) exit 1; print value }' "$env_file"
}

require_value() {
  local key="$1" v
  v="$(value "$key")" || { echo "missing or duplicate $key" >&2; exit 1; }
  test -n "$v" || { echo "$key must not be empty" >&2; exit 1; }
  printf '%s' "$v"
}

secret="$(require_value IGIT_REPLICATION_JWT_HMAC)"
if [[ ${#secret} -lt 32 || "$secret" == *replace-with* ]]; then
  echo 'IGIT_REPLICATION_JWT_HMAC must be a private secret of at least 32 characters' >&2
  exit 1
fi

for key in IGIT_REPLICATION_MAX_BYTES IGIT_REPLICATION_RATE_PER_MINUTE IGIT_REPLICATION_BYTES_PER_MINUTE; do
  v="$(require_value "$key")"
  [[ "$v" =~ ^[1-9][0-9]*$ ]] || { echo "$key must be a positive integer" >&2; exit 1; }
done

if [[ "$mode" == reaper ]]; then
  chain_id="$(require_value CHAIN_ID)"
  [[ "$chain_id" == injective-1 ]] || {
    echo "CHAIN_ID must be injective-1 when enabling the TTL reaper" >&2
    exit 1
  }

  contract="$(require_value CONTRACT)"
  [[ "$contract" =~ ^inj1[0-9a-z]{20,}$ && "$contract" != *replace_me* ]] || {
    echo 'CONTRACT must be a real Injective mainnet contract address' >&2
    exit 1
  }

  lcd="$(require_value LCD)"
  [[ "$lcd" == https://* ]] || { echo 'LCD must use HTTPS' >&2; exit 1; }
  lower_lcd="${lcd,,}"
  case "$lower_lcd" in
    *testnet*|*localhost*|*127.0.0.1*)
      echo 'LCD must point to an Injective mainnet endpoint when enabling the TTL reaper' >&2
      exit 1
      ;;
  esac
fi

echo "replication config check: $mode configuration is valid"
