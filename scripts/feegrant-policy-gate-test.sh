#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/feegrant-policy-gate-test.sh (remove after one release cycle)
echo "[DEPRECATED] feegrant-policy-gate-test.sh has moved to scripts/ci/feegrant-policy-gate-test.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/feegrant-policy-gate-test.sh" "$@"
