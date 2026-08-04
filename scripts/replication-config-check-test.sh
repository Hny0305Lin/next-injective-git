#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHECK="$ROOT/scripts/replication-config-check.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

write_config() {
  local file="$1" lcd="$2" chain="$3" contract="$4" secret="$5"
  printf '%s\n' \
    "IGIT_REPLICATION_JWT_HMAC=$secret" \
    "CONTRACT=$contract" \
    "CHAIN_ID=$chain" \
    "LCD=$lcd" \
    'IGIT_REPLICATION_MAX_BYTES=2147483648' \
    'IGIT_REPLICATION_RATE_PER_MINUTE=12' \
    'IGIT_REPLICATION_BYTES_PER_MINUTE=4294967296' > "$file"
}

assert_ok() {
  bash "$CHECK" "$@" >/dev/null
}

assert_fail() {
  if bash "$CHECK" "$@" >/dev/null 2>&1; then
    echo "expected config check to fail: $*" >&2
    exit 1
  fi
}

valid_secret=0123456789abcdef0123456789abcdef
valid_contract=inj1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq
write_config "$TMP/valid.env" https://lcd.injective.network:443 injective-1 "$valid_contract" "$valid_secret"
assert_ok reaper "$TMP/valid.env"

write_config "$TMP/testnet.env" https://testnet.sentry.lcd.injective.network:443 injective-1 "$valid_contract" "$valid_secret"
assert_fail reaper "$TMP/testnet.env"
assert_ok staged "$TMP/testnet.env"

write_config "$TMP/wrong-chain.env" https://lcd.injective.network:443 injective-888 "$valid_contract" "$valid_secret"
assert_fail reaper "$TMP/wrong-chain.env"

write_config "$TMP/placeholder.env" https://lcd.injective.network:443 injective-1 inj1replace_me replace-with-a-random-32-byte-minimum-secret
assert_fail staged "$TMP/placeholder.env"

write_config "$TMP/bad-contract.env" https://lcd.injective.network:443 injective-1 inj1short "$valid_secret"
assert_fail reaper "$TMP/bad-contract.env"

echo "replication config check test: staged/reaper chain, endpoint, address, and secret guards passed"
