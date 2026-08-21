#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/feegrant-policy-gate.sh (remove after one release cycle)
echo "[DEPRECATED] feegrant-policy-gate.sh has moved to scripts/ci/feegrant-policy-gate.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/feegrant-policy-gate.sh" "$@"
