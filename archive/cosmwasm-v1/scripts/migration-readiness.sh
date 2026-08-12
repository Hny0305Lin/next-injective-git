#!/usr/bin/env bash
# Read-only source-readiness gate. It verifies implementation and test-source
# evidence only; it never claims that deployment, migration, security review,
# or clean-machine cutover evidence exists.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
required=0
source_only=0
usage() {
  echo "usage: migration-readiness.sh [--required] [--source-only]" >&2
}
while (($#)); do
  case "$1" in
    --required) required=1 ;;
    --source-only) source_only=1 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
  shift
done

pass=0
skip=0
fail=0
pass_check() { echo "PASS: $*"; pass=$((pass + 1)); }
skip_check() { echo "SKIP: $*"; skip=$((skip + 1)); }
fail_check() { echo "FAIL: $*" >&2; fail=$((fail + 1)); }

for path in \
  "$ROOT/contracts/repo-registry/tests/integration.rs" \
  "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" \
  "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" \
  "$ROOT/contracts/evm-v2/src/RepoRegistryV2BadgeModule.sol" \
  "$ROOT/contracts/evm-v2/src/RepoRegistryV2EconomicModule.sol" \
  "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" \
  "$ROOT/contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol" \
  "$ROOT/contracts/evm-v2/test/RepoRegistryV2BadgeModule.t.sol" \
  "$ROOT/contracts/evm-v2/test/RepoRegistryV2EconomicModule.t.sol" \
  "$ROOT/contracts/evm-v2/test/RepoRegistryV2Invariant.t.sol" \
  "$ROOT/contracts/evm-v2/test/RepoRegistryV2Gas.t.sol" \
  "$ROOT/contracts/evm-v2/abi/RepoRegistryV2.json" \
  "$ROOT/contracts/evm-v2/abi/RepoRegistryV2ImportController.json" \
  "$ROOT/contracts/evm-v2/abi/RepoRegistryV2BadgeModule.json" \
  "$ROOT/contracts/evm-v2/abi/RepoRegistryV2EconomicModule.json" \
  "$ROOT/contracts/evm-v2/package.json" \
  "$ROOT/contracts/evm-v2/package-lock.json" \
  "$ROOT/cli/cmd/igit/setup.go" \
  "$ROOT/cli/cmd/igit/setup_test.go" \
  "$ROOT/cli/cmd/igit-migrate-v1/main.go" \
  "$ROOT/cli/cmd/igit-migrate-v1/main_test.go" \
  "$ROOT/cli/cmd/igit-migrate-v2-state/main.go" \
  "$ROOT/cli/cmd/igit-migrate-v2-state/main_test.go" \
  "$ROOT/cli/cmd/igit-migrate-v2-run/main.go" \
  "$ROOT/cli/cmd/igit-migrate-v2-run/main_test.go" \
  "$ROOT/cli/cmd/igit-release-profile-check/main.go" \
  "$ROOT/cli/internal/bootstrap/bootstrap.go" \
  "$ROOT/cli/internal/bootstrap/bootstrap_integration_test.go" \
  "$ROOT/cli/internal/bootstrap/bootstrap_integration_unix_test.go" \
  "$ROOT/cli/internal/bootstrap/bootstrap_test.go" \
  "$ROOT/cli/internal/bootstrap/deps.json" \
  "$ROOT/cli/internal/bootstrap/process_windows.go" \
  "$ROOT/cli/internal/bootstrap/process_windows_test.go" \
  "$ROOT/cli/internal/chain/backend.go" \
  "$ROOT/cli/internal/chain/client_test.go" \
  "$ROOT/cli/internal/chain/evm_abi.go" \
  "$ROOT/cli/internal/chain/evm_badge.go" \
  "$ROOT/cli/internal/chain/evm_badge_test.go" \
  "$ROOT/cli/internal/chain/evm_economic.go" \
  "$ROOT/cli/internal/chain/evm_economic_test.go" \
  "$ROOT/cli/internal/chain/evm_gas.go" \
  "$ROOT/cli/internal/chain/evm_gas_test.go" \
  "$ROOT/cli/internal/chain/evm_import_state.go" \
  "$ROOT/cli/internal/chain/evm_import_state_test.go" \
  "$ROOT/cli/internal/chain/evm_ownership_test.go" \
  "$ROOT/cli/internal/chain/evm_registry.go" \
  "$ROOT/cli/internal/chain/evm_registry_test.go" \
  "$ROOT/cli/internal/chain/evm_rpc.go" \
  "$ROOT/cli/internal/migration/plan.go" \
  "$ROOT/cli/internal/migration/plan_test.go" \
  "$ROOT/cli/internal/migration/manifest.go" \
  "$ROOT/cli/internal/migration/manifest_test.go" \
  "$ROOT/cli/internal/migration/verify.go" \
  "$ROOT/cli/internal/migration/verify_test.go" \
  "$ROOT/cli/internal/migration/export.go" \
  "$ROOT/cli/internal/migration/export_test.go" \
  "$ROOT/cli/internal/migration/journal.go" \
  "$ROOT/cli/internal/migration/journal_test.go" \
  "$ROOT/cli/internal/migration/runner.go" \
  "$ROOT/cli/internal/migration/runner_test.go" \
  "$ROOT/docs/cosmwasm-v1-protocol.md" \
  "$ROOT/docs/evm-v2-migration.md" \
  "$ROOT/docs/evm-v2-repo-identity.md" \
  "$ROOT/scripts/evm-v2-check.sh" \
  "$ROOT/scripts/evm-v2-check-test.sh" \
  "$ROOT/scripts/evm-v2-foundry-abi-check.mjs" \
  "$ROOT/scripts/evm-v2-solc-check.mjs" \
  "$ROOT/scripts/identity-readiness.mjs" \
  "$ROOT/scripts/migration-cutover-readiness.sh" \
  "$ROOT/scripts/migration-cutover-readiness.ps1" \
  "$ROOT/scripts/migration-cutover-readiness-test.ps1" \
  "$ROOT/scripts/migration-cutover-readiness-test.sh" \
  "$ROOT/scripts/migration-readiness-file-test.ps1" \
  "$ROOT/scripts/migration-readiness.ps1" \
  "$ROOT/scripts/readiness-file.ps1" \
  "$ROOT/scripts/race-check.sh" \
  "$ROOT/scripts/semver-check.mjs" \
  "$ROOT/scripts/semver-check.test.mjs" \
  "$ROOT/scripts/v1-export-snapshot.sh" \
  "$ROOT/scripts/v1-export-snapshot-test.sh" \
  "$ROOT/web/src/lib/chain.ts" \
  "$ROOT/web/src/pages/Repo/index.tsx" \
  "$ROOT/web/test/chain-evm-v2.test.mjs" \
  "$ROOT/web/scripts/release-profile-check.mjs" \
  "$ROOT/web/test/release-profile-check.test.mjs" \
  "$ROOT/web/package-lock.json"; do
  if [[ -s "$path" ]]; then
    pass_check "required file ${path#$ROOT/}"
  else
    fail_check "missing required file ${path#$ROOT/}"
  fi
done

if grep -Fq 'Test-IgitNonEmptyFile -LiteralPath $path' "$ROOT/scripts/migration-readiness.ps1" &&
   grep -Fq 'empty file unexpectedly passed' "$ROOT/scripts/migration-readiness-file-test.ps1"; then
  pass_check "PowerShell required-file gate rejects zero-byte source evidence"
else
  fail_check "PowerShell required-file non-empty regression evidence is incomplete"
fi

