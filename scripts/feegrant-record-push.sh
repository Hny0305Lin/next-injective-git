#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/feegrant-record-push.sh (remove after one release cycle)
echo "[DEPRECATED] feegrant-record-push.sh has moved to scripts/ops/feegrant-record-push.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/feegrant-record-push.sh" "$@"
