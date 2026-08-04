#!/usr/bin/env bash
# Issue one policy-approved Cosmos feegrant and record it only after the chain
# confirms code=0. Identity verification and nonce signing happen upstream;
# this command accepts only the resulting salted identity hash.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="${IGIT_FEEGRANT_GATE:-$ROOT/scripts/feegrant-policy-gate.sh}"
STATE="${IGIT_FEEGRANT_STATE:-/var/lib/igit-feegrant/state.tsv}"
INJECTIVED_BIN="${INJECTIVED_BIN:-injectived}"
ADDRESS="${1:?usage: feegrant-issue.sh <inj-address> <identity-hash>}"
IDENTITY_HASH="${2:?usage: feegrant-issue.sh <inj-address> <identity-hash>}"

GRANTER="${IGIT_FEEGRANT_GRANTER:?set IGIT_FEEGRANT_GRANTER to the treasury key name or address}"
CHAIN_ID="${IGIT_FEEGRANT_CHAIN_ID:?set IGIT_FEEGRANT_CHAIN_ID}"
NODE="${IGIT_FEEGRANT_NODE:?set IGIT_FEEGRANT_NODE}"
KEYRING_BACKEND="${IGIT_FEEGRANT_KEYRING_BACKEND:-file}"
GAS_PRICES="${IGIT_FEEGRANT_GAS_PRICES:-500000000inj}"
SPEND_LIMIT="${IGIT_FEEGRANT_SPEND_LIMIT:-30000000000000000inj}"
ALLOWED_MESSAGES="${IGIT_FEEGRANT_ALLOWED_MESSAGES:-/cosmwasm.wasm.v1.MsgExecuteContract}"
MAX_SPEND_LIMIT=30000000000000000

command -v "$INJECTIVED_BIN" >/dev/null 2>&1 || {
  echo "feegrant issuer: injectived not found: $INJECTIVED_BIN" >&2
  exit 1
}
command -v jq >/dev/null 2>&1 || {
  echo "feegrant issuer: jq is required" >&2
  exit 1
}
command -v flock >/dev/null 2>&1 || {
  echo "feegrant issuer: flock is required" >&2
  exit 1
}
[[ "$SPEND_LIMIT" =~ ^([0-9]+)inj$ ]] || {
  echo "feegrant issuer: spend limit must be an integer amount in inj" >&2
  exit 1
}
(( 10#${BASH_REMATCH[1]} <= MAX_SPEND_LIMIT )) || {
  echo "feegrant issuer: spend limit exceeds the 0.03 INJ policy cap" >&2
  exit 1
}
[[ "$ALLOWED_MESSAGES" == "/cosmwasm.wasm.v1.MsgExecuteContract" ]] || {
  echo "feegrant issuer: allowed message policy must remain MsgExecuteContract" >&2
  exit 1
}

mkdir -p "$(dirname "$STATE")"
chmod 700 "$(dirname "$STATE")"
exec 9>"$STATE.issue.lock"
flock 9

now="$(date +%s)"
ttl="${IGIT_FEEGRANT_TTL_SECONDS:-604800}"
[[ "$ttl" =~ ^[1-9][0-9]*$ ]] || {
  echo "feegrant issuer: TTL must be a positive integer" >&2
  exit 1
}
expires=$((now + ttl))
expiration_iso="$(date -u -d "@$expires" '+%Y-%m-%dT%H:%M:%SZ')"

# The issue lock serializes check -> broadcast -> record across all issuer
# workers. Without it, two workers could both pass check before either grant is
# recorded.
"$GATE" check "$ADDRESS" "$IDENTITY_HASH" "$now" >/dev/null

if ! tx_json=$("$INJECTIVED_BIN" tx feegrant grant "$GRANTER" "$ADDRESS" \
    --spend-limit "$SPEND_LIMIT" \
    --expiration "$expiration_iso" \
    --allowed-messages "$ALLOWED_MESSAGES" \
    --chain-id "$CHAIN_ID" \
    --node "$NODE" \
    --keyring-backend "$KEYRING_BACKEND" \
    --gas auto \
    --gas-adjustment 1.4 \
    --gas-prices "$GAS_PRICES" \
    --broadcast-mode sync \
    --output json \
    --yes); then
  echo "feegrant issuer: injectived command failed; state was not updated" >&2
  exit 1
fi

if ! printf '%s\n' "$tx_json" | jq -e '((.code // 0) | tonumber) == 0' >/dev/null; then
  echo "feegrant issuer: chain transaction failed; state was not updated" >&2
  printf '%s\n' "$tx_json" >&2
  exit 1
fi

tx_hash="$(printf '%s\n' "$tx_json" | jq -er '.txhash // empty' | tr '[:upper:]' '[:lower:]')"
[[ "$tx_hash" =~ ^[[:xdigit:]]{64}$ ]] || {
  echo "feegrant issuer: successful transaction did not return a 64-hex txhash" >&2
  printf '%s\n' "$tx_json" >&2
  exit 1
}

"$GATE" record-grant "$ADDRESS" "$IDENTITY_HASH" "$tx_hash" "$expires" "$now" >/dev/null
printf 'feegrant issued: address=%s expires_at=%s tx=%s\n' "$ADDRESS" "$expires" "$tx_hash"
