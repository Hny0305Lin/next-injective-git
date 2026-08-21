#!/usr/bin/env bash
# Verify the MetaMask EIP-712 sponsorship tx on-chain: prove it went through the
# web3/EIP-712 path (ExtensionOptionsWeb3Tx + ethsecp256k1 pubkey + EIP712_V2)
# and that the sponsorship split settled.
set -uo pipefail
LCD="https://testnet.sentry.lcd.injective.network:443"
ADDR="inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d"
PREFIX="${1:-3BD391ED2896}"

curl -s -G "${LCD}/cosmos/tx/v1beta1/txs" \
  --data-urlencode "query=message.sender='${ADDR}'" \
  --data-urlencode "pagination.limit=10" \
  --data-urlencode "order_by=ORDER_BY_DESC" > /tmp/mm.json

echo "== tx count for ${ADDR}: $(jq -r '.tx_responses | length' /tmp/mm.json) =="
echo
echo "== proof: the matching tx (${PREFIX}...) =="
jq -r --arg p "$PREFIX" '
  .tx_responses[] | select(.txhash | ascii_upcase | startswith($p)) | {
    txhash, code, height,
    raw_log: (.raw_log | if length > 80 then .[0:80] else . end),
    extension_options: [.tx.body.extension_options[]."@type"],
    sign_mode: .tx.auth_info.signer_infos[0].mode_info.single.mode,
    pubkey_type: .tx.auth_info.signer_infos[0].public_key."@type"
  }' /tmp/mm.json

echo
echo "== the split (transfer events: recipient / amount) =="
jq -r --arg p "$PREFIX" '
  .tx_responses[] | select(.txhash | ascii_upcase | startswith($p))
  | .events[] | select(.type=="transfer")
  | (.attributes | from_entries)
  | "  -> " + (.recipient // "?") + "  " + (.amount // "?")' /tmp/mm.json
