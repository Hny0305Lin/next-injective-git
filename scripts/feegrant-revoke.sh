#!/usr/bin/env bash
# Revoke a policy grant on chain and record the revoke only after code=0.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="${IGIT_FEEGRANT_GATE:-$ROOT/scripts/feegrant-policy-gate.sh}"
STATE="${IGIT_FEEGRANT_STATE:-/var/lib/igit-feegrant/state.tsv}"
INJECTIVED_BIN="${INJECTIVED_BIN:-injectived}"
ADDRESS="${1:?usage: feegrant-revoke.sh <inj-address>}"

GRANTER="${IGIT_FEEGRANT_GRANTER:?set IGIT_FEEGRANT_GRANTER to the treasury key name or address}"
CHAIN_ID="${IGIT_FEEGRANT_CHAIN_ID:?set IGIT_FEEGRANT_CHAIN_ID}"
NODE="${IGIT_FEEGRANT_NODE:?set IGIT_FEEGRANT_NODE}"
KEYRING_BACKEND="${IGIT_FEEGRANT_KEYRING_BACKEND:-file}"
GAS_PRICES="${IGIT_FEEGRANT_GAS_PRICES:-500000000inj}"

command -v "$INJECTIVED_BIN" >/dev/null 2>&1 || {
  echo "feegrant revoke: injectived not found: $INJECTIVED_BIN" >&2
  exit 1
}
command -v jq >/dev/null 2>&1 || {
  echo "feegrant revoke: jq is required" >&2
  exit 1
}
command -v flock >/dev/null 2>&1 || {
  echo "feegrant revoke: flock is required" >&2
  exit 1
}

mkdir -p "$(dirname "$STATE")"
chmod 700 "$(dirname "$STATE")"
exec 9>"$STATE.issue.lock"
flock 9

now="$(date +%s)"
status_output="$(bash "$GATE" status "$ADDRESS" "$now")"
if grep -Fq 'status=none' <<<"$status_output"; then
  echo "feegrant revoke: no recorded grant for $ADDRESS" >&2
  exit 1
fi
if grep -Fq 'status=revoked' <<<"$status_output"; then
  echo "feegrant revoke: grant already revoked for $ADDRESS"
  exit 0
fi

if ! tx_json=$("$INJECTIVED_BIN" tx feegrant revoke "$GRANTER" "$ADDRESS" \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --keyring-backend "$KEYRING_BACKEND" \
    --gas auto \
    --gas-adjustment 1.4 \
    --gas-prices "$GAS_PRICES" \
    --broadcast-mode sync \
    --output json \
    --yes); then
  echo "feegrant revoke: injectived command failed; state was not updated" >&2
  exit 1
fi

if ! printf '%s\n' "$tx_json" | jq -e '((.code // 0) | tonumber) == 0' >/dev/null; then
  echo "feegrant revoke: chain transaction failed; state was not updated" >&2
  printf '%s\n' "$tx_json" >&2
  exit 1
fi

tx_hash="$(printf '%s\n' "$tx_json" | jq -er '.txhash // empty' | tr '[:upper:]' '[:lower:]')"
[[ "$tx_hash" =~ ^[[:xdigit:]]{64}$ ]] || {
  echo "feegrant revoke: successful transaction did not return a 64-hex txhash" >&2
  printf '%s\n' "$tx_json" >&2
  exit 1
}

bash "$GATE" record-revoke "$ADDRESS" "$tx_hash" "$now" >/dev/null
printf 'feegrant revoked: address=%s tx=%s\n' "$ADDRESS" "$tx_hash"
