#!/usr/bin/env bash
# Record one successful fee-sponsored update_ref only after querying the chain.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="${IGIT_FEEGRANT_GATE:-$ROOT/scripts/feegrant-policy-gate.sh}"
STATE="${IGIT_FEEGRANT_STATE:-/var/lib/igit-feegrant/state.tsv}"
INJECTIVED_BIN="${INJECTIVED_BIN:-injectived}"
ADDRESS="${1:?usage: feegrant-record-push.sh <inj-address> <tx-hash>}"
TX_HASH="${2:?usage: feegrant-record-push.sh <inj-address> <tx-hash>}"
CHAIN_ID="${IGIT_FEEGRANT_CHAIN_ID:?set IGIT_FEEGRANT_CHAIN_ID}"
NODE="${IGIT_FEEGRANT_NODE:?set IGIT_FEEGRANT_NODE}"
KEYRING_BACKEND="${IGIT_FEEGRANT_KEYRING_BACKEND:-file}"

[[ "$ADDRESS" =~ ^inj1[0-9a-z]{38}$ ]] || { echo 'feegrant record: invalid inj address' >&2; exit 2; }
[[ "$TX_HASH" =~ ^[[:xdigit:]]{64}$ ]] || { echo 'feegrant record: tx hash must be 64 hex characters' >&2; exit 2; }
command -v "$INJECTIVED_BIN" >/dev/null 2>&1 || { echo "feegrant record: injectived not found: $INJECTIVED_BIN" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo 'feegrant record: jq is required' >&2; exit 1; }

tx_json="$("$INJECTIVED_BIN" query tx "$TX_HASH" \
  --chain-id "$CHAIN_ID" --node "$NODE" --keyring-backend "$KEYRING_BACKEND" --output json)" || {
  echo 'feegrant record: failed to query transaction' >&2
  exit 1
}

printf '%s\n' "$tx_json" | jq -e '
  ((.tx_response.code // .code // 1) | tonumber) == 0
  and ([((.tx_response.tx.body.messages // .tx.body.messages // [])[]?)
        | select((."@type" // "") == "/cosmwasm.wasm.v1.MsgExecuteContract")
        | (if (.msg | type) == "string" then (try (.msg | fromjson) catch null) else .msg end)
        | select(type == "object" and (.update_ref | type) == "object")
       ] | length) == 1
' >/dev/null || {
  echo 'feegrant record: transaction is not a successful update_ref execute' >&2
  exit 1
}

"$GATE" record-push "$ADDRESS" "${TX_HASH,,}" >/dev/null
printf 'feegrant push recorded: address=%s tx=%s\n' "$ADDRESS" "${TX_HASH^^}"
