#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ ! -f "$ROOT/CLAUDE.md" && ! -f "$ROOT/README.md" ]]; then
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi
if [[ ! -f "$ROOT/CLAUDE.md" ]]; then
  ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo "$ROOT")"
fi
GATE="$ROOT/scripts/migration/migration-cutover-readiness.sh"
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
  suite-verification.json
  blockscout-verification.json
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
  elif [[ "$name" != "deployment.json" && "$name" != "suite-verification.json" && "$name" != "blockscout-verification.json" ]]; then
    printf 'fixture for %s\n' "$name" > "$TMP/$name"
  fi
done

address() { printf '0x%040x' "$1"; }
code_hash() { printf '0x%064x' "$1"; }
receipt() { printf '{"status":"0x1","blockHash":"%s"}' "$(code_hash 900)"; }
contracts=(SuiteDirectory BootstrapCoordinator RepositoryCore RecoveryModule ModerationModule EconomicModule UsernameModule BadgeModule ReleaseModule)
{
  printf '{"schema":"igit.evm-suite.deployment.v1","status":"bootstrapping","compiler":{"version":"0.8.24"},'
  printf '"source":{"commit":"%s"},"chain":{"chain_id":1439},"snapshot_root":"%s",' "$EXPECTED_COMMIT" "$(code_hash 700)"
  printf '"contracts":['
  for index in "${!contracts[@]}"; do
    (( index == 0 )) || printf ','
    printf '{"transaction_order":%d,"contract_name":"%s","address":"%s","transaction_hash":"%s","receipt":%s,"runtime":{"code_hash_keccak256":"%s","template_match_verified":true}}' \
      "$index" "${contracts[$index]}" "$(address $((index + 1)))" "$(code_hash $((index + 100)))" "$(receipt)" "$(code_hash $((index + 200)))"
  done
  printf '],"directory_binding_verification":{"active":false,"registered_module_count":7}}\n'
} > "$TMP/deployment.json"
{
  printf '{"schema":"igit.evm-suite.activation-verification.v1","source_commit":"%s","chain_id":1439,"suite_version":3,' "$EXPECTED_COMMIT"
  printf '"state":1,"active":true,"coordinator_activated":true,"registered_module_count":7,"block_number":"0x123","block_hash":"%s",' "$(code_hash 900)"
  printf '"snapshot_root":"%s","directory":"%s","modules":[' "$(code_hash 700)" "$(address 1)"
  for index in 2 3 4 5 6 7 8; do
    (( index == 2 )) || printf ','
    printf '{"contract_name":"%s","address":"%s","observed_code_hash":"%s","directory_code_hash":"%s","directory_verified":true,"module_directory":"%s","bootstrap_finalized":true}' \
      "${contracts[$index]}" "$(address $((index + 1)))" "$(code_hash $((index + 200)))" "$(code_hash $((index + 200)))" "$(address 1)"
  done
  printf ']}\n'
} > "$TMP/suite-verification.json"
{
  printf '{"schema":"igit.evm-suite.blockscout-verification.v1","source_commit":"%s","chain_id":1439,"explorer":"https://fixture.invalid","contracts":[' "$EXPECTED_COMMIT"
  for index in "${!contracts[@]}"; do
    (( index == 0 )) || printf ','
    printf '{"contract_name":"%s","address":"%s","status":"verified"}' "${contracts[$index]}" "$(address $((index + 1)))"
  done
  printf ']}\n'
} > "$TMP/blockscout-verification.json"
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

cp "$TMP/suite-verification.json" "$TMP/suite-verification.json.valid"
sed 's/"active":true/"active":false/' "$TMP/suite-verification.json.valid" > "$TMP/suite-verification.json"
(
  cd "$TMP"
  sha256sum -- "${required[@]}" > cutover-evidence.sha256
)
if bash "$GATE" "$TMP" "$EXPECTED_COMMIT" >/dev/null 2>&1; then
  echo "inactive Suite verification unexpectedly passed" >&2
  exit 1
fi
mv "$TMP/suite-verification.json.valid" "$TMP/suite-verification.json"
(
  cd "$TMP"
  sha256sum -- "${required[@]}" > cutover-evidence.sha256
)

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
