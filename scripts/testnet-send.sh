#!/usr/bin/env bash
# One-off: send testnet INJ from igit-dev to a recipient (faucet workaround).
# Usage: testnet-send.sh <recipient-inj-address> <amount-inj>
set -euo pipefail
NODE="https://testnet.sentry.tm.injective.network:443"
LCD="https://testnet.sentry.lcd.injective.network:443"
TO="${1:?usage: testnet-send.sh <recipient> <amount-inj>}"
AMT_INJ="${2:?usage: testnet-send.sh <recipient> <amount-inj>}"

case "${TO}" in
  inj1*) ;;
  *) echo "refusing: recipient must be an inj1... address"; exit 1;;
esac

# INJ -> base units (18 decimals), integer INJ or one decimal point
whole="${AMT_INJ%%.*}"
frac=""
[ "${AMT_INJ}" != "${whole}" ] && frac="${AMT_INJ#*.}"
frac="$(printf '%-18s' "${frac}" | tr ' ' '0')"
base="${whole}${frac}"
base="$(echo "${base}" | sed 's/^0*//')"
echo "sending ${AMT_INJ} INJ (${base}inj) to ${TO}"

OUT=$(injectived tx bank send igit-dev "${TO}" "${base}inj" \
  --from igit-dev --chain-id injective-888 --node "${NODE}" \
  --keyring-backend test --gas auto --gas-adjustment 1.4 \
  --gas-prices 500000000inj --broadcast-mode sync --output json --yes 2>&1 | tail -1)
echo "${OUT}" | jq -r '{txhash, code, raw_log}'
TX=$(echo "${OUT}" | jq -r .txhash)
sleep 6
curl -s "${LCD}/cosmos/tx/v1beta1/txs/${TX}" -o /tmp/send.json
echo "on-chain code: $(jq -r '.tx_response.code' /tmp/send.json)"
curl -s "${LCD}/cosmos/bank/v1beta1/balances/${TO}" -o /tmp/rbal.json
echo "recipient balance now:"; jq -c '.balances' /tmp/rbal.json
