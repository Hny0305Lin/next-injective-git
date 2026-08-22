#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/evm-v2-check.sh (remove after one release cycle)
echo "[DEPRECATED] evm-v2-check.sh has moved to scripts/ci/evm-v2-check.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/evm-v2-check.sh" "$@"
