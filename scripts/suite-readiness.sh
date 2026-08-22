#!/usr/bin/env bash
# Compatibility shim — moved to scripts/ci/suite-readiness.sh (remove after one release cycle)
echo "[DEPRECATED] suite-readiness.sh has moved to scripts/ci/suite-readiness.sh — please update your reference" >&2
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/ci/suite-readiness.sh" "$@"
