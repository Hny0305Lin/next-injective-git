#!/usr/bin/env bash
# List all txs sent by our testnet account, with wasm action summary.
set -uo pipefail
LCD="https://testnet.sentry.lcd.injective.network:443"
OWNER="${1:-inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5}"

curl -s -G "${LCD}/cosmos/tx/v1beta1/txs" \
  --data-urlencode "query=message.sender='${OWNER}'" \
  --data-urlencode "pagination.limit=100" \
  --data-urlencode "order_by=ORDER_BY_DESC" > /tmp/txs.json

tr -d '\r' < "$(dirname "$0")/../.txlist.jq" > /tmp/txlist.jq
echo "total txs: $(jq -r '.tx_responses | length' /tmp/txs.json)"
jq -r -f /tmp/txlist.jq /tmp/txs.json
