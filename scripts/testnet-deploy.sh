#!/usr/bin/env bash
# Deploy repo-registry to Injective testnet and print the contract address.
# Usage: testnet-deploy.sh <store_txhash>
set -euo pipefail

LCD="https://testnet.sentry.lcd.injective.network:443"
NODE="https://testnet.sentry.tm.injective.network:443"
CHAIN_ID="injective-888"
KEY="igit-dev"
KB="test"
GAS_FLAGS=(--gas auto --gas-adjustment 1.4 --gas-prices 500000000inj)

TXHASH="${1:?usage: testnet-deploy.sh <store_txhash>}"

echo "==> query store tx ${TXHASH}"
curl -s "${LCD}/cosmos/tx/v1beta1/txs/${TXHASH}" > /tmp/store_tx.json
CODE=$(jq -r '.tx_response.code' /tmp/store_tx.json)
if [ "${CODE}" != "0" ]; then
  echo "store tx failed (code=${CODE}):"
  jq -r '.tx_response.raw_log' /tmp/store_tx.json
  exit 1
fi
CODE_ID=$(jq -r '[.tx_response.events[] | select(.type | test("CodeStored|store_code"))
  | .attributes[] | select(.key == "code_id") | .value][0]' /tmp/store_tx.json | tr -d '"')
echo "code_id=${CODE_ID}"

ADMIN=$(injectived keys show "${KEY}" -a --keyring-backend "${KB}")
echo "==> instantiate with admin=${ADMIN}"
INIT_MSG=$(printf '{"admin":"%s"}' "${ADMIN}")
OUT=$(injectived tx wasm instantiate "${CODE_ID}" "${INIT_MSG}" \
  --label igit-repo-registry \
  --admin "${ADMIN}" \
  --from "${KEY}" --chain-id "${CHAIN_ID}" --node "${NODE}" \
  --keyring-backend "${KB}" "${GAS_FLAGS[@]}" \
  --broadcast-mode sync --output json --yes 2>&1 | tail -1)
echo "${OUT}" | jq -r '{txhash, code, raw_log}'
ITX=$(echo "${OUT}" | jq -r '.txhash')

echo "==> wait for inclusion"
sleep 6
curl -s "${LCD}/cosmos/tx/v1beta1/txs/${ITX}" > /tmp/inst_tx.json
ICODE=$(jq -r '.tx_response.code' /tmp/inst_tx.json)
if [ "${ICODE}" != "0" ]; then
  echo "instantiate tx failed (code=${ICODE}):"
  jq -r '.tx_response.raw_log' /tmp/inst_tx.json
  exit 1
fi
CONTRACT=$(jq -r '[.tx_response.events[] | select(.type | test("ContractInstantiated|instantiate"))
  | .attributes[] | select(.key == "contract_address" or .key == "_contract_address") | .value][0]' \
  /tmp/inst_tx.json | tr -d '"')
echo "contract_address=${CONTRACT}"

echo "==> configure igit"
igit config set contract_address "${CONTRACT}"
igit config set key_name "${KEY}"
igit config list
