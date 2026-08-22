#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/race-check.sh (remove after one release cycle)
echo "[DEPRECATED] race-check.sh has moved to scripts/ci/race-check.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/race-check.sh" "$@"
