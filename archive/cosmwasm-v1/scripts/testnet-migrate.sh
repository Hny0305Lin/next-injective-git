#!/usr/bin/env bash
# In-place contract migration: point the existing instance at a new code_id.
# The current contract must have a matching `schedule_upgrade` proposal whose
# 14-day delay has expired before this command can succeed.
# Usage: testnet-migrate.sh <store_txhash> <wasm_sha256>
set -euo pipefail
LCD="https://testnet.sentry.lcd.injective.network:443"
NODE="https://testnet.sentry.tm.injective.network:443"
CONTRACT=$(jq -r .contract_address /root/.igit/config.json)
TXHASH="${1:?usage: testnet-migrate.sh <store_txhash> <wasm_sha256>}"
WASM_SHA256="${2:?usage: testnet-migrate.sh <store_txhash> <wasm_sha256>}"
[[ "$WASM_SHA256" =~ ^[[:xdigit:]]{64}$ ]] || {
  echo 'wasm_sha256 must be 64 hexadecimal characters' >&2
  exit 2
}

curl -s "${LCD}/cosmos/tx/v1beta1/txs/${TXHASH}" > /tmp/store_tx.json
CODE_ID=$(jq -r '[.tx_response.events[] | select(.type | test("CodeStored|store_code"))
  | .attributes[] | select(.key == "code_id") | .value][0]' /tmp/store_tx.json | tr -d '"')
echo "migrating ${CONTRACT} -> code_id ${CODE_ID}"

MIGRATE_MSG=$(jq -cn --arg sha "${WASM_SHA256,,}" '{wasm_sha256:$sha}')
OUT=$(injectived tx wasm migrate "${CONTRACT}" "${CODE_ID}" "${MIGRATE_MSG}" \
  --from igit-dev --chain-id injective-888 --node "${NODE}" \
  --keyring-backend test --gas auto --gas-adjustment 1.4 \
  --gas-prices 500000000inj --broadcast-mode sync --output json --yes 2>&1 | tail -1)
echo "${OUT}" | jq -r '{txhash, code, raw_log}'
MTX=$(echo "${OUT}" | jq -r .txhash)
sleep 6
curl -s "${LCD}/cosmos/tx/v1beta1/txs/${MTX}" > /tmp/mig_tx.json
MCODE=$(jq -r '.tx_response.code' /tmp/mig_tx.json)
if [ "${MCODE}" != "0" ]; then
  echo "MIGRATE FAILED (code ${MCODE}):"
  jq -r '.tx_response.raw_log' /tmp/mig_tx.json
  exit 1
fi
echo "migrate tx confirmed."

echo "== post-migrate checks =="
injectived query wasm contract "${CONTRACT}" --node "${NODE}" --output json | jq -c .contract_info
OWNER=$(injectived keys show igit-dev -a --keyring-backend test)
echo "-- pre-existing repos survive:"
injectived query wasm contract-state smart "${CONTRACT}" "{\"list_repos\":{\"owner\":\"${OWNER}\"}}" \
  --node "${NODE}" --output json | jq -r '.data.repos[] | .name + "  forked_from=" + (.forked_from // "null")'
