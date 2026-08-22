#!/usr/bin/env bash
# Run the deterministic HK outage and CID-miss fallback drill.
#
# This is a repository-local acceptance fixture. It does not change either
# production gateway; live mainland/HK drills still require the operator to
# run the documented probes from the target network.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"

command -v "$GO_BIN" >/dev/null 2>&1 || {
  echo "gateway fallback acceptance: Go toolchain not found: $GO_BIN" >&2
  exit 1
}

cd "$ROOT/cli"
test_name=TestGatewayFallbackAcceptance
listed="$("$GO_BIN" test ./internal/ipfs -list "^${test_name}$")"
matches="$(printf '%s\n' "$listed" | awk -v name="$test_name" '$0 == name { count++ } END { print count + 0 }')"
if [[ "$matches" -ne 1 ]]; then
  echo "gateway fallback acceptance: expected exactly one $test_name test, found $matches" >&2
  exit 1
fi
"$GO_BIN" test ./internal/ipfs -run '^TestGatewayFallbackAcceptance$' -count=1 -v
echo "gateway fallback acceptance: HK outage and CID-miss fallback passed"
