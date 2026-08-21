#!/usr/bin/env bash
# Run the race detector for the stateful EVM/migration control plane.
# This is intentionally separate from the ordinary Go test gate because race
# builds need a working C compiler. Missing tooling is SKIP locally and FAIL
# only when --required is supplied by CI/release automation.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ ! -f "$ROOT/CLAUDE.md" && ! -f "$ROOT/README.md" ]]; then
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi
if [[ ! -f "$ROOT/CLAUDE.md" ]]; then
  ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo "$ROOT")"
fi
required=0

usage() {
  cat <<'EOF'
usage: race-check.sh [--required]

Runs CGO_ENABLED=1 go test -race for cli/internal/suitemigration and
cli/internal/chain. Without --required, an unavailable Go/C toolchain is a
clear SKIP; with --required it is a failure.
EOF
}

while (($#)); do
  case "$1" in
    --required) required=1 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done

if ! command -v go >/dev/null 2>&1; then
  if ((required)); then
    echo "FAIL: go is required for race checks" >&2
    exit 1
  fi
  echo "SKIP: go not found; race checks were not run"
  exit 0
fi

compiler="${CC:-}"
if [[ -n "$compiler" ]]; then
  if ! command -v "$compiler" >/dev/null 2>&1; then
    if ((required)); then
      echo "FAIL: configured C compiler '$compiler' was not found" >&2
      exit 1
    fi
    echo "SKIP: configured C compiler '$compiler' was not found"
    exit 0
  fi
elif command -v gcc >/dev/null 2>&1; then
  compiler="gcc"
elif command -v clang >/dev/null 2>&1; then
  compiler="clang"
else
  if ((required)); then
    echo "FAIL: gcc or clang is required for Go race checks" >&2
    exit 1
  fi
  echo "SKIP: gcc/clang not found; race checks were not run"
  exit 0
fi

echo "== Go race check =="
echo "compiler=$compiler"
pushd "$ROOT/cli" >/dev/null
CGO_ENABLED=1 go test -race -mod=readonly -count=1 ./internal/suitemigration ./internal/chain
popd >/dev/null
echo "PASS: Go race checks for migration and chain control plane"
