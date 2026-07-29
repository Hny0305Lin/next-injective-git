#!/usr/bin/env bash
# Show on-chain state of the deployed repo-registry (Injective testnet).
set -uo pipefail

NODE="https://testnet.sentry.tm.injective.network:443"
CONTRACT="${1:-inj17jshk9dwjhu42mx2ywhy0k2a9qy6v0d6qeua37}"
OWNER="${2:-inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5}"

q() { injectived query wasm contract-state smart "${CONTRACT}" "$1" --node "${NODE}" --output json | jq .data; }

echo "=== contract info (code_id / admin / label) ==="
injectived query wasm contract "${CONTRACT}" --node "${NODE}" --output json | jq .contract_info

echo "=== contract config (in-contract admin / committee) ==="
q '{"config":{}}'

echo "=== repos owned by ${OWNER} ==="
q "{\"list_repos\":{\"owner\":\"${OWNER}\"}}"

echo "=== refs of each repo ==="
for r in $(q "{\"list_repos\":{\"owner\":\"${OWNER}\"}}" | jq -r '.repos[].name'); do
  echo "--- ${r} ---"
  q "{\"list_refs\":{\"owner\":\"${OWNER}\",\"repo\":\"${r}\"}}"
done

echo "=== account balance ==="
injectived query bank balances "${OWNER}" --node "${NODE}" --output json | jq -c '.balances'
