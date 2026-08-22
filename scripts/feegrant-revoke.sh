#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/feegrant-revoke.sh (remove after one release cycle)
echo "[DEPRECATED] feegrant-revoke.sh has moved to scripts/ops/feegrant-revoke.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/feegrant-revoke.sh" "$@"
