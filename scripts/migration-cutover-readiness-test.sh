#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="$ROOT/scripts/migration-cutover-readiness.sh"
EXPECTED_COMMIT="${EXPECTED_CUTOVER_COMMIT:-0123456789abcdef0123456789abcdef01234567}"
if [[ ! "$EXPECTED_COMMIT" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "invalid EXPECTED_CUTOVER_COMMIT fixture value" >&2
  exit 2
fi
EXPECTED_COMMIT="${EXPECTED_COMMIT,,}"
MISMATCHED_COMMIT="fedcba9876543210fedcba9876543210fedcba98"
if [[ "$MISMATCHED_COMMIT" == "$EXPECTED_COMMIT" ]]; then
  MISMATCHED_COMMIT="0123456789abcdef0123456789abcdef01234567"
fi
TMP="$(mktemp -d "${TMPDIR:-/tmp}/igit-cutover-gate.XXXXXX")"
trap 'rm -rf -- "$TMP"' EXIT

if bash "$GATE" "$TMP" >/dev/null 2>&1; then
  echo "missing expected commit unexpectedly passed" >&2
  exit 1
fi
if bash "$GATE" "$TMP" not-a-commit >/dev/null 2>&1; then
  echo "malformed expected commit unexpectedly passed" >&2
  exit 1
fi
if bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null 2>&1; then
  echo "empty evidence unexpectedly passed" >&2
  exit 1
fi

required=(
  deployment.json
  foundry-test.txt
  foundry-invariant.txt
  foundry-gas.txt
  admin-dry-run-journal.tar
  migration-plan.json
  migration-manifest.json
  migration-receipt-journal.tar
  imported-state.json
  windows-clean-e2e.txt
  linux-clean-e2e.txt
  web-receipt-e2e.txt
  security-review.pdf
  finality-runbook.md
  cutover-approval.txt
)
for name in "${required[@]}"; do
  if [[ "$name" == "cutover-approval.txt" ]]; then
    printf '%s\n' \
      'decision=approved' \
      'reviewer=发布审核员' \
      "reviewed_commit=$EXPECTED_COMMIT" \
      'reviewed_at=2026-08-12T00:00:00Z' > "$TMP/$name"
  else
    printf 'fixture for %s\n' "$name" > "$TMP/$name"
  fi
done
(
  cd "$TMP"
  sha256sum -- "${required[@]}" > cutover-evidence.sha256
)
bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null

if bash "$GATE" "$TMP" "$MISMATCHED_COMMIT" >/dev/null 2>&1; then
  echo "mismatched expected commit unexpectedly passed" >&2
  exit 1
fi

ROOT_LINK="$TMP.root-link"
ln -s "$TMP" "$ROOT_LINK"
if bash "$GATE" "$ROOT_LINK" "$EXPECTED_COMMIT" >/dev/null 2>&1; then
  echo "root symbolic-link evidence directory unexpectedly passed" >&2
  exit 1
fi
rm -- "$ROOT_LINK"

printf '%s\r\n' \
  'decision=approved' \
  'reviewer=发布审核员' \
  "reviewed_commit=$EXPECTED_COMMIT" \
  'reviewed_at=2026-08-12T00:00:00Z' > "$TMP/cutover-approval.txt"
(
  cd "$TMP"
  for name in "${required[@]}"; do
    digest="$(sha256sum -- "$name" | awk '{print $1}')"
    printf '%s  %s\r\n' "$digest" "$name"
  done > cutover-evidence.sha256
)
if ! bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null; then
  echo "UTF-8 CRLF fixture did not pass" >&2
  exit 1
fi

cp "$TMP/cutover-evidence.sha256" "$TMP/cutover-evidence.sha256.valid"
printf '%064d  missing-extra.txt\n' 0 >> "$TMP/cutover-evidence.sha256"
if bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null 2>&1; then
  echo "missing extra checksum record unexpectedly passed" >&2
  exit 1
fi
mv "$TMP/cutover-evidence.sha256.valid" "$TMP/cutover-evidence.sha256"

cp "$TMP/cutover-evidence.sha256" "$TMP/cutover-evidence.sha256.valid"
printf '%064d  DEPLOYMENT.JSON\r\n' 0 >> "$TMP/cutover-evidence.sha256"
if bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null 2>&1; then
  echo "case-folded checksum collision unexpectedly passed" >&2
  exit 1
fi
mv "$TMP/cutover-evidence.sha256.valid" "$TMP/cutover-evidence.sha256"

printf 'linked evidence\n' > "$TMP/link-target.txt"
ln -s link-target.txt "$TMP/link-evidence.txt"
linked_digest="$(sha256sum -- "$TMP/link-target.txt" | awk '{print $1}')"
cp "$TMP/cutover-evidence.sha256" "$TMP/cutover-evidence.sha256.valid"
printf '%s  link-evidence.txt\r\n' "$linked_digest" >> "$TMP/cutover-evidence.sha256"
if bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null 2>&1; then
  echo "symbolic-link evidence path unexpectedly passed" >&2
  exit 1
fi
rm -- "$TMP/link-evidence.txt" "$TMP/link-target.txt"
mv "$TMP/cutover-evidence.sha256.valid" "$TMP/cutover-evidence.sha256"

printf 'tampered\n' >> "$TMP/deployment.json"
if bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null 2>&1; then
  echo "tampered evidence unexpectedly passed" >&2
  exit 1
fi

printf 'cutover readiness gate test: pass\n'
