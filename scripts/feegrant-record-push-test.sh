#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/scripts/feegrant-record-push.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAKE="$TMP/injectived"
cat > "$FAKE" <<'EOF'
#!/usr/bin/env bash
case "${FAKE_TX_MODE:-success}" in
  success)
    printf '%s\n' '{"tx_response":{"code":0,"tx":{"body":{"messages":[{"@type":"/cosmwasm.wasm.v1.MsgExecuteContract","msg":{"update_ref":{"owner":"inj1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repo":"demo","ref_name":"refs/heads/main"}}}]}}}}'
    ;;
  failed)
    printf '%s\n' '{"tx_response":{"code":12,"tx":{"body":{"messages":[]}}}}'
    ;;
  wrong-message)
    printf '%s\n' '{"tx_response":{"code":0,"tx":{"body":{"messages":[{"@type":"/cosmos.bank.v1beta1.MsgSend","msg":{}}]}}}}'
    ;;
esac
EOF
chmod 700 "$FAKE"

ADDRESS="inj1$(printf 'a%.0s' {1..38})"
TX="ABCDEFabcdefABCDEFabcdefABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"
export INJECTIVED_BIN="$FAKE"
export IGIT_FEEGRANT_CHAIN_ID=injective-1
export IGIT_FEEGRANT_NODE=https://tm.injective.network:443
export IGIT_FEEGRANT_STATE="$TMP/state.tsv"
printf 'grant\t%s\t%s\t1\t9999999999\t%s\n' "$ADDRESS" "$(printf 'b%.0s' {1..64})" "$TX" > "$IGIT_FEEGRANT_STATE"

bash "$SCRIPT" "$ADDRESS" "$TX" >/dev/null
grep -Fq "push" "$IGIT_FEEGRANT_STATE"

for mode in failed wrong-message; do
  if FAKE_TX_MODE="$mode" bash "$SCRIPT" "$ADDRESS" "$TX" >/dev/null 2>&1; then
    echo "feegrant record test: $mode transaction unexpectedly recorded" >&2
    exit 1
  fi
done

echo "feegrant record push test: successful update_ref verification and fail-closed paths passed"
