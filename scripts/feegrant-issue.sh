#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/feegrant-issue.sh (remove after one release cycle)
echo "[DEPRECATED] feegrant-issue.sh has moved to scripts/ops/feegrant-issue.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/feegrant-issue.sh" "$@"