if grep -Fq 'func ValidatePublishedProfilesV1Only() error' "$ROOT/cli/internal/config/config.go" &&
   grep -Fq 'config.ValidatePublishedProfilesV1Only()' "$ROOT/cli/cmd/igit-release-profile-check/main.go" &&
   grep -Fq 'go run ./cmd/igit-release-profile-check' "$ROOT/.github/workflows/release.yml" &&
   grep -Fq 'npm run check:release-profile' "$ROOT/.github/workflows/release.yml" &&
   grep -Fq 'isReleaseVersion' "$ROOT/scripts/semver-check.mjs" &&
   grep -Fq 'node --test scripts/semver-check.test.mjs' "$ROOT/.github/workflows/release.yml"; then
  pass_check "release profiles and strict SemVer use dedicated executable fail-closed guards"
else
  fail_check "release profile or strict SemVer guard is incomplete"
fi

solc_lock_valid=false
if command -v node >/dev/null 2>&1; then
  if node -e '
       const fs = require("node:fs");
       const packageJson = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
       const lock = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
       if (packageJson.devDependencies?.solc !== "0.8.24" ||
           lock.packages?.[""]?.devDependencies?.solc !== "0.8.24" ||
           lock.packages?.["node_modules/solc"]?.version !== "0.8.24") process.exit(1);
     ' "$ROOT/contracts/evm-v2/package.json" "$ROOT/contracts/evm-v2/package-lock.json"; then
    solc_lock_valid=true
  fi
elif grep -Eq '"solc"[[:space:]]*:[[:space:]]*"0\.8\.24"' "$ROOT/contracts/evm-v2/package.json" &&
     grep -Fq '"node_modules/solc": {' "$ROOT/contracts/evm-v2/package-lock.json" &&
     grep -A 3 -F '"node_modules/solc": {' "$ROOT/contracts/evm-v2/package-lock.json" |
       grep -Eq '"version"[[:space:]]*:[[:space:]]*"0\.8\.24"'; then
  solc_lock_valid=true
fi
if [[ "$solc_lock_valid" == true ]] &&
   grep -Fq 'const solcScript = resolve(contractDir, "node_modules", "solc", "solc.js")' "$ROOT/scripts/evm-v2-solc-check.mjs" &&
   grep -Fq 'solcPackage.version !== "0.8.24"' "$ROOT/scripts/evm-v2-solc-check.mjs" &&
   grep -Fq 'version: v1.7.1' "$ROOT/.github/workflows/ci.yml" &&
   grep -Fq 'version: v1.7.1' "$ROOT/.github/workflows/release.yml"; then
  pass_check "Foundry and portable solc toolchains are version-pinned"
else
  fail_check "Foundry or portable solc pin is incomplete"
fi

if grep -Fq 'abi/RepoRegistryV2EconomicModule.json empty' "$ROOT/scripts/evm-v2-check-test.sh" &&
   grep -Fq 'test/RepoRegistryV2Invariant.t.sol missing' "$ROOT/scripts/evm-v2-check-test.sh" &&
   grep -Fq 'test/RepoRegistryV2Gas.t.sol missing' "$ROOT/scripts/evm-v2-check-test.sh" &&
   grep -Fq 'stale ABI fixture unexpectedly passed' "$ROOT/scripts/evm-v2-check-test.sh" &&
   grep -Fq 'evm-v2-foundry-abi-check.mjs' "$ROOT/scripts/evm-v2-check.sh" &&
   grep -Fq 'bash scripts/evm-v2-check-test.sh' "$ROOT/.github/workflows/ci.yml" &&
   grep -Fq 'bash scripts/evm-v2-check-test.sh' "$ROOT/.github/workflows/release.yml"; then
  pass_check "EVM V2 required-file gate has executable fail-closed regression coverage"
else
  fail_check "EVM V2 required-file gate regression or workflow wiring is incomplete"
fi

if [[ -s "$ROOT/README.md" ]] &&
   grep -q 'docs/evm-v2-migration.md' "$ROOT/README.md" &&
   grep -q 'docs/evm-v2-repo-identity.md' "$ROOT/README.md" &&
   grep -q 'contracts/evm-v2/' "$ROOT/README.md"; then
  pass_check "README links the V2 migration, identity ADR, and contract package"
else
  fail_check "README is missing V2 migration/identity/package references"
fi

if grep -Fq 'immutable `reverted` journal' "$ROOT/README.md" &&
   grep -Fq 'NNNNNN.reverted.json' "$ROOT/docs/evm-v2-migration.md" &&
   grep -Fq 'confirmation-depth policy' "$ROOT/docs/evm-v2-migration.md" &&
   grep -Fq 'CometBFT-style' "$ROOT/docs/evm-v2-migration.md" &&
   grep -Fq 'finality assumption' "$ROOT/docs/evm-v2-migration.md"; then
  pass_check "reverted receipt evidence and explicit finality policy are documented"
else
  fail_check "reverted receipt evidence or finality policy documentation is incomplete"
fi

if grep -Fq 'openPasswordTerminal = openSystemPasswordTerminal' "$ROOT/cli/internal/chain/evm_keystore.go" &&
   grep -Fq 'CONIN$' "$ROOT/cli/internal/chain/evm_keystore.go" &&
   grep -Fq 'CONOUT$' "$ROOT/cli/internal/chain/evm_keystore.go" &&
   grep -Fq '/dev/tty' "$ROOT/cli/internal/chain/evm_keystore.go" &&
   grep -Fq 'TestReadEVMKeystorePasswordUsesDedicatedTerminalStreams' "$ROOT/cli/internal/chain/evm_keystore_test.go" &&
   grep -Fq 'TestKeyPasswordRejectsProtocolPipeWithoutWritingPrompt' "$ROOT/cli/internal/chain/evm_keystore_test.go" &&
   grep -Fq 'TestRemoteHelperEntrypointKeepsGitProtocolOnStandardStreams' "$ROOT/cli/cmd/git-remote-igit/main_test.go"; then
  pass_check "EVM keystore prompts use a controlling terminal without polluting the Git helper protocol"
else
  fail_check "EVM keystore controlling-terminal isolation or regression evidence is incomplete"
fi

if grep -Fq 'WriteDisabled bool' "$ROOT/cli/internal/chain/backend.go" &&
   grep -Fq 'resolved.WriteDisabled = true' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'legacyReadFallback(cfg)' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'legacyReadFallback is intentionally enabled only' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'TestLegacyReadFallbackStopsPushAndDeleteBeforePreflightOrSideEffects' "$ROOT/cli/internal/remote/push_flow_test.go" &&
   grep -Fq 'TestExplicitEVMSelectionDoesNotEnableLegacyReadFallback' "$ROOT/cli/internal/chain/evm_registry_test.go" &&
   grep -Fq 'TestEVMReadFallbackIsLimitedToAutoV2CompatibilityMode' "$ROOT/cli/internal/chain/backend_test.go"; then
  pass_check "legacy V1 fallback is explicitly read-only and cannot trigger EVM-mode push/delete side effects"
else
  fail_check "legacy fallback write boundary or explicit-EVM no-fallback regression evidence is incomplete"
fi

