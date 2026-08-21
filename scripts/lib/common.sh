#!/usr/bin/env bash
# scripts/lib/common.sh — shared helpers for all scripts/* sub-scripts
# Usage: source "$(dirname "${BASH_SOURCE[0]}")/../lib/common.sh"  (or /lib/common.sh from repo root)
# Provides: ROOT, strict mode, logging helpers, and path resolution that survives reorg.

# Do not set -e here — caller decides. Export ROOT and helpers.
if [[ -z "${IGIT_COMMON_SOURCED:-}" ]]; then
  IGIT_COMMON_SOURCED=1

  # Resolve repository root (two levels up from scripts/lib/)
  if [[ -z "${ROOT:-}" ]]; then
    _common_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    # _common_dir is .../scripts/lib; ROOT is two up
    ROOT="$(cd "${_common_dir}/../.." && pwd)"
    export ROOT
  fi

  # --- logging ---
  _igit_log()  { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }
  igit_info()  { _igit_log "INFO  $*"; }
  igit_warn()  { _igit_log "WARN  $*"; }
  igit_error() { _igit_log "ERROR $*"; }
  igit_die()   { igit_error "$*"; exit 1; }

  # --- path helpers ---
  # Resolve a scripts/* path that may have moved (ci/ops/deploy/migration/storage)
  # Usage: igit_script_path "suite-readiness.sh" -> absolute path if found
  igit_script_path() {
    local name="$1"
    local candidates=(
      "$ROOT/scripts/$name"
      "$ROOT/scripts/ci/$name"
      "$ROOT/scripts/ops/$name"
      "$ROOT/scripts/deploy/$name"
      "$ROOT/scripts/migration/$name"
      "$ROOT/scripts/storage/$name"
    )
    for c in "${candidates[@]}"; do
      [[ -f "$c" ]] && { echo "$c"; return 0; }
    done
    echo "$ROOT/scripts/$name"
    return 1
  }

  export -f igit_info igit_warn igit_error igit_die igit_script_path 2>/dev/null || true
fi
