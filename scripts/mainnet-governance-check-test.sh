#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHECK="$ROOT/scripts/mainnet-governance-check.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

CONTRACT="inj1$(printf 'd%.0s' {1..38})"
ADMIN="inj1$(printf 'a%.0s' {1..38})"
COMMITTEE="inj1$(printf 'b%.0s' {1..38})"
TREASURY="inj1$(printf 'c%.0s' {1..38})"

cat > "$TMP/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
url="${!#}"
if [[ "$url" == *node_info* ]]; then
  printf '{"default_node_info":{"network":"%s"}}\n' "${FAKE_NETWORK:-injective-1}"
  exit 0
fi
if [[ "$url" == */cosmwasm/wasm/v1/code/* ]]; then
  deployed_hash="${EXPECTED_WASM_SHA256:-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
  if [[ "${FAKE_CODE_MODE:-valid}" == mismatch ]]; then
    deployed_hash="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  fi
  printf '{"code_info":{"data_hash":"%s"}}\n' "$deployed_hash"
  exit 0
fi
if [[ "$url" == */cosmwasm/wasm/v1/contract/*/smart/* ]]; then
  count=0
  [[ -f "$FAKE_CURL_COUNT" ]] && count="$(<"$FAKE_CURL_COUNT")"
  count=$((count + 1))
  printf '%s\n' "$count" > "$FAKE_CURL_COUNT"
  if [[ "$count" == 1 ]]; then
    case "${FAKE_CONFIG:-valid}" in
      valid)
        printf '{"data":{"admin":"%s","moderation_committee":"%s","treasury":"%s","platform_fee_bps":300,"username_deposit":{"denom":"inj","amount":"100000000000000000"},"username_fee":{"denom":"inj","amount":"10000000000000000"},"reserved_usernames":["admin","api","git","help","injective","owner","root","support","www"]}}\n' "$FAKE_ADMIN" "$FAKE_COMMITTEE" "$FAKE_TREASURY"
        ;;
      zero-fee)
        printf '{"data":{"admin":"%s","moderation_committee":"%s","treasury":"%s","platform_fee_bps":300,"username_fee":{"denom":"inj","amount":"0"},"reserved_usernames":["admin","api","git","help","injective","owner","root","support","www"]}}\n' "$FAKE_ADMIN" "$FAKE_COMMITTEE" "$FAKE_TREASURY"
        ;;
      high-platform)
        printf '{"data":{"admin":"%s","moderation_committee":"%s","treasury":"%s","platform_fee_bps":501,"username_fee":{"denom":"inj","amount":"1"},"reserved_usernames":["admin","api","git","help","injective","owner","root","support","www"]}}\n' "$FAKE_ADMIN" "$FAKE_COMMITTEE" "$FAKE_TREASURY"
        ;;
      missing-reserved)
        printf '{"data":{"admin":"%s","moderation_committee":"%s","treasury":"%s","platform_fee_bps":300,"username_fee":{"denom":"inj","amount":"1"},"reserved_usernames":["admin"]}}\n' "$FAKE_ADMIN" "$FAKE_COMMITTEE" "$FAKE_TREASURY"
        ;;
      missing-committee)
        printf '{"data":{"admin":"%s","moderation_committee":null,"treasury":"%s","platform_fee_bps":300,"username_fee":{"denom":"inj","amount":"1"},"reserved_usernames":["admin","api","git","help","injective","owner","root","support","www"]}}\n' "$FAKE_ADMIN" "$FAKE_TREASURY"
        ;;
      *)
        echo "unknown FAKE_CONFIG" >&2
        exit 1
        ;;
    esac
  else
    case "${FAKE_UPGRADE:-valid}" in
      valid)
        printf '{"data":{"proposal":null,"timelock_seconds":1209600}}\n'
        ;;
      pending)
        printf '{"data":{"proposal":{"wasm_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","proposed_at":100,"execute_after":1209700},"timelock_seconds":1209600}}\n'
        ;;
      bad-delay)
        printf '{"data":{"proposal":{"wasm_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","proposed_at":100,"execute_after":1209701},"timelock_seconds":1209600}}\n'
        ;;
      short-timelock)
        printf '{"data":{"proposal":null,"timelock_seconds":604800}}\n'
        ;;
      *)
        echo "unknown FAKE_UPGRADE" >&2
        exit 1
        ;;
    esac
  fi
  exit 0
fi
if [[ "$url" == */cosmwasm/wasm/v1/contract/* ]]; then
  printf '{"contract_info":{"admin":"%s","code_id":"7"}}\n' "${FAKE_WASM_ADMIN}"
  exit 0
fi
echo "unexpected fake curl URL: $url" >&2
exit 1
EOF
chmod 700 "$TMP/curl"

export PATH="$TMP:$PATH"
export LCD=https://lcd.injective.network
export CHAIN_ID=injective-1
export CONTRACT ADMIN COMMITTEE TREASURY
export FAKE_WASM_ADMIN="$ADMIN"
export FAKE_ADMIN="$ADMIN"
export FAKE_COMMITTEE="$COMMITTEE"
export FAKE_TREASURY="$TREASURY"
export EXPECTED_ADMIN="$ADMIN"
export EXPECTED_COMMITTEE="$COMMITTEE"
export EXPECTED_WASM_SHA256="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
export FAKE_CURL_COUNT="$TMP/curl.count"

run_ok() {
  rm -f "$FAKE_CURL_COUNT"
  bash "$CHECK" >/dev/null
}

run_fail() {
  rm -f "$FAKE_CURL_COUNT"
  if bash "$CHECK" >/dev/null 2>&1; then
    echo "expected governance check to fail" >&2
    exit 1
  fi
}

run_ok
EXPECTED_WASM_SHA256= run_ok
FAKE_CODE_MODE=mismatch run_fail
FAKE_UPGRADE=pending run_ok
FAKE_UPGRADE=bad-delay run_fail
FAKE_UPGRADE=short-timelock run_fail
FAKE_NETWORK=injective-888 run_fail
LCD=https://testnet.sentry.lcd.injective.network:443 run_fail
FAKE_CONFIG=zero-fee run_fail
FAKE_CONFIG=high-platform run_fail
FAKE_CONFIG=missing-reserved run_fail
FAKE_CONFIG=missing-committee run_fail
EXPECTED_ADMIN="inj1$(printf 'e%.0s' {1..38})" run_fail

echo "mainnet governance check test: network, multisig, fee, reserved names, upgrade timelock, and Wasm hash guards passed"
