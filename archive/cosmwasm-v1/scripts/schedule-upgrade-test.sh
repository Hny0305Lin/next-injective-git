#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/scripts/schedule-upgrade.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat > "$TMP/injectived" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$FAKE_INJECTIVED_LOG"
printf '{"txhash":"ABC"}\n'
EOF
chmod 700 "$TMP/injectived"

export PATH="$TMP:$PATH"
export FAKE_INJECTIVED_LOG="$TMP/injectived.log"
export CONTRACT="inj1$(printf 'c%.0s' {1..38})"
export WASM_SHA256="ABCDEFabcdefABCDEFabcdefABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD"
export FROM=tech-admin
export NODE=https://tm.injective.network:443
export KEYRING_BACKEND=test

bash "$SCRIPT" >/dev/null
grep -Fq 'schedule_upgrade' "$FAKE_INJECTIVED_LOG"
grep -Fq 'abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd' "$FAKE_INJECTIVED_LOG"
grep -Fq -- '--chain-id injective-1' "$FAKE_INJECTIVED_LOG"

if CHAIN_ID=injective-888 bash "$SCRIPT" >/dev/null 2>&1; then
  echo "schedule upgrade test: wrong chain unexpectedly succeeded" >&2
  exit 1
fi
if WASM_SHA256=short bash "$SCRIPT" >/dev/null 2>&1; then
  echo "schedule upgrade test: invalid hash unexpectedly succeeded" >&2
  exit 1
fi

echo "schedule upgrade test: mainnet chain, hash validation, and execute payload passed"
