#!/usr/bin/env bash
# Fail-closed cutover evidence gate. Unlike migration-readiness.sh, this gate
# accepts only immutable operator evidence and never derives deployment claims
# from source files or test names.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EVIDENCE="${1:-}"
EXPECTED_COMMIT="${2:-}"
usage() {
  echo "usage: migration-cutover-readiness.sh <reviewed-evidence-directory> <expected-commit>" >&2
}
if [[ -z "$EVIDENCE" || "$EVIDENCE" == -* || ! "$EXPECTED_COMMIT" =~ ^[0-9a-fA-F]{40}$ || $# -ne 2 ]]; then
  usage
  exit 2
fi
EXPECTED_COMMIT="${EXPECTED_COMMIT,,}"
while [[ "$EVIDENCE" != "/" && "$EVIDENCE" == */ ]]; do
  EVIDENCE="${EVIDENCE%/}"
done
if [[ -L "$EVIDENCE" ]]; then
  echo "CUTOVER READINESS: FAIL (evidence directory must not be a symbolic link)" >&2
  exit 1
fi
EVIDENCE="$(cd "$EVIDENCE" 2>/dev/null && pwd)" || {
  echo "CUTOVER READINESS: FAIL (evidence directory does not exist)" >&2
  exit 1
}
EVIDENCE="$(cd "$EVIDENCE" && pwd -P)"

manifest="$EVIDENCE/cutover-evidence.sha256"
approval="$EVIDENCE/cutover-approval.txt"
scope="$EVIDENCE/cutover-scope.json"
if [[ ! -s "$scope" || -L "$scope" ]]; then
  echo "CUTOVER READINESS: FAIL (missing or unsafe cutover-scope.json)" >&2
  exit 1
fi
cutover_mode="$(node -e 'const fs=require("fs");const value=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));if(typeof value.mode!=="string")process.exit(1);process.stdout.write(value.mode)' "$scope")" || {
  echo "CUTOVER READINESS: FAIL (invalid cutover-scope.json)" >&2
  exit 1
}
required=(
  cutover-scope.json
  deployment.json
  suite-verification.json
  blockscout-verification.json
  foundry-test.txt
  foundry-invariant.txt
  foundry-gas.txt
  windows-clean-e2e.txt
  linux-clean-e2e.txt
  web-receipt-e2e.txt
  security-review.pdf
  finality-runbook.md
  cutover-approval.txt
)
case "$cutover_mode" in
  fresh-empty-suite)
    activation_journal="$(node -e 'const fs=require("fs");const value=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));const name=value.activation_journal;if(typeof name!=="string"||!/^[A-Za-z0-9._-]+$/.test(name))process.exit(1);process.stdout.write(name)' "$scope")" || {
      echo "CUTOVER READINESS: FAIL (invalid fresh-suite activation journal path)" >&2
      exit 1
    }
    required+=(empty-username-escrow-attestation.json "$activation_journal")
    ;;
  cosmwasm-v1-migration)
    required+=(
      admin-dry-run-journal.tar
      migration-plan.json
      migration-manifest.json
      migration-receipt-journal.tar
      imported-state.json
    )
    ;;
  *)
    echo "CUTOVER READINESS: FAIL (unsupported cutover mode: $cutover_mode)" >&2
    exit 1
    ;;
esac

if [[ ! -s "$manifest" ]]; then
  echo "CUTOVER READINESS: FAIL (missing cutover-evidence.sha256)" >&2
  exit 1
fi
if [[ -L "$manifest" ]]; then
  echo "CUTOVER READINESS: FAIL (checksum manifest must not be a symbolic link)" >&2
  exit 1
