#!/usr/bin/env bash
# Repository-local regression tests for feegrant-policy-gate.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="$ROOT/scripts/feegrant-policy-gate.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

STATE="$TMP/state.tsv"
ADDR_A="inj1$(printf 'a%.0s' {1..38})"
ADDR_B="inj1$(printf 'b%.0s' {1..38})"
ID_A="$(printf 'a%.0s' {1..64})"
ID_B="$(printf 'b%.0s' {1..64})"
TX_A="$(printf '1%.0s' {1..64})"
TX_B="$(printf '2%.0s' {1..64})"
TX_C="$(printf '3%.0s' {1..64})"
TX_D="$(printf '4%.0s' {1..64})"
BASE=1700000000

run_ok() {
  local output
  if ! output=$(IGIT_FEEGRANT_STATE="$STATE" \
    IGIT_FEEGRANT_MAX_PUSHES=3 \
    IGIT_FEEGRANT_TTL_SECONDS=100 \
    IGIT_FEEGRANT_COOLDOWN_SECONDS=1000 \
    IGIT_FEEGRANT_MAX_GRANTS_PER_DAY=100 \
    bash "$GATE" "$@" 2>&1); then
    echo "feegrant policy gate test: expected success: $*" >&2
    echo "$output" >&2
    exit 1
  fi
  printf '%s\n' "$output"
}

run_fail() {
  local output
  if output=$(IGIT_FEEGRANT_STATE="$STATE" \
    IGIT_FEEGRANT_MAX_PUSHES=3 \
    IGIT_FEEGRANT_TTL_SECONDS=100 \
    IGIT_FEEGRANT_COOLDOWN_SECONDS=1000 \
    IGIT_FEEGRANT_MAX_GRANTS_PER_DAY=100 \
    bash "$GATE" "$@" 2>&1); then
    echo "feegrant policy gate test: expected failure: $*" >&2
    echo "$output" >&2
    exit 1
  fi
  printf '%s\n' "$output"
}

assert_contains() {
  local output="$1" expected="$2"
  grep -Fq -- "$expected" <<<"$output" || {
    echo "feegrant policy gate test: output did not contain '$expected'" >&2
    echo "$output" >&2
    exit 1
  }
}

assert_contains "$(run_ok check "$ADDR_A" "$ID_A" "$BASE")" "eligible"
run_ok record-grant "$ADDR_A" "$ID_A" "$TX_A" "$((BASE + 100))" "$BASE" >/dev/null
run_ok record-push "$ADDR_A" "$TX_B" "$((BASE + 1))" >/dev/null
run_ok record-push "$ADDR_A" "$TX_C" "$((BASE + 2))" >/dev/null
run_ok record-push "$ADDR_A" "$TX_D" "$((BASE + 3))" >/dev/null
assert_contains "$(run_ok status "$ADDR_A" "$((BASE + 3))")" "pushes=3"
assert_contains "$(run_fail record-push "$ADDR_A" "$TX_A" "$((BASE + 4))")" "push allowance exhausted"
assert_contains "$(run_fail check "$ADDR_B" "$ID_A" "$((BASE + 4))")" "identity_in_use"
run_ok record-revoke "$ADDR_A" "$TX_A" "$((BASE + 5))" >/dev/null
assert_contains "$(run_fail check "$ADDR_A" "$ID_A" "$((BASE + 6))")" "revoked"

# A new grant is allowed once the address/identity cooldown has elapsed.
assert_contains "$(run_ok check "$ADDR_A" "$ID_A" "$((BASE + 1001))")" "eligible"
run_ok record-grant "$ADDR_A" "$ID_A" "$TX_B" "$((BASE + 1101))" "$((BASE + 1001))" >/dev/null
assert_contains "$(run_ok status "$ADDR_A" "$((BASE + 1002))")" "pushes=0"

# The rolling daily treasury cap is checked before a second grant is recorded.
DAILY_STATE="$TMP/daily.tsv"
if ! IGIT_FEEGRANT_STATE="$DAILY_STATE" \
  IGIT_FEEGRANT_MAX_GRANTS_PER_DAY=1 \
  IGIT_FEEGRANT_TTL_SECONDS=100 \
  IGIT_FEEGRANT_COOLDOWN_SECONDS=1000 \
  IGIT_FEEGRANT_MAX_PUSHES=3 \
  bash "$GATE" record-grant "$ADDR_A" "$ID_A" "$TX_A" "$((BASE + 100))" "$BASE" >/dev/null 2>&1; then
  echo "feegrant policy gate test: first daily grant failed" >&2
  exit 1
fi
daily_output=$(IGIT_FEEGRANT_STATE="$DAILY_STATE" \
  IGIT_FEEGRANT_MAX_GRANTS_PER_DAY=1 \
  IGIT_FEEGRANT_TTL_SECONDS=100 \
  IGIT_FEEGRANT_COOLDOWN_SECONDS=1000 \
  IGIT_FEEGRANT_MAX_PUSHES=3 \
  bash "$GATE" check "$ADDR_B" "$ID_B" "$((BASE + 1))" 2>&1 || true)
assert_contains "$daily_output" "daily treasury grant limit reached"

# A corrupted state file must fail closed.
printf 'not-a-valid-record\n' > "$TMP/corrupt.tsv"
corrupt_output=$(IGIT_FEEGRANT_STATE="$TMP/corrupt.tsv" bash "$GATE" status "$ADDR_A" "$BASE" 2>&1 || true)
assert_contains "$corrupt_output" "malformed state file"

echo "feegrant policy gate test: eligibility, push quota, identity cooldown, revoke, daily cap, and fail-closed state checks passed"
