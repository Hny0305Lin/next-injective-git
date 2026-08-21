#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/evm-v2-check-test.sh (remove after one release cycle)
echo "[DEPRECATED] evm-v2-check-test.sh has moved to scripts/ci/evm-v2-check-test.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/evm-v2-check-test.sh" "$@"
