#!/usr/bin/env bash
# Announce a contract Wasm hash. The contract enforces a 14-day delay before
# the corresponding migrate transaction can execute.
set -euo pipefail

CONTRACT="${CONTRACT:?set CONTRACT}"
WASM_SHA256="${WASM_SHA256:?set WASM_SHA256}"
FROM="${FROM:?set FROM key name}"
CHAIN_ID="${CHAIN_ID:-injective-1}"
NODE="${NODE:?set NODE LCD/RPC endpoint}"
KEYRING_BACKEND="${KEYRING_BACKEND:-file}"
GAS_PRICES="${GAS_PRICES:-500000000inj}"

[[ "$CHAIN_ID" == injective-1 ]] || { echo 'schedule-upgrade is mainnet-only: CHAIN_ID must be injective-1' >&2; exit 2; }
[[ "$WASM_SHA256" =~ ^[[:xdigit:]]{64}$ ]] || { echo 'WASM_SHA256 must be 64 hexadecimal characters' >&2; exit 2; }

MSG="$(jq -cn --arg sha "${WASM_SHA256,,}" '{schedule_upgrade:{wasm_sha256:$sha}}')"
injectived tx wasm execute "$CONTRACT" "$MSG" \
  --from "$FROM" --chain-id "$CHAIN_ID" --node "$NODE" \
  --keyring-backend "$KEYRING_BACKEND" --gas auto --gas-adjustment 1.4 \
  --gas-prices "$GAS_PRICES" --broadcast-mode sync --output json --yes