if grep -Fq 'function updateRepoInfo(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'event RepoInfoUpdated(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'testUpdateRepoInfo' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" &&
   grep -Fq 'UpdateRepoInfo(repo string, description, defaultBranch *string) error' "$ROOT/cli/internal/chain/backend.go" &&
   grep -Fq 'updateRepoInfo(string,bool,string,bool,string)' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'registry.UpdateRepoInfo(repo, description, branch)' "$ROOT/cli/cmd/igit/main.go"; then
  pass_check "repo metadata is wired through V2 source, tests, backend, and CLI"
else
  fail_check "repo metadata V2/backend/CLI wiring is incomplete"
fi

if grep -Fq 'function listReposPage(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'function listRefsPageById(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'function listCollaboratorsPageById(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'func (e *EVMRegistryV2) ListRepos(' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'listRefsPageByID' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'listCollaboratorsPageByID' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'TestClientListReposDrainsV1NamePages' "$ROOT/cli/internal/chain/client_test.go" &&
   grep -Fq 'TestEVMRegistryListReposDrainsBoundedOwnerPages' "$ROOT/cli/internal/chain/evm_registry_test.go" &&
   grep -Fq 'TestEVMRegistryDrainsStableIDPagesWithoutV1Fallback' "$ROOT/cli/internal/chain/evm_registry_test.go" &&
   grep -Fq 'EVM owner repository listing drains bounded pages without V1 fallback' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq 'EVM stable repo ID reads drain two ref and collaborator pages without V1 fallback' "$ROOT/web/test/chain-evm-v2.test.mjs"; then
  pass_check "bounded repository/ref/collaborator pagination is wired through contract, Go, and Web"
else
  fail_check "repository/ref/collaborator pagination implementation or regression evidence is incomplete"
fi

if grep -Fq 'func (r *EVMRPC) BlockNumber(' "$ROOT/cli/internal/chain/evm_rpc.go" &&
   grep -Fq 'CallContractAt' "$ROOT/cli/internal/chain/evm_rpc.go" &&
   grep -Fq 'snapshotBlockTag' "$ROOT/cli/internal/chain/evm_registry.go" &&
   grep -Fq 'evmSnapshotBlockTag' "$ROOT/web/src/lib/chain.ts" &&
   grep -Fq 'eth_blockNumber' "$ROOT/web/test/chain-evm-v2.test.mjs"; then
  pass_check "multi-page V2 enumeration is pinned to one EVM block tag"
else
  fail_check "V2 pagination block snapshot implementation or regression evidence is incomplete"
fi

if grep -Fq 'storedRef.packUris.length >= MAX_PACK_URIS' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'testNormalUpdateCannotGrowStoredPackUrisPastLimit' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" &&
   grep -Fq 'MAX_PACK_URIS = 128' "$ROOT/docs/evm-v2-repo-identity.md"; then
  pass_check "V2 stored pack URI growth is bounded and covered by regression evidence"
else
  fail_check "V2 stored pack URI cumulative limit or regression evidence is missing"
fi

if grep -Fq 'transfer_ownership_preserves_and_resets_v1_extension_state' "$ROOT/contracts/repo-registry/tests/integration.rs" &&
   grep -Fq 'transfer_ownership_is_allowed_while_frozen_or_delisted' "$ROOT/contracts/repo-registry/tests/integration.rs" &&
   grep -Fq 'fork_copies_only_v1_metadata_and_ref_snapshot' "$ROOT/contracts/repo-registry/tests/integration.rs" &&
   grep -Fq 'fork_copies_refs_beyond_the_default_query_page' "$ROOT/contracts/repo-registry/tests/integration.rs" &&
   grep -Fq 'Badges and moderation' "$ROOT/docs/cosmwasm-v1-protocol.md"; then
  pass_check "V1 ownership transfer and fork side effects are frozen by characterization tests"
else
  fail_check "V1 ownership transfer/fork characterization evidence is incomplete"
fi

if grep -Fq 'award_badge_rejects_delisted_repo_like_frozen_repo' "$ROOT/contracts/repo-registry/tests/integration.rs" &&
   grep -Fq 'testDelistedStatusAlsoRejectsBadgeLikeV1' "$ROOT/contracts/evm-v2/test/RepoRegistryV2BadgeModule.t.sol" &&
   grep -Fq 'moderationStatus != STATUS_ACTIVE' "$ROOT/contracts/evm-v2/src/RepoRegistryV2BadgeModule.sol" &&
   grep -Fq 'TestEVMBadgeRecipientPagesUseOneBlockAndCanonicalStableRepoLookup' "$ROOT/cli/internal/chain/evm_badge_test.go" &&
   grep -Fq 'TestEVMBadgeRepoReadFallsBackToV1ButAwardNeverWritesV1' "$ROOT/cli/internal/chain/evm_badge_test.go" &&
   grep -Fq 'EVM badge award targets the module, waits for receipt, and never writes V1' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq 'EVM badge award surfaces a reverted receipt without a V1 write fallback' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq '"badges_by_recipient"' "$ROOT/cli/internal/migration/plan.go"; then
  pass_check "V1 and V2 badge moderation parity is characterized; historical badge import remains explicitly deferred"
else
  fail_check "Badge moderation parity, CLI/Web path, or deferred historical import evidence is incomplete"
fi

if grep -Fq 'TestEVMEconomicSponsorTargetsModuleWithExactValueAndReceiptPipeline' "$ROOT/cli/internal/chain/evm_economic_test.go" &&
   grep -Fq 'TestEVMEconomicSplitsEncodeArraysAndReadsStayAtOneBlock' "$ROOT/cli/internal/chain/evm_economic_test.go" &&
   grep -Fq 'TestEVMEconomicLegacyReadFallbackNeverWritesV1' "$ROOT/cli/internal/chain/evm_economic_test.go" &&
   grep -Fq 'EVM economic sponsor sends exact INJ value, waits for receipt, and never writes V1' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq 'EVM economic sponsor fails closed for missing module and reverted receipt' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq 'EVM economic split update encodes dynamic arrays, waits for receipt, and never writes V1' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq 'EVM economic split update fails closed for missing module and reverted receipt' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq '(.revenue_splits |' "$ROOT/scripts/v1-export-snapshot.sh" &&
   grep -Fq '(.sponsor_totals |' "$ROOT/scripts/v1-export-snapshot.sh" &&
   grep -Fq 'DeferredSections: []string{"repo_extensions"' "$ROOT/cli/internal/migration/plan.go" &&
   grep -Fq 'deferred_sections' "$ROOT/docs/evm-v2-migration.md" &&
   grep -Fq -- '--acknowledge-core-only-import' "$ROOT/docs/evm-v2-migration.md" &&
   grep -Fq 'repo extension/economic/security' "$ROOT/docs/evm-v2-migration.md"; then
  pass_check "Go/Web V2 economic paths are receipt-checked and never write V1; historical economic import remains explicitly deferred"
else
  fail_check "Economic Go/Web receipt, no-write-fallback, fixed-block, or deferred historical import evidence is incomplete"
fi

if grep -Fq 'PlanSchema' "$ROOT/cli/internal/migration/plan.go" &&
   grep -Fq 'Executable:       false' "$ROOT/cli/internal/migration/plan.go" &&
   grep -Fq 'CoreImportScope' "$ROOT/cli/internal/migration/plan.go" &&
   grep -Fq 'DeferredSections' "$ROOT/cli/internal/migration/plan.go" &&
   grep -Fq 'TestBuildPlanIsDeterministicBoundedAndAddressNormalized' "$ROOT/cli/internal/migration/plan_test.go" &&
   grep -Fq 'TestBuildPlanPreservesHistoricalCommitSHAAndMetadataBytes' "$ROOT/cli/internal/migration/plan_test.go" &&
   grep -Fq 'never contacts RPC, signs, or broadcasts' "$ROOT/cli/cmd/igit-migrate-v1/main.go"; then
  pass_check "offline V1 snapshot-to-V2 import planning is deterministic and non-broadcasting"
else
  fail_check "offline V1 import-plan source or regression evidence is incomplete"
fi

if grep -Fq 'function createImportSession(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'function importRepo(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'function importRefs(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'function importCollaborators(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'function importProgress(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'function finalizeImport(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'event ImportFinalized(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'testOrderedImportPreservesHistoricalMetadataAndFinalizes' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" &&
   grep -Fq 'testImportLocksPublicStateAndFailedBatchDoesNotAdvance' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" &&
   grep -Fq 'testImportCountAndFinalizationChecksAreEnforced' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" &&
   grep -Fq 'testNativeRepoPermanentlyClosesImportWindow' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" &&
   grep -Fq 'testImportProgressSurvivesFinalizationAndWindowStaysClosed' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol" &&
   grep -Fq 'testImportedDelistedRepoRemainsWritableAndInvalidStatusFailsClosed' "$ROOT/contracts/evm-v2/test/RepoRegistryV2.t.sol"; then
  pass_check "V2 snapshot import is ordered, observable, one-shot, count-bound, and explicitly finalized"
else
  fail_check "V2 snapshot import state machine or regression evidence is incomplete"
fi

if grep -Fq 'TransactionManifestSchema' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Fq 'func BuildTransactionManifest(' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Fq 'func ImportCommitment(' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Fq 'func ReadTransactionManifest(' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Fq 'func VerifyTransactionManifest(' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Fq 'DeferredSections' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Fq 'EventEmitter' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Eq 'CalldataReady:[[:space:]]+true' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Eq 'Signed:[[:space:]]+false' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Eq 'Broadcast:[[:space:]]+false' "$ROOT/cli/internal/migration/manifest.go" &&
   grep -Fq 'TestBuildTransactionManifestIsDeterministicAndMatchesCheckedABI' "$ROOT/cli/internal/migration/manifest_test.go" &&
   grep -Fq 'TestBuildControllerTransactionManifestUsesControllerABIAndRegistryEvents' "$ROOT/cli/internal/migration/manifest_test.go" &&
   grep -Fq 'TestBuildTransactionManifestRejectsTamperedPlan' "$ROOT/cli/internal/migration/manifest_test.go" &&
   grep -Fq 'TestReadAndVerifyTransactionManifestStrictlyBindsPlan' "$ROOT/cli/internal/migration/manifest_test.go" &&
   grep -Fq 'TestVerifyTransactionManifestRejectsEveryUnboundMutation' "$ROOT/cli/internal/migration/manifest_test.go" &&
   grep -Fq 'controller-contract' "$ROOT/cli/cmd/igit-migrate-v1/main.go" &&
   grep -Fq 'manifest-output' "$ROOT/cli/cmd/igit-migrate-v1/main.go"; then
  pass_check "offline import plan produces commitment-bound controller or registry calldata without signing or broadcast"
else
  fail_check "unsigned import controller/registry manifest generation or regression evidence is incomplete"
fi

if grep -Fq 'os.Link(temporaryName, output)' "$ROOT/cli/cmd/igit-migrate-v1/main.go" &&
   grep -Fq 'syncOutputDirectory' "$ROOT/cli/cmd/igit-migrate-v1/main.go" &&
   grep -Fq 'PartialPublicationError' "$ROOT/cli/cmd/igit-migrate-v1/main.go" &&
   grep -Fq 'PublishedOutputError' "$ROOT/cli/cmd/igit-migrate-v1/main.go" &&
   grep -Fq 'TestWriteSecureOutputNeverOverwritesExistingEvidence' "$ROOT/cli/cmd/igit-migrate-v1/main_test.go" &&
   grep -Fq 'TestValidateNewOutputsRejectsExistingManifestBeforePlanPublication' "$ROOT/cli/cmd/igit-migrate-v1/main_test.go" &&
   grep -Fq 'TestPublishImportArtifactsReportsImmutablePartialPublication' "$ROOT/cli/cmd/igit-migrate-v1/main_test.go" &&
   grep -Fq 'TestPublishImportArtifactsReportsBothPublishedWhenManifestSyncFails' "$ROOT/cli/cmd/igit-migrate-v1/main_test.go" &&
   grep -Fq 'TestWriteSecureOutputReportsPublishedButUnsyncedEvidence' "$ROOT/cli/cmd/igit-migrate-v1/main_test.go" &&
   grep -Fq 'TestWriteSecureOutputPublicationRaceDoesNotClobberWinner' "$ROOT/cli/cmd/igit-migrate-v1/main_test.go"; then
  pass_check "plan and manifest are immutable, no-clobber, directory-synced migration evidence"
else
  fail_check "plan/manifest immutable publication or durability regression evidence is incomplete"
fi

if grep -Fq 'CUTOVER READINESS: PASS' "$ROOT/scripts/migration-cutover-readiness.sh" &&
   grep -Fq 'cutover-evidence.sha256' "$ROOT/scripts/migration-cutover-readiness.sh" &&
   grep -Fq 'security-review.pdf' "$ROOT/scripts/migration-cutover-readiness.sh" &&
   grep -Fq 'windows-clean-e2e.txt' "$ROOT/scripts/migration-cutover-readiness.sh" &&
   grep -Fq 'CUTOVER READINESS: PASS' "$ROOT/scripts/migration-cutover-readiness.ps1" &&
   grep -Fq 'UTF-8 CRLF fixture did not pass' "$ROOT/scripts/migration-cutover-readiness-test.ps1" &&
   grep -Fq 'UTF-8 CRLF fixture did not pass' "$ROOT/scripts/migration-cutover-readiness-test.sh" &&
   grep -Fq 'mismatched expected commit unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.ps1" &&
   grep -Fq 'mismatched expected commit unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.sh" &&
   grep -Fq 'missing extra checksum record unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.ps1" &&
   grep -Fq 'missing extra checksum record unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.sh" &&
   grep -Fq 'case-folded checksum collision unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.ps1" &&
   grep -Fq 'case-folded checksum collision unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.sh" &&
   grep -Fq 'junction/reparse-point evidence path unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.ps1" &&
   grep -Fq 'symbolic-link evidence path unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.sh" &&
   grep -Fq 'root junction/reparse-point evidence directory unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.ps1" &&
   grep -Fq 'root symbolic-link evidence directory unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.sh" &&
   grep -Fq 'tampered evidence unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.ps1" &&
   grep -Fq 'tampered evidence unexpectedly passed' "$ROOT/scripts/migration-cutover-readiness-test.sh"; then
  pass_check "cutover readiness is a separate fail-closed, hash-bound operator evidence gate"
else
  fail_check "cutover evidence gate or its fail-closed regression test is incomplete"
fi

if grep -Fq 'ImportedStateSchema' "$ROOT/cli/internal/migration/verify.go" &&
   grep -Fq 'func ReadImportedState(' "$ROOT/cli/internal/migration/verify.go" &&
   grep -Fq 'func MarshalImportedState(' "$ROOT/cli/internal/migration/verify.go" &&
   grep -Fq 'func VerifyImportedState(' "$ROOT/cli/internal/migration/verify.go" &&
   grep -Fq 'ImportScope' "$ROOT/cli/internal/migration/verify.go" &&
   grep -Fq 'validateFixedBlockTag' "$ROOT/cli/internal/migration/verify.go" &&
   grep -Fq 'TestVerifyImportedStateAcceptsExactStateInAnyEnumerationOrder' "$ROOT/cli/internal/migration/verify_test.go" &&
   grep -Fq 'TestVerifyImportedStateRejectsIdentityAndContentMismatches' "$ROOT/cli/internal/migration/verify_test.go" &&
   grep -Fq 'verify-state' "$ROOT/cli/cmd/igit-migrate-v1/main.go" &&
   grep -Fq 'TestRunStateMismatchWritesNoPlanOrManifest' "$ROOT/cli/cmd/igit-migrate-v1/main_test.go"; then
  pass_check "offline post-import verifier binds one fixed EVM block snapshot to the exact plan before writing outputs"
else
  fail_check "offline post-import state verification or fail-closed CLI evidence is incomplete"
fi

if grep -Fq 'func (e *EVMRegistryV2) ImportProgressAt(' "$ROOT/cli/internal/chain/evm_import_state.go" &&
   grep -Fq 'func (e *EVMRegistryV2) GetRepoByIDAt(' "$ROOT/cli/internal/chain/evm_import_state.go" &&
   grep -Fq 'func (e *EVMRegistryV2) ListRefsByIDAt(' "$ROOT/cli/internal/chain/evm_import_state.go" &&
   grep -Fq 'func (e *EVMRegistryV2) ListCollaboratorsByIDAt(' "$ROOT/cli/internal/chain/evm_import_state.go" &&
   grep -Fq 'TestEVMImportStateReaderPinsEveryContractCall' "$ROOT/cli/internal/chain/evm_import_state_test.go" &&
   grep -Fq 'TestDecodeEVMImportProgressFailsClosed' "$ROOT/cli/internal/chain/evm_import_state_test.go" &&
   grep -Fq 'func (r *EVMRPC) BlockByNumber(' "$ROOT/cli/internal/chain/evm_rpc.go" &&
   grep -Fq 'func ExportImportedState(' "$ROOT/cli/internal/migration/export.go" &&
   grep -Fq 'validateFinalizationLog' "$ROOT/cli/internal/migration/export.go" &&
   grep -Fq 'VerifyImportedState(plan, state)' "$ROOT/cli/internal/migration/export.go" &&
   grep -Fq 'TestExportImportedStatePinsFinalizeReceiptBlockAndVerifiesExactState' "$ROOT/cli/internal/migration/export_test.go" &&
   grep -Fq 'TestExportImportedStateRejectsReceiptProgressStateAndReorgMismatches' "$ROOT/cli/internal/migration/export_test.go" &&
   grep -Fq 'finalize-tx' "$ROOT/cli/cmd/igit-migrate-v2-state/main.go" &&
   grep -Fq 'writeExclusiveArtifact' "$ROOT/cli/cmd/igit-migrate-v2-state/main.go" &&
   grep -Fq 'signed: false' "$ROOT/cli/cmd/igit-migrate-v2-state/main.go" &&
   grep -Fq 'TestRunExporterFailureCreatesNoOutputOrDirectory' "$ROOT/cli/cmd/igit-migrate-v2-state/main_test.go" &&
   grep -Fq 'TestWriteExclusiveArtifactDoesNotOverwriteEvidence' "$ROOT/cli/cmd/igit-migrate-v2-state/main_test.go"; then
  pass_check "receipt-aware V2 exporter pins the finalize block, rejects reorg/state mismatches, and publishes evidence without overwrite"
else
  fail_check "receipt-aware fixed-block exporter, command, or fail-closed regression evidence is incomplete"
fi

if grep -Fq 'ReceiptJournalSchema' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'func OpenReceiptJournal(' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'func InspectReceiptJournal(' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'func (journal *ReceiptJournal) RecordPrepared(' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'func (journal *ReceiptJournal) RecordBroadcast(' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'func (journal *ReceiptJournal) RecordMined(' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'func (journal *ReceiptJournal) RecordReverted(' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'func (journal *ReceiptJournal) RecordRevertedAt(' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'RevertedJournalRecordSchema' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'syncJournalDirectory' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'os.Link(temporaryName, path)' "$ROOT/cli/internal/migration/journal.go" &&
   grep -Fq 'TestReceiptJournalRecordsImmutableOrderedLifecycleAndReopens' "$ROOT/cli/internal/migration/journal_test.go" &&
   grep -Fq 'TestReceiptJournalRecordsRevertedReceiptAsTerminalEvidence' "$ROOT/cli/internal/migration/journal_test.go" &&
   grep -Fq 'TestReceiptJournalAcceptsDirectRegistryImportEvidence' "$ROOT/cli/internal/migration/journal_test.go" &&
   grep -Fq 'func ExecuteImport(' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'SendRawTransaction' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'isAlreadyKnownTransaction' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'revalidateJournalReceipt' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'revalidateRevertedJournalReceipt' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'DefaultImportMaxGasLimit' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'chain.AdjustEVMGasLimit' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'BlockByNumber' "$ROOT/cli/internal/migration/runner.go" &&
   grep -Fq 'func AdjustEVMGasLimit(' "$ROOT/cli/internal/chain/evm_gas.go" &&
   grep -Fq 'TestAdjustEVMGasLimitRejectsTrueOverflowWithoutIntermediateWrap' "$ROOT/cli/internal/chain/evm_gas_test.go" &&
   grep -Fq 'TestExecuteImportJournalsEveryTransitionAndResumesCompletedRun' "$ROOT/cli/internal/migration/runner_test.go" &&
   grep -Fq 'TestExecuteImportStopsOnRevertedOrInvalidReceiptWithBroadcastDurable' "$ROOT/cli/internal/migration/runner_test.go" &&
   grep -Fq 'TestExecuteImportRevalidatesRevertedReceiptEvidence' "$ROOT/cli/internal/migration/runner_test.go" &&
   grep -Fq 'TestIsAlreadyKnownTransactionUsesWordBoundaries' "$ROOT/cli/internal/migration/runner_test.go" &&
   grep -Fq 'TestExecuteImportRejectsIdentityGasAndReorgMismatches' "$ROOT/cli/internal/migration/runner_test.go" &&
   grep -Fq 'confirm-plan-sha256' "$ROOT/cli/cmd/igit-migrate-v2-run/main.go" &&
   grep -Fq 'acknowledge-core-only-import' "$ROOT/cli/cmd/igit-migrate-v2-run/main.go" &&
   grep -Fq 'checkOnly' "$ROOT/cli/cmd/igit-migrate-v2-run/main.go" &&
   grep -Fq 'statusOnly' "$ROOT/cli/cmd/igit-migrate-v2-run/main.go" &&
   grep -Fq 'NewEVMKeystoreSigner' "$ROOT/cli/cmd/igit-migrate-v2-run/main.go" &&
   grep -Fq 'TestRunRequiresExactPlanHashBeforeConfigOrExecution' "$ROOT/cli/cmd/igit-migrate-v2-run/main_test.go" &&
   grep -Fq 'TestRunRequiresExplicitCoreOnlyAcknowledgement' "$ROOT/cli/cmd/igit-migrate-v2-run/main_test.go" &&
   grep -Fq 'TestRunCheckValidatesArtifactsWithoutJournalKeyOrRPC' "$ROOT/cli/cmd/igit-migrate-v2-run/main_test.go" &&
   grep -Fq 'TestRunStatusInspectsJournalWithoutKeyOrRPC' "$ROOT/cli/cmd/igit-migrate-v2-run/main_test.go" &&
   grep -Fq 'TestRunRejectsTamperedManifestBeforeConfirmation' "$ROOT/cli/cmd/igit-migrate-v2-run/main_test.go"; then
  pass_check "confirmed V2 admin runner durably journals signed raw transactions and receipt-checked ordered execution"
else
  fail_check "V2 migration runner, immutable receipt journal, confirmation boundary, or recovery tests are incomplete"
fi

if grep -Fq 'contract RepoRegistryV2ImportController' "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" &&
   grep -Fq 'BATCH_DOMAIN = "igit:v2:import:batch"' "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" &&
   grep -Fq 'function setRegistry(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" &&
   grep -Fq 'function abortImport(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" &&
   grep -Fq 'function setFrozen(' "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" &&
   grep -Fq 'bytes32 public registryCodeHash' "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" &&
   grep -Fq 'candidate.codehash' "$ROOT/contracts/evm-v2/src/RepoRegistryV2ImportController.sol" &&
   grep -Fq 'function importWindowClosed()' "$ROOT/contracts/evm-v2/src/RepoRegistryV2.sol" &&
   grep -Fq 'testOrderedCommitmentFinalizesAndPublishes' "$ROOT/contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol" &&
   grep -Fq 'testPayloadMismatchDoesNotAdvanceControllerCommitment' "$ROOT/contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol" &&
   grep -Fq 'testAbortMovesToFreshRegistryAndOldCommitmentCannotPublish' "$ROOT/contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol" &&
   grep -Fq 'testControllerRejectsRegistryThatAlreadyPublishedState' "$ROOT/contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol" &&
   grep -Fq 'testControllerRejectsRegistryThatAlreadyFinalizedImport' "$ROOT/contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol" &&
   grep -Fq 'testAbortRejectsDifferentRegistryRuntime' "$ROOT/contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol"; then
  pass_check "deployment-level import controller binds ordered commitment and explicit recovery"
else
  fail_check "import controller source or regression evidence is incomplete"
fi

if grep -Fq 'pendingOwnershipTransferWithEvm' "$ROOT/web/src/lib/chain.ts" &&
   grep -Fq 'beginOwnershipTransferWithEvm' "$ROOT/web/src/lib/chain.ts" &&
   grep -Fq 'acceptOwnershipWithEvm' "$ROOT/web/src/lib/chain.ts" &&
   grep -Fq 'expireOwnershipTransferWithEvm' "$ROOT/web/src/lib/chain.ts" &&
   grep -Fq 'Ownership transfer' "$ROOT/web/src/pages/Repo/index.tsx" &&
   grep -Fq 'EVM ownership transfer query decodes pending state and an empty state' "$ROOT/web/test/chain-evm-v2.test.mjs" &&
   grep -Fq 'EVM ownership transfer writes encode every action and wait for receipts' "$ROOT/web/test/chain-evm-v2.test.mjs"; then
  pass_check "Web ownership transfer uses stable repo IDs and receipt-checked EVM actions"
else
  fail_check "Web ownership-transfer query, actions, UI, or regression evidence is incomplete"
fi

if command -v node >/dev/null 2>&1; then
  if node "$ROOT/scripts/identity-readiness.mjs" "$ROOT"; then
    pass_check "stable repoId identity, transfer, client, ABI, and documentation evidence"
  else
    fail_check "stable repoId identity evidence is incomplete"
  fi
elif ((required)); then
  fail_check "Node.js is required for the stable repoId identity gate"
else
  skip_check "Node.js not found; stable repoId identity gate was not run"
fi

if command -v node >/dev/null 2>&1; then
  if node "$ROOT/scripts/evm-v2-solc-check.mjs"; then
    pass_check "Solidity 0.8.24 source and Foundry test compilation"
  else
    fail_check "Solidity 0.8.24 source/test compilation"
  fi
elif ((required)); then
  fail_check "Node.js is required for the Solidity source compilation gate"
else
  skip_check "Node.js not found; Solidity source compilation gate was not run"
fi

if command -v jq >/dev/null 2>&1; then
  if jq -e '
    any(.[];
      .type == "function" and .name == "updateRepoInfo" and
      [.inputs[].type] == ["string", "bool", "string", "bool", "string"] and
      .stateMutability == "nonpayable") and
    any(.[];
      .type == "event" and .name == "RepoInfoUpdated" and
      [.inputs[].type] == ["bytes32", "address", "string", "uint8", "string", "string"])
  ' "$ROOT/contracts/evm-v2/abi/RepoRegistryV2.json" >/dev/null; then
    pass_check "checked-in V2 ABI exposes repo metadata function and event"
  else
    fail_check "checked-in V2 ABI is missing the repo metadata function or event"
  fi
else
  if ((required)); then
    fail_check "jq is required for metadata ABI structure validation"
  else
    skip_check "jq not found; metadata ABI structure is covered by Foundry CI"
  fi
fi

if command -v jq >/dev/null 2>&1; then
  if jq -e '
    any(.[];
      .type == "function" and .name == "createImportSession" and
      [.inputs[].type] == ["bytes32", "string", "address", "uint64", "uint256", "uint256", "uint256", "uint256"] and
      .stateMutability == "nonpayable") and
    any(.[];
      .type == "function" and .name == "importRepo" and
      [.inputs[].type] == ["bytes32", "uint256", "bytes32", "address", "string", "string", "string", "uint8", "uint64", "uint64", "uint256", "uint256", "bytes32"]) and
    any(.[]; .type == "function" and .name == "importRefs" and .inputs[3].type == "tuple[]") and
    any(.[]; .type == "function" and .name == "importCollaborators" and .inputs[3].type == "tuple[]") and
    any(.[];
      .type == "function" and .name == "importProgress" and
      [.inputs[].type] == ["bytes32"] and
      [.outputs[].type] == ["bool", "bool", "bool", "uint256", "uint256", "uint256", "uint256", "uint256", "uint256", "uint256", "uint256", "uint256"]) and
    any(.[]; .type == "function" and .name == "importWindowClosed" and [.outputs[].type] == ["bool"]) and
    any(.[]; .type == "function" and .name == "finalizeImport" and [.inputs[].type] == ["bytes32"]) and
    any(.[]; .type == "event" and .name == "ImportFinalized")
  ' "$ROOT/contracts/evm-v2/abi/RepoRegistryV2.json" >/dev/null; then
    pass_check "checked-in V2 ABI exposes the ordered snapshot import protocol"
  else
    fail_check "checked-in V2 ABI is missing or mismatches the snapshot import protocol"
  fi
else
  if ((required)); then
    fail_check "jq is required for import ABI structure validation"
  else
    skip_check "jq not found; import ABI structure is covered by Go ABI round-trip tests"
  fi
fi

if command -v jq >/dev/null 2>&1; then
  if jq -e '
    any(.[]; .type == "function" and .name == "createImportSession" and
      [.inputs[].type] == ["bytes32", "string", "address", "uint64", "uint256", "uint256", "uint256", "uint256", "bytes32"]) and
    any(.[]; .type == "function" and .name == "abortImport" and [.inputs[].type] == ["bytes32", "address"]) and
    any(.[]; .type == "function" and .name == "registryCodeHash" and [.outputs[].type] == ["bytes32"]) and
    any(.[]; .type == "function" and .name == "setFrozen" and [.inputs[].type] == ["address", "string", "bool"]) and
    any(.[]; .type == "event" and .name == "ImportPublished") and
    any(.[]; .type == "event" and .name == "ImportAborted")
  ' "$ROOT/contracts/evm-v2/abi/RepoRegistryV2ImportController.json" >/dev/null; then
    pass_check "checked-in import controller ABI exposes commitment, recovery, and moderation forwarding"
  else
    fail_check "checked-in import controller ABI is missing commitment or recovery protocol"
  fi
else
  if ((required)); then
    fail_check "jq is required for controller ABI structure validation"
  else
    skip_check "jq not found; controller ABI structure is covered by Go ABI round-trip tests"
  fi
fi

if command -v node >/dev/null 2>&1; then
  if node - "$ROOT" <<'NODE'
const fs = require("node:fs");
const path = require("node:path");

const root = process.argv[2];
const failures = [];
const check = (condition, message) => {
  if (!condition) failures.push(message);
};
const read = (relative) => fs.readFileSync(path.join(root, relative), "utf8");

function blockAt(source, openBrace) {
  let depth = 0;
  let mode = "code";
  for (let i = openBrace; i < source.length; i += 1) {
    const current = source[i];
    const next = source[i + 1];
    if (mode === "line-comment") {
      if (current === "\n") mode = "code";
      continue;
    }
    if (mode === "block-comment") {
      if (current === "*" && next === "/") {
        mode = "code";
        i += 1;
      }
      continue;
    }
    if (mode === "'" || mode === '"' || mode === "`") {
      if (current === "\\") {
        i += 1;
      } else if (current === mode) {
        mode = "code";
      }
      continue;
    }
    if (current === "/" && next === "/") {
      mode = "line-comment";
      i += 1;
    } else if (current === "/" && next === "*") {
      mode = "block-comment";
      i += 1;
    } else if (current === "'" || current === '"' || current === "`") {
      mode = current;
    } else if (current === "{") {
      depth += 1;
    } else if (current === "}") {
      depth -= 1;
      if (depth === 0) return source.slice(openBrace, i + 1);
    }
  }
  throw new Error("unterminated source block");
}

function namedFunction(source, name) {
  const pattern = new RegExp(
    "\\b(?:export\\s+default\\s+|export\\s+)?(?:async\\s+)?function\\s+" + name + "\\s*\\(",
  );
  const match = pattern.exec(source);
  if (!match) throw new Error("missing function " + name);
  const openBrace = source.indexOf("{", match.index + match[0].length);
  if (openBrace < 0) throw new Error("missing body for function " + name);
  return blockAt(source, openBrace);
}

function arrowFunction(source, name) {
  const pattern = new RegExp(
    "\\bconst\\s+" + name + "\\s*=\\s*(?:async\\s*)?\\([^)]*\\)\\s*=>\\s*\\{",
  );
  const match = pattern.exec(source);
  if (!match) throw new Error("missing arrow function " + name);
  return blockAt(source, match.index + match[0].lastIndexOf("{"));
}

function testBodies(source) {
  const tests = [];
  const pattern = /\btest\s*\(\s*(["'])(.*?)\1\s*,/g;
  for (let match; (match = pattern.exec(source)) !== null;) {
    const arrow = source.indexOf("=>", pattern.lastIndex);
    const openBrace = arrow < 0 ? -1 : source.indexOf("{", arrow + 2);
    if (openBrace < 0) throw new Error("missing body for test " + match[2]);
    tests.push({ name: match[2], body: blockAt(source, openBrace) });
  }
  return tests;
}

try {
  const chain = read("web/src/lib/chain.ts");
  const repoPage = read("web/src/pages/Repo/index.tsx");
  const webTests = read("web/test/chain-evm-v2.test.mjs");

  const selectorStart = /const\s+EVM_SELECTORS\s*=\s*\{/.exec(chain);
  check(selectorStart !== null, "chain.ts must declare EVM_SELECTORS");
  const selectorBlock = selectorStart
    ? blockAt(chain, chain.indexOf("{", selectorStart.index))
    : "";
  check(
    /\bupdateRepoInfo\s*:\s*["']1a532166["']/.test(selectorBlock),
    "updateRepoInfo selector must remain 0x1a532166",
  );

  const encoder = namedFunction(chain, "encodeUpdateRepoInfoCall");
  check(/EVM_SELECTORS\.updateRepoInfo/.test(encoder), "metadata encoder must use the pinned selector");
  check(
    /abiBool\s*\(\s*description\s*!==\s*undefined\s*\)/.test(encoder) &&
      /abiBool\s*\(\s*defaultBranch\s*!==\s*undefined\s*\)/.test(encoder),
    "metadata encoder must preserve omitted-versus-empty patch flags",
  );

  const update = namedFunction(chain, "updateRepoInfoWithEvm");
  check(/encodeUpdateRepoInfoCall\s*\(/.test(update), "metadata writer must use the V2 encoder");
  check(/\bsendEvmTransaction\s*\(/.test(update), "metadata writer must use the shared EVM transaction pipeline");
  const sender = namedFunction(chain, "sendEvmTransaction");
  check(/["']eth_sendTransaction["']/.test(sender), "shared EVM transaction pipeline must submit an EVM transaction");
  const receiptIndex = sender.indexOf("await waitForEvmReceipt");
  const cacheIndex = sender.indexOf("clearQueryCache");
  const returnIndex = sender.indexOf("return txHash");
  check(
    receiptIndex >= 0 && cacheIndex > receiptIndex && returnIndex > cacheIndex,
    "shared EVM transaction pipeline must await a successful receipt before cache invalidation and return",
  );
  const forbiddenWriteFallbacks = [
    /\breadWithV1Fallback\s*\(/,
    /\bsmartQuery\s*(?:<[^>]+>)?\s*\(/,
    /\bfetch\s*\(/,
    /cosmwasm\/wasm/i,
    /\bsignAndBroadcast\s*\(/,
    /\.execute\s*\(/,
  ];
  check(
    forbiddenWriteFallbacks.every((pattern) => !pattern.test(update)),
    "metadata writer must not contain a CosmWasm/LCD write fallback",
  );

  check(
    /import\s*\{[\s\S]*?\bupdateRepoInfoWithEvm\b[\s\S]*?\}\s*from\s*["']\.\.\/\.\.\/lib\/chain["']/.test(repoPage),
    "Repo page must import updateRepoInfoWithEvm",
  );
  const repoComponent = namedFunction(repoPage, "Repo");
  const saveMetadata = arrowFunction(repoComponent, "saveMetadata");
  check(
    /await\s+updateRepoInfoWithEvm\s*\(\s*provider\s*,\s*cfg\s*,\s*repo\s*,\s*\{/.test(saveMetadata),
    "Repo save handler must call the EVM metadata writer",
  );
  check(
    /await\s+resolveRepo\s*\(\s*cfg\s*,\s*addr\s*,\s*repo\s*\)/.test(saveMetadata) &&
      /setResolvedRepo\s*\(\s*refreshed\s*\)/.test(saveMetadata) &&
      /setInfo\s*\(\s*refreshed\s*\.\s*info\s*\)/.test(saveMetadata),
    "Repo save handler must refresh identity and displayed metadata after the receipt",
  );
  check(
    !/readWithV1Fallback|smartQuery|cosmwasm\/wasm|signAndBroadcast/.test(saveMetadata),
    "Repo save handler must not introduce a CosmWasm write path",
  );
  check(
    /onClick\s*=\s*\{\s*openMetadataEditor\s*\}/.test(repoComponent) &&
      /onSubmit\s*=\s*\{[\s\S]*?void\s+saveMetadata\s*\(\s*\)/.test(repoComponent) &&
      /<button\b(?=[^>]*className=["']repo-metadata-save["'])(?=[^>]*type=["']submit["'])/.test(repoComponent),
    "Repo page must expose an editor trigger and submit the metadata form",
  );

  const tests = testBodies(webTests);
  const successfulPatch = tests.find(({ body }) =>
    (body.match(/\bupdateRepoInfoWithEvm\s*\(/g) || []).length >= 2 &&
    /description\s*:\s*["']["']/.test(body) &&
    /defaultBranch\s*:\s*undefined/.test(body) &&
    /description\s*:\s*undefined/.test(body) &&
    /defaultBranch\s*:\s*["']["']/.test(body) &&
    /firstData\.slice\s*\(\s*0\s*,\s*10\s*\)\s*,\s*["']0x1a532166["']/.test(body) &&
    /["']eth_getTransactionReceipt["']/.test(body) &&
    /status\s*:\s*["']0x1["']/.test(body),
  );
  check(Boolean(successfulPatch), "web tests must cover patch flags, selector, and a successful receipt");

  const receiptFailure = tests.find(({ body }) =>
    /assert\.rejects\s*\(/.test(body) &&
    /\bupdateRepoInfoWithEvm\s*\(/.test(body) &&
    /["']eth_getTransactionReceipt["']/.test(body) &&
    /status\s*:\s*["']0x0["']/.test(body) &&
    /assert\.equal\s*\(\s*fetchCalls\s*,\s*0\s*\)/.test(body),
  );
  check(Boolean(receiptFailure), "web tests must surface receipt failure without LCD fallback");

  const missingContractFailure = tests.find(({ body }) =>
    /cfg\.evmContract\s*=\s*["']["']/.test(body) &&
    /assert\.rejects\s*\(/.test(body) &&
    /\bupdateRepoInfoWithEvm\s*\(/.test(body) &&
    /assert\.equal\s*\(\s*providerCalls\s*,\s*0\s*\)/.test(body) &&
    /assert\.equal\s*\(\s*fetchCalls\s*,\s*0\s*\)/.test(body),
  );
  check(Boolean(missingContractFailure), "web tests must fail closed when the V2 contract is absent");

  if (failures.length > 0) {
    for (const failure of failures) console.error("web metadata evidence: " + failure);
    process.exit(1);
  }
  console.log("web metadata evidence: structured source and test coverage verified");
} catch (error) {
  console.error("web metadata evidence: " + (error && error.message ? error.message : String(error)));
  process.exit(1);
}
NODE
  then
    pass_check "Web repo metadata write path and regression evidence"
  else
    fail_check "Web repo metadata write path or regression evidence is incomplete"
  fi
elif ((required)); then
  fail_check "Node.js is required for the structured Web metadata source gate"
else
  skip_check "Node.js not found; structured Web metadata source gate was not run"
fi

kubo_sha256="f24e4d24445c8abf7bd26bd034cb9f14dac77e30452731a617d2e8e2f2ceb150"
if command -v jq >/dev/null 2>&1; then
  if jq -e --arg sha "$kubo_sha256" '
    .kubo.version == "0.42.0" and
    (.kubo.artifacts["windows-amd64"] |
      .archive == "zip" and
      .sha256 == $sha and
      .files["kubo/ipfs.exe"] == "ipfs.exe" and
      (.urls | length) >= 2 and
      all(.urls[]; startswith("https://")))
  ' "$ROOT/cli/internal/bootstrap/deps.json" >/dev/null; then
    pass_check "Windows native Kubo artifact is pinned and structurally valid"
  else
    fail_check "Windows native Kubo manifest entry is missing or invalid"
  fi
else
  if ((required)); then
    fail_check "jq is required for Kubo dependency manifest validation"
  else
    skip_check "jq not found; Kubo manifest structure is covered by Go tests and native PowerShell readiness"
  fi
fi

shell_failed=0
while IFS= read -r -d '' script; do
  if ! bash -n "$script"; then
    echo "FAIL: bash syntax ${script#$ROOT/}" >&2
    shell_failed=1
  fi
done < <(find "$ROOT/scripts" -maxdepth 1 -type f -name '*.sh' ! -name 'tmp-*' -print0 | sort -z)
if ((shell_failed == 0)); then
  pass_check "maintained shell scripts parse"
else
  fail=$((fail + 1))
fi

if ((source_only)); then
  skip_check "toolchain tests disabled by --source-only"
elif command -v go >/dev/null 2>&1; then
  if (cd "$ROOT/cli" && go test ./...); then
    pass_check "Go CLI tests"
  else
    fail_check "Go CLI tests"
  fi
elif ((required)); then
  fail_check "Go toolchain not found"
else
  skip_check "Go toolchain not found"
fi

if ((source_only)); then
  skip_check "Web tests disabled by --source-only"
elif command -v npm >/dev/null 2>&1; then
  if (cd "$ROOT/web" && npm run test:api && npm run build); then
    pass_check "Web API tests and production build"
  else
    fail_check "Web API tests and production build"
  fi
elif ((required)); then
  fail_check "npm toolchain not found"
else
  skip_check "npm toolchain not found"
fi

if ((source_only)); then
  skip_check "CosmWasm V1 tests disabled by --source-only"
elif command -v rustup >/dev/null 2>&1; then
  v1_rust_toolchain="${IGIT_V1_RUST_TOOLCHAIN:-}"
  if [[ -z "$v1_rust_toolchain" ]]; then
    active_rust_host=""
    if command -v rustc >/dev/null 2>&1; then
      active_rust_host="$(rustc -vV 2>/dev/null | sed -n 's/^host: //p')"
    fi
    v1_rust_toolchain="1.81.0${active_rust_host:+-$active_rust_host}"
  fi
  rust_version="$(rustup run "$v1_rust_toolchain" rustc --version 2>/dev/null || true)"
  if [[ ! "$rust_version" =~ ^rustc\ 1\.81\. ]]; then
    fail_check "CosmWasm V1 rustup toolchain $v1_rust_toolchain is unavailable or not rustc 1.81.x"
  elif rustup run "$v1_rust_toolchain" cargo test --locked --manifest-path "$ROOT/contracts/repo-registry/Cargo.toml"; then
    pass_check "CosmWasm V1 behavior tests (rustup $v1_rust_toolchain)"
  else
    fail_check "CosmWasm V1 behavior tests (rustup $v1_rust_toolchain)"
  fi
elif command -v cargo >/dev/null 2>&1 && command -v rustc >/dev/null 2>&1; then
  rust_version="$(rustc --version)"
  if [[ ! "$rust_version" =~ ^rustc\ 1\.81\. ]]; then
    fail_check "CosmWasm V1 requires rustc 1.81.x (found: $rust_version; rustup unavailable)"
  elif cargo test --locked --manifest-path "$ROOT/contracts/repo-registry/Cargo.toml"; then
    pass_check "CosmWasm V1 behavior tests"
  else
    fail_check "CosmWasm V1 behavior tests"
  fi
elif ((required)); then
  fail_check "Rust toolchain not found"
else
  skip_check "Rust toolchain not found"
fi

if ((source_only)); then
  skip_check "EVM V2 Foundry checks disabled by --source-only"
elif command -v forge >/dev/null 2>&1; then
  if "$ROOT/scripts/evm-v2-check.sh"; then
    pass_check "EVM V2 Foundry checks"
  else
    fail_check "EVM V2 Foundry checks"
  fi
elif ((required)); then
  fail_check "Foundry (forge) not found"
else
  skip_check "Foundry (forge) not found; use scripts/evm-v2-check.sh --required for release gating"
fi

if ((fail > 0)); then
  echo "MIGRATION SOURCE READINESS: FAIL (pass=$pass skip=$skip fail=$fail)" >&2
  exit 1
fi
echo "MIGRATION SOURCE READINESS: PASS (pass=$pass skip=$skip fail=$fail)"
echo "CUTOVER READINESS: NOT EVALUATED (run scripts/migration-cutover-readiness.sh with reviewed evidence and expected commit)"
