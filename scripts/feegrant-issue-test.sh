#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ops/feegrant-issue-test.sh (remove after one release cycle)
echo "[DEPRECATED] feegrant-issue-test.sh has moved to scripts/ops/feegrant-issue-test.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ops/feegrant-issue-test.sh" "$@"