fi
declare -A checksums=()
declare -A folded_paths=()
while IFS= read -r record || [[ -n "$record" ]]; do
  record="${record%$'\r'}"
  if [[ ! "$record" =~ ^([0-9a-fA-F]{64})[[:space:]]+\*?(.+)$ ]]; then
    echo "CUTOVER READINESS: FAIL (malformed checksum record)" >&2
    exit 1
  fi
  digest="${BASH_REMATCH[1],,}"
  path="${BASH_REMATCH[2]}"
  if [[ "$path" == /* || "$path" == \\* || "$path" =~ (^|[/\\])\.\.([/\\]|$) ]]; then
    echo "CUTOVER READINESS: FAIL (checksum path escapes evidence directory: $path)" >&2
    exit 1
  fi
  if [[ -n "${checksums[$path]+present}" ]]; then
    echo "CUTOVER READINESS: FAIL (duplicate checksum record: $path)" >&2
    exit 1
  fi
  folded_path="${path,,}"
  if [[ -n "${folded_paths[$folded_path]+present}" ]]; then
    echo "CUTOVER READINESS: FAIL (case-folded checksum path collision: $path)" >&2
    exit 1
  fi
  checksums["$path"]="$digest"
  folded_paths["$folded_path"]="$path"
done < "$manifest"

for name in "${required[@]}"; do
  if [[ ! -s "$EVIDENCE/$name" ]]; then
    echo "CUTOVER READINESS: FAIL (missing evidence: $name)" >&2
    exit 1
  fi
  if [[ -z "${checksums[$name]+present}" ]]; then
    echo "CUTOVER READINESS: FAIL (evidence hash is not bound: $name)" >&2
    exit 1
  fi
done

declare -A approval_fields=()
approval_valid=true
while IFS= read -r approval_line || [[ -n "$approval_line" ]]; do
  approval_line="${approval_line%$'\r'}"
  if [[ "$approval_line" != *=* ]]; then
    approval_valid=false
    break
  fi
  approval_key="${approval_line%%=*}"
  approval_value="${approval_line#*=}"
  case "$approval_key" in
    decision|reviewer|reviewed_commit|reviewed_at)
      if [[ -n "${approval_fields[$approval_key]+present}" ]]; then
        approval_valid=false
        break
      fi
      approval_fields["$approval_key"]="$approval_value"
      ;;
  esac
done < "$approval"

reviewed_commit="${approval_fields[reviewed_commit]:-}"
reviewed_commit="${reviewed_commit,,}"
if [[ "$approval_valid" != true ||
      "${approval_fields[decision]:-}" != "approved" ||
      ! "${approval_fields[reviewer]:-}" =~ ^[^[:space:]].*$ ||
      "$reviewed_commit" != "$EXPECTED_COMMIT" ||
      ! "${approval_fields[reviewed_at]:-}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T.*Z$ ]]; then
  echo "CUTOVER READINESS: FAIL (cutover approval is incomplete)" >&2
  exit 1
fi

for name in "${!checksums[@]}"; do
  path="$EVIDENCE/$name"
  current="$EVIDENCE"
  IFS='/' read -r -a components <<< "$name"
  for component in "${components[@]}"; do
    current="$current/$component"
    if [[ -L "$current" ]]; then
      echo "CUTOVER READINESS: FAIL (checksum path contains a symbolic link: $name)" >&2
      exit 1
    fi
  done
  if [[ ! -f "$path" ]]; then
    echo "CUTOVER READINESS: FAIL (checksum record names a missing file: $name)" >&2
    exit 1
  fi
  actual="$(sha256sum -- "$path" | awk '{print $1}')"
  if [[ "$actual" != "${checksums[$name]}" ]]; then
    echo "CUTOVER READINESS: FAIL (evidence checksum mismatch: $name)" >&2
    exit 1
  fi
done

command -v node >/dev/null 2>&1 || {
  echo "CUTOVER READINESS: FAIL (node is required for Suite evidence validation)" >&2
  exit 1
}
node "$ROOT/scripts/validate-suite-cutover.mjs" "$EVIDENCE" "$EXPECTED_COMMIT" >/dev/null || {
  echo "CUTOVER READINESS: FAIL (Suite deployment or activation evidence is invalid)" >&2
  exit 1
}

echo "CUTOVER READINESS: PASS (reviewed, hash-bound evidence verified)"
echo "evidence directory: $EVIDENCE"
echo "source gate: $ROOT/scripts/suite-readiness.sh --required"
