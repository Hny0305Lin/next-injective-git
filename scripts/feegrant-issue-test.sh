#!/usr/bin/env bash
# Regression test for feegrant-issue.sh using a fake injectived binary.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ISSUER="$ROOT/scripts/feegrant-issue.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAKE="$TMP/injectived"
LOG="$TMP/injectived.log"
cat > "$FAKE" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$FAKE_LOG"
if [[ "${FAKE_FAIL:-0}" == "1" ]]; then
  printf '{"height":"1","txhash":"","code":7,"raw_log":"insufficient funds"}\n'
else
  printf '{"height":"1","txhash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","code":0}\n'
fi
EOF
chmod 700 "$FAKE"

ADDR="inj1$(printf 'a%.0s' {1..38})"
IDENTITY="$(printf 'a%.0s' {1..64})"
STATE="$TMP/state.tsv"

output=$(
  FAKE_LOG="$LOG" \
  INJECTIVED_BIN="$FAKE" \
  IGIT_FEEGRANT_STATE="$STATE" \
  IGIT_FEEGRANT_GRANTER=treasury \
  IGIT_FEEGRANT_CHAIN_ID=injective-888 \
  IGIT_FEEGRANT_NODE=https://node.invalid \
  IGIT_FEEGRANT_TTL_SECONDS=100 \
  bash "$ISSUER" "$ADDR" "$IDENTITY" 2>&1
)
grep -Fq "feegrant issued: address=$ADDR" <<<"$output"
grep -Fq $'grant\t' "$STATE" || {
  echo "feegrant issuer test: successful transaction was not recorded" >&2
  cat "$STATE" >&2
  exit 1
}
grep -Fq -- "--spend-limit 30000000000000000inj" "$LOG"
grep -Fq -- "--allowed-messages /cosmwasm.wasm.v1.MsgExecuteContract" "$LOG"
grep -Fq -- "--chain-id injective-888" "$LOG"

if FAKE_LOG="$LOG" \
  INJECTIVED_BIN="$FAKE" \
  IGIT_FEEGRANT_STATE="$STATE" \
  IGIT_FEEGRANT_GRANTER=treasury \
  IGIT_FEEGRANT_CHAIN_ID=injective-888 \
  IGIT_FEEGRANT_NODE=https://node.invalid \
  bash "$ISSUER" "$ADDR" "$IDENTITY" >/dev/null 2>&1; then
  echo "feegrant issuer test: duplicate active grant unexpectedly succeeded" >&2
  exit 1
fi
test "$(wc -l < "$LOG")" -eq 1

REVOKER="$ROOT/scripts/feegrant-revoke.sh"
revoke_output=$(
  FAKE_LOG="$LOG" \
  INJECTIVED_BIN="$FAKE" \
  IGIT_FEEGRANT_STATE="$STATE" \
  IGIT_FEEGRANT_GRANTER=treasury \
  IGIT_FEEGRANT_CHAIN_ID=injective-888 \
  IGIT_FEEGRANT_NODE=https://node.invalid \
  bash "$REVOKER" "$ADDR" 2>&1
)
grep -Fq "feegrant revoked: address=$ADDR" <<<"$revoke_output"
grep -Fq $'revoke\t' "$STATE"
grep -Fq -- "tx feegrant revoke treasury $ADDR" "$LOG"
test "$(wc -l < "$LOG")" -eq 2

already_output=$(
  FAKE_LOG="$LOG" \
  INJECTIVED_BIN="$FAKE" \
  IGIT_FEEGRANT_STATE="$STATE" \
  IGIT_FEEGRANT_GRANTER=treasury \
  IGIT_FEEGRANT_CHAIN_ID=injective-888 \
  IGIT_FEEGRANT_NODE=https://node.invalid \
  bash "$REVOKER" "$ADDR" 2>&1
)
grep -Fq "already revoked" <<<"$already_output"
test "$(wc -l < "$LOG")" -eq 2

if IGIT_FEEGRANT_SPEND_LIMIT=30000000000000001inj \
  FAKE_LOG="$LOG" \
  INJECTIVED_BIN="$FAKE" \
  IGIT_FEEGRANT_STATE="$TMP/cap.tsv" \
  IGIT_FEEGRANT_GRANTER=treasury \
  IGIT_FEEGRANT_CHAIN_ID=injective-888 \
  IGIT_FEEGRANT_NODE=https://node.invalid \
  bash "$ISSUER" "$ADDR" "$IDENTITY" >/dev/null 2>&1; then
  echo "feegrant issuer test: over-cap spend limit unexpectedly succeeded" >&2
  exit 1
fi
if IGIT_FEEGRANT_ALLOWED_MESSAGES=/cosmos.bank.v1beta1.MsgSend \
  FAKE_LOG="$LOG" \
  INJECTIVED_BIN="$FAKE" \
  IGIT_FEEGRANT_STATE="$TMP/message.tsv" \
  IGIT_FEEGRANT_GRANTER=treasury \
  IGIT_FEEGRANT_CHAIN_ID=injective-888 \
  IGIT_FEEGRANT_NODE=https://node.invalid \
  bash "$ISSUER" "$ADDR" "$IDENTITY" >/dev/null 2>&1; then
  echo "feegrant issuer test: disallowed message policy unexpectedly succeeded" >&2
  exit 1
fi
test "$(wc -l < "$LOG")" -eq 2

FAIL_STATE="$TMP/fail.tsv"
if FAKE_LOG="$LOG" \
  FAKE_FAIL=1 \
  INJECTIVED_BIN="$FAKE" \
  IGIT_FEEGRANT_STATE="$FAIL_STATE" \
  IGIT_FEEGRANT_GRANTER=treasury \
  IGIT_FEEGRANT_CHAIN_ID=injective-888 \
  IGIT_FEEGRANT_NODE=https://node.invalid \
  bash "$ISSUER" "$ADDR" "$IDENTITY" >/dev/null 2>&1; then
  echo "feegrant issuer test: failed chain transaction unexpectedly succeeded" >&2
  exit 1
fi
if [ -s "$FAIL_STATE" ]; then
  echo "feegrant issuer test: failed transaction modified policy state" >&2
  cat "$FAIL_STATE" >&2
  exit 1
fi

echo "feegrant issuer test: serialized issue/revoke, duplicate rejection, and fail-closed broadcast handling passed"
