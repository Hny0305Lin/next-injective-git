<#
.SYNOPSIS
  Run the migration readiness gate from native Windows PowerShell.

.DESCRIPTION
  This is the Windows counterpart of migration-readiness.sh. It checks the
  source tree first, then runs toolchain checks that are available locally.
  Missing optional toolchains are reported as SKIP unless -Required is used.
  The script never deploys a contract, broadcasts a transaction, or changes
  the user's configuration.
#>
[CmdletBinding()]
param(
  [switch]$Required,
  [switch]$SourceOnly
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
. (Join-Path $PSScriptRoot "readiness-file.ps1")
$pass = 0
$skip = 0
$fail = 0

function Pass-Check([string]$Message) {
  $script:pass++
  Write-Host "PASS: $Message"
}

function Skip-Check([string]$Message) {
  $script:skip++
  Write-Host "SKIP: $Message"
}

function Fail-Check([string]$Message) {
  $script:fail++
  Write-Host "FAIL: $Message" -ForegroundColor Red
}

function Find-Command([string]$Name) {
  return Get-Command $Name -ErrorAction SilentlyContinue
}

function Invoke-Checked([string]$Name, [string[]]$Arguments, [string]$WorkingDirectory) {
  Push-Location $WorkingDirectory
  $previousErrorAction = $ErrorActionPreference
  try {
    # Native npm/go/cargo tools commonly write progress and warnings to stderr.
    # Capture those lines without turning them into terminating PowerShell
    # errors; the process exit code remains the authoritative result.
    $ErrorActionPreference = "Continue"
    $output = & $Name @Arguments 2>&1
    $exitCode = $LASTEXITCODE
    foreach ($line in $output) {
      Write-Host $line
    }
    return ($exitCode -eq 0)
  } finally {
    $ErrorActionPreference = $previousErrorAction
    Pop-Location
  }
}

$requiredPaths = @(
  "contracts/repo-registry/tests/integration.rs",
  "contracts/evm-v2/src/RepoRegistryV2.sol",
  "contracts/evm-v2/src/RepoRegistryV2ImportController.sol",
  "contracts/evm-v2/src/RepoRegistryV2BadgeModule.sol",
  "contracts/evm-v2/src/RepoRegistryV2EconomicModule.sol",
  "contracts/evm-v2/test/RepoRegistryV2.t.sol",
  "contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol",
  "contracts/evm-v2/test/RepoRegistryV2BadgeModule.t.sol",
  "contracts/evm-v2/test/RepoRegistryV2EconomicModule.t.sol",
  "contracts/evm-v2/test/RepoRegistryV2Invariant.t.sol",
  "contracts/evm-v2/test/RepoRegistryV2Gas.t.sol",
  "contracts/evm-v2/abi/RepoRegistryV2.json",
  "contracts/evm-v2/abi/RepoRegistryV2ImportController.json",
  "contracts/evm-v2/abi/RepoRegistryV2BadgeModule.json",
  "contracts/evm-v2/abi/RepoRegistryV2EconomicModule.json",
  "contracts/evm-v2/package.json",
  "contracts/evm-v2/package-lock.json",
  "cli/cmd/igit/setup.go",
  "cli/cmd/igit/setup_test.go",
  "cli/cmd/igit-migrate-v1/main.go",
  "cli/cmd/igit-migrate-v1/main_test.go",
  "cli/cmd/igit-migrate-v2-state/main.go",
  "cli/cmd/igit-migrate-v2-state/main_test.go",
  "cli/cmd/igit-migrate-v2-run/main.go",
  "cli/cmd/igit-migrate-v2-run/main_test.go",
  "cli/cmd/igit-release-profile-check/main.go",
  "cli/internal/bootstrap/bootstrap.go",
  "cli/internal/bootstrap/bootstrap_integration_test.go",
  "cli/internal/bootstrap/bootstrap_integration_unix_test.go",
  "cli/internal/bootstrap/bootstrap_test.go",
  "cli/internal/bootstrap/deps.json",
  "cli/internal/bootstrap/process_windows.go",
  "cli/internal/bootstrap/process_windows_test.go",
  "cli/internal/chain/backend.go",
  "cli/internal/chain/client_test.go",
  "cli/internal/chain/evm_abi.go",
  "cli/internal/chain/evm_badge.go",
  "cli/internal/chain/evm_badge_test.go",
  "cli/internal/chain/evm_economic.go",
  "cli/internal/chain/evm_economic_test.go",
  "cli/internal/chain/evm_gas.go",
  "cli/internal/chain/evm_gas_test.go",
  "cli/internal/chain/evm_import_state.go",
  "cli/internal/chain/evm_import_state_test.go",
  "cli/internal/chain/evm_ownership_test.go",
  "cli/internal/chain/evm_registry.go",
  "cli/internal/chain/evm_registry_test.go",
  "cli/internal/chain/evm_rpc.go",
  "cli/internal/migration/plan.go",
  "cli/internal/migration/plan_test.go",
  "cli/internal/migration/manifest.go",
  "cli/internal/migration/manifest_test.go",
  "cli/internal/migration/verify.go",
  "cli/internal/migration/verify_test.go",
  "cli/internal/migration/export.go",
  "cli/internal/migration/export_test.go",
  "cli/internal/migration/journal.go",
  "cli/internal/migration/journal_test.go",
  "cli/internal/migration/runner.go",
  "cli/internal/migration/runner_test.go",
  "docs/cosmwasm-v1-protocol.md",
  "docs/evm-v2-migration.md",
  "docs/evm-v2-repo-identity.md",
  "scripts/evm-v2-check.sh",
  "scripts/evm-v2-check-test.sh",
  "scripts/evm-v2-foundry-abi-check.mjs",
  "scripts/evm-v2-solc-check.mjs",
  "scripts/identity-readiness.mjs",
  "scripts/migration-cutover-readiness.sh",
  "scripts/migration-cutover-readiness.ps1",
  "scripts/migration-cutover-readiness-test.ps1",
  "scripts/migration-cutover-readiness-test.sh",
  "scripts/migration-readiness-file-test.ps1",
  "scripts/migration-readiness.ps1",
  "scripts/readiness-file.ps1",
  "scripts/race-check.sh",
  "scripts/semver-check.mjs",
  "scripts/semver-check.test.mjs",
  "scripts/v1-export-snapshot.sh",
  "scripts/v1-export-snapshot-test.sh",
  "web/src/lib/chain.ts",
  "web/src/pages/Repo/index.tsx",
  "web/test/chain-evm-v2.test.mjs",
  "web/scripts/release-profile-check.mjs",
  "web/test/release-profile-check.test.mjs",
  "web/package-lock.json"
)

foreach ($relative in $requiredPaths) {
  $path = Join-Path $root $relative
  if (Test-IgitNonEmptyFile -LiteralPath $path) {
    Pass-Check "required file $relative"
  } else {
    Fail-Check "missing required file $relative"
  }
}

$configSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/config/config.go") -Raw
$profileCheck = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit-release-profile-check/main.go") -Raw
$releaseWorkflow = Get-Content -LiteralPath (Join-Path $root ".github/workflows/release.yml") -Raw
$ciWorkflow = Get-Content -LiteralPath (Join-Path $root ".github/workflows/ci.yml") -Raw
$semverCheck = Get-Content -LiteralPath (Join-Path $root "scripts/semver-check.mjs") -Raw
if (
  $configSource.Contains("func ValidatePublishedProfilesV1Only() error") -and
  $profileCheck.Contains("config.ValidatePublishedProfilesV1Only()") -and
  $releaseWorkflow.Contains("go run ./cmd/igit-release-profile-check") -and
  $releaseWorkflow.Contains("npm run check:release-profile") -and
  $semverCheck.Contains("isReleaseVersion") -and
  $releaseWorkflow.Contains("node --test scripts/semver-check.test.mjs")
) {
  Pass-Check "release profiles and strict SemVer use dedicated executable fail-closed guards"
} else {
  Fail-Check "release profile or strict SemVer guard is incomplete"
}

$evmGateTests = Get-Content -LiteralPath (Join-Path $root "scripts/evm-v2-check-test.sh") -Raw
if (
  $evmGateTests.Contains("abi/RepoRegistryV2EconomicModule.json empty") -and
  $evmGateTests.Contains("test/RepoRegistryV2Invariant.t.sol missing") -and
  $evmGateTests.Contains("test/RepoRegistryV2Gas.t.sol missing") -and
  $evmGateTests.Contains("stale ABI fixture unexpectedly passed") -and
  (Get-Content -LiteralPath (Join-Path $root "scripts/evm-v2-check.sh") -Raw).Contains("evm-v2-foundry-abi-check.mjs") -and
  $ciWorkflow.Contains("bash scripts/evm-v2-check-test.sh") -and
  $releaseWorkflow.Contains("bash scripts/evm-v2-check-test.sh")
) {
  Pass-Check "EVM V2 required-file gate has executable fail-closed regression coverage"
} else {
  Fail-Check "EVM V2 required-file gate regression or workflow wiring is incomplete"
}

$solcPackage = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/package.json") -Raw | ConvertFrom-Json
$solcLock = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/package-lock.json") -Raw
$solcScript = Get-Content -LiteralPath (Join-Path $root "scripts/evm-v2-solc-check.mjs") -Raw
if (
  $solcPackage.devDependencies.solc -eq "0.8.24" -and
  $solcLock -match '(?s)"node_modules/solc"\s*:\s*\{.*?"version"\s*:\s*"0\.8\.24"' -and
  $solcScript.Contains('const solcScript = resolve(contractDir, "node_modules", "solc", "solc.js")') -and
  $solcScript.Contains('solcPackage.version !== "0.8.24"') -and
  $ciWorkflow.Contains("version: v1.7.1") -and
  $releaseWorkflow.Contains("version: v1.7.1")
) {
  Pass-Check "Foundry and portable solc toolchains are version-pinned"
} else {
  Fail-Check "Foundry or portable solc pin is incomplete"
}

try {
  & (Join-Path $root "scripts/migration-readiness-file-test.ps1")
  Pass-Check "PowerShell required-file gate rejects missing, directory, and zero-byte evidence"
} catch {
  Fail-Check "PowerShell required-file regression: $($_.Exception.Message)"
}

$readmePath = Join-Path $root "README.md"
$readme = if (Test-Path -LiteralPath $readmePath) {
  Get-Content -LiteralPath $readmePath -Raw
} else {
  ""
}
if (
  $readme.Contains("docs/evm-v2-migration.md") -and
  $readme.Contains("docs/evm-v2-repo-identity.md") -and
  $readme.Contains("contracts/evm-v2/")
) {
  Pass-Check "README links the V2 migration, identity ADR, and contract package"
} else {
  Fail-Check "README is missing V2 migration/identity/package references"
}

$migrationGuidePath = Join-Path $root "docs/evm-v2-migration.md"
$migrationGuide = if (Test-Path -LiteralPath $migrationGuidePath) {
  Get-Content -LiteralPath $migrationGuidePath -Raw
} else {
  ""
}
if (
  $readme.Contains('immutable `reverted` journal') -and
  $migrationGuide.Contains("NNNNNN.reverted.json") -and
  $migrationGuide.Contains("confirmation-depth policy") -and
  $migrationGuide.Contains("CometBFT-style") -and
  $migrationGuide.Contains("finality assumption")
) {
  Pass-Check "reverted receipt evidence and explicit finality policy are documented"
} else {
  Fail-Check "reverted receipt evidence or finality policy documentation is incomplete"
}

$keystoreSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_keystore.go") -Raw
$keystoreTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_keystore_test.go") -Raw
$remoteHelperTests = Get-Content -LiteralPath (Join-Path $root "cli/cmd/git-remote-igit/main_test.go") -Raw
if (
  $keystoreSource.Contains("openPasswordTerminal = openSystemPasswordTerminal") -and
  $keystoreSource.Contains('CONIN$') -and
  $keystoreSource.Contains('CONOUT$') -and
  $keystoreSource.Contains('/dev/tty') -and
  $keystoreTests.Contains("TestReadEVMKeystorePasswordUsesDedicatedTerminalStreams") -and
  $keystoreTests.Contains("TestKeyPasswordRejectsProtocolPipeWithoutWritingPrompt") -and
  $remoteHelperTests.Contains("TestRemoteHelperEntrypointKeepsGitProtocolOnStandardStreams")
) {
  Pass-Check "EVM keystore prompts use a controlling terminal without polluting the Git helper protocol"
} else {
  Fail-Check "EVM keystore controlling-terminal isolation or regression evidence is incomplete"
}

$backendSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/backend.go") -Raw
$evmRegistrySource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_registry.go") -Raw
$remotePushTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/remote/push_flow_test.go") -Raw
$evmRegistryTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_registry_test.go") -Raw
$backendTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/backend_test.go") -Raw
if (
  $backendSource.Contains("WriteDisabled bool") -and
  $evmRegistrySource.Contains("resolved.WriteDisabled = true") -and
  $evmRegistrySource.Contains("legacyReadFallback(cfg)") -and
  $evmRegistrySource.Contains("legacyReadFallback is intentionally enabled only") -and
  $remotePushTests.Contains("TestLegacyReadFallbackStopsPushAndDeleteBeforePreflightOrSideEffects") -and
  $evmRegistryTests.Contains("TestExplicitEVMSelectionDoesNotEnableLegacyReadFallback") -and
  $backendTests.Contains("TestEVMReadFallbackIsLimitedToAutoV2CompatibilityMode")
) {
  Pass-Check "legacy V1 fallback is explicitly read-only and cannot trigger EVM-mode push/delete side effects"
} else {
  Fail-Check "legacy fallback write boundary or explicit-EVM no-fallback regression evidence is incomplete"
}

$migrationPlanSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/plan.go") -Raw
$migrationPlanTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/plan_test.go") -Raw
$migrationCommand = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit-migrate-v1/main.go") -Raw
if (
  $migrationPlanSource.Contains("PlanSchema") -and
  $migrationPlanSource.Contains("Executable:       false") -and
  $migrationPlanSource.Contains("CoreImportScope") -and
  $migrationPlanSource.Contains("DeferredSections") -and
  $migrationPlanTests.Contains("TestBuildPlanIsDeterministicBoundedAndAddressNormalized") -and
  $migrationPlanTests.Contains("TestBuildPlanPreservesHistoricalCommitSHAAndMetadataBytes") -and
  $migrationCommand.Contains("never contacts RPC, signs, or broadcasts")
) {
  Pass-Check "offline V1 snapshot-to-V2 import planning is deterministic and non-broadcasting"
} else {
  Fail-Check "offline V1 import-plan source or regression evidence is incomplete"
}

$migrationManifestSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/manifest.go") -Raw
$migrationManifestTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/manifest_test.go") -Raw
if (
  $migrationManifestSource.Contains("TransactionManifestSchema") -and
  $migrationManifestSource.Contains("func BuildTransactionManifest(") -and
  $migrationManifestSource.Contains("func ImportCommitment(") -and
  $migrationManifestSource.Contains("func ReadTransactionManifest(") -and
  $migrationManifestSource.Contains("func VerifyTransactionManifest(") -and
  $migrationManifestSource.Contains("DeferredSections") -and
  $migrationManifestSource.Contains("EventEmitter") -and
  $migrationManifestSource -match "CalldataReady:\s+true" -and
  $migrationManifestSource -match "Signed:\s+false" -and
  $migrationManifestSource -match "Broadcast:\s+false" -and
  $migrationManifestTests.Contains("TestBuildTransactionManifestIsDeterministicAndMatchesCheckedABI") -and
  $migrationManifestTests.Contains("TestBuildControllerTransactionManifestUsesControllerABIAndRegistryEvents") -and
  $migrationManifestTests.Contains("TestBuildTransactionManifestRejectsTamperedPlan") -and
  $migrationManifestTests.Contains("TestReadAndVerifyTransactionManifestStrictlyBindsPlan") -and
  $migrationManifestTests.Contains("TestVerifyTransactionManifestRejectsEveryUnboundMutation") -and
  $migrationCommand.Contains("controller-contract") -and
  $migrationCommand.Contains("manifest-output")
) {
  Pass-Check "offline import plan produces commitment-bound controller or registry calldata without signing or broadcast"
} else {
  Fail-Check "unsigned import controller/registry manifest generation or regression evidence is incomplete"
}

$migrationCommandTests = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit-migrate-v1/main_test.go") -Raw
if (
  $migrationCommand.Contains("os.Link(temporaryName, output)") -and
  $migrationCommand.Contains("syncOutputDirectory") -and
  $migrationCommand.Contains("PartialPublicationError") -and
  $migrationCommand.Contains("PublishedOutputError") -and
  $migrationCommandTests.Contains("TestWriteSecureOutputNeverOverwritesExistingEvidence") -and
  $migrationCommandTests.Contains("TestValidateNewOutputsRejectsExistingManifestBeforePlanPublication") -and
  $migrationCommandTests.Contains("TestPublishImportArtifactsReportsImmutablePartialPublication") -and
  $migrationCommandTests.Contains("TestPublishImportArtifactsReportsBothPublishedWhenManifestSyncFails") -and
  $migrationCommandTests.Contains("TestWriteSecureOutputReportsPublishedButUnsyncedEvidence") -and
  $migrationCommandTests.Contains("TestWriteSecureOutputPublicationRaceDoesNotClobberWinner")
) {
  Pass-Check "plan and manifest are immutable, no-clobber, directory-synced migration evidence"
} else {
  Fail-Check "plan/manifest immutable publication or durability regression evidence is incomplete"
}

$cutoverShell = Get-Content -LiteralPath (Join-Path $root "scripts/migration-cutover-readiness.sh") -Raw
$cutoverPowerShell = Get-Content -LiteralPath (Join-Path $root "scripts/migration-cutover-readiness.ps1") -Raw
$cutoverTests = Get-Content -LiteralPath (Join-Path $root "scripts/migration-cutover-readiness-test.ps1") -Raw
$cutoverShellTests = Get-Content -LiteralPath (Join-Path $root "scripts/migration-cutover-readiness-test.sh") -Raw
if (
  $cutoverShell.Contains("CUTOVER READINESS: PASS") -and
  $cutoverShell.Contains("cutover-evidence.sha256") -and
  $cutoverShell.Contains("security-review.pdf") -and
  $cutoverShell.Contains("windows-clean-e2e.txt") -and
  $cutoverPowerShell.Contains("CUTOVER READINESS: PASS") -and
  $cutoverTests.Contains("UTF-8 CRLF fixture did not pass") -and
  $cutoverShellTests.Contains("UTF-8 CRLF fixture did not pass") -and
  $cutoverTests.Contains("mismatched expected commit unexpectedly passed") -and
  $cutoverShellTests.Contains("mismatched expected commit unexpectedly passed") -and
  $cutoverTests.Contains("missing extra checksum record unexpectedly passed") -and
  $cutoverShellTests.Contains("missing extra checksum record unexpectedly passed") -and
  $cutoverTests.Contains("case-folded checksum collision unexpectedly passed") -and
  $cutoverShellTests.Contains("case-folded checksum collision unexpectedly passed") -and
  $cutoverTests.Contains("junction/reparse-point evidence path unexpectedly passed") -and
  $cutoverShellTests.Contains("symbolic-link evidence path unexpectedly passed") -and
  $cutoverTests.Contains("root junction/reparse-point evidence directory unexpectedly passed") -and
  $cutoverShellTests.Contains("root symbolic-link evidence directory unexpectedly passed") -and
  $cutoverTests.Contains("tampered evidence unexpectedly passed") -and
  $cutoverShellTests.Contains("tampered evidence unexpectedly passed")
) {
  Pass-Check "cutover readiness is a separate fail-closed, hash-bound operator evidence gate"
} else {
  Fail-Check "cutover evidence gate or its fail-closed regression test is incomplete"
}

$migrationVerifySource = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/verify.go") -Raw
$migrationVerifyTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/verify_test.go") -Raw
if (
  $migrationVerifySource.Contains("ImportedStateSchema") -and
  $migrationVerifySource.Contains("func ReadImportedState(") -and
  $migrationVerifySource.Contains("func MarshalImportedState(") -and
  $migrationVerifySource.Contains("func VerifyImportedState(") -and
  $migrationVerifySource.Contains("ImportScope") -and
  $migrationVerifySource.Contains("validateFixedBlockTag") -and
  $migrationVerifyTests.Contains("TestVerifyImportedStateAcceptsExactStateInAnyEnumerationOrder") -and
  $migrationVerifyTests.Contains("TestVerifyImportedStateRejectsIdentityAndContentMismatches") -and
  $migrationCommand.Contains("verify-state") -and
  $migrationCommandTests.Contains("TestRunStateMismatchWritesNoPlanOrManifest")
) {
  Pass-Check "offline post-import verifier binds one fixed EVM block snapshot to the exact plan before writing outputs"
} else {
  Fail-Check "offline post-import state verification or fail-closed CLI evidence is incomplete"
}

$migrationStateReaderSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_import_state.go") -Raw
$migrationStateReaderTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_import_state_test.go") -Raw
$migrationRpcSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_rpc.go") -Raw
$migrationExportSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/export.go") -Raw
$migrationExportTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/export_test.go") -Raw
$migrationExportCommand = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit-migrate-v2-state/main.go") -Raw
$migrationExportCommandTests = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit-migrate-v2-state/main_test.go") -Raw
if (
  $migrationStateReaderSource.Contains("func (e *EVMRegistryV2) ImportProgressAt(") -and
  $migrationStateReaderSource.Contains("func (e *EVMRegistryV2) GetRepoByIDAt(") -and
  $migrationStateReaderSource.Contains("func (e *EVMRegistryV2) ListRefsByIDAt(") -and
  $migrationStateReaderSource.Contains("func (e *EVMRegistryV2) ListCollaboratorsByIDAt(") -and
  $migrationStateReaderTests.Contains("TestEVMImportStateReaderPinsEveryContractCall") -and
  $migrationStateReaderTests.Contains("TestDecodeEVMImportProgressFailsClosed") -and
  $migrationRpcSource.Contains("func (r *EVMRPC) BlockByNumber(") -and
  $migrationExportSource.Contains("func ExportImportedState(") -and
  $migrationExportSource.Contains("validateFinalizationLog") -and
  $migrationExportSource.Contains("VerifyImportedState(plan, state)") -and
  $migrationExportTests.Contains("TestExportImportedStatePinsFinalizeReceiptBlockAndVerifiesExactState") -and
  $migrationExportTests.Contains("TestExportImportedStateRejectsReceiptProgressStateAndReorgMismatches") -and
  $migrationExportCommand.Contains("finalize-tx") -and
  $migrationExportCommand.Contains("writeExclusiveArtifact") -and
  $migrationExportCommand.Contains("signed: false") -and
  $migrationExportCommandTests.Contains("TestRunExporterFailureCreatesNoOutputOrDirectory") -and
  $migrationExportCommandTests.Contains("TestWriteExclusiveArtifactDoesNotOverwriteEvidence")
) {
  Pass-Check "receipt-aware V2 exporter pins the finalize block, rejects reorg/state mismatches, and publishes evidence without overwrite"
} else {
  Fail-Check "receipt-aware fixed-block exporter, command, or fail-closed regression evidence is incomplete"
}

$migrationJournalSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/journal.go") -Raw
$migrationJournalTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/journal_test.go") -Raw
$migrationRunnerSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/runner.go") -Raw
$migrationRunnerTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/runner_test.go") -Raw
$migrationRunnerCommand = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit-migrate-v2-run/main.go") -Raw
$migrationRunnerCommandTests = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit-migrate-v2-run/main_test.go") -Raw
$evmGasSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_gas.go") -Raw
$evmGasTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_gas_test.go") -Raw
if (
  $migrationJournalSource.Contains("ReceiptJournalSchema") -and
  $migrationJournalSource.Contains("func OpenReceiptJournal(") -and
  $migrationJournalSource.Contains("func InspectReceiptJournal(") -and
  $migrationJournalSource.Contains("func (journal *ReceiptJournal) RecordPrepared(") -and
  $migrationJournalSource.Contains("func (journal *ReceiptJournal) RecordBroadcast(") -and
  $migrationJournalSource.Contains("func (journal *ReceiptJournal) RecordMined(") -and
  $migrationJournalSource.Contains("func (journal *ReceiptJournal) RecordReverted(") -and
  $migrationJournalSource.Contains("func (journal *ReceiptJournal) RecordRevertedAt(") -and
  $migrationJournalSource.Contains("RevertedJournalRecordSchema") -and
  $migrationJournalSource.Contains("syncJournalDirectory") -and
  $migrationJournalSource.Contains("os.Link(temporaryName, path)") -and
  $migrationJournalTests.Contains("TestReceiptJournalRecordsImmutableOrderedLifecycleAndReopens") -and
  $migrationJournalTests.Contains("TestReceiptJournalRecordsRevertedReceiptAsTerminalEvidence") -and
  $migrationJournalTests.Contains("TestReceiptJournalAcceptsDirectRegistryImportEvidence") -and
  $migrationRunnerSource.Contains("func ExecuteImport(") -and
  $migrationRunnerSource.Contains("SendRawTransaction") -and
  $migrationRunnerSource.Contains("isAlreadyKnownTransaction") -and
  $migrationRunnerSource.Contains("revalidateJournalReceipt") -and
  $migrationRunnerSource.Contains("revalidateRevertedJournalReceipt") -and
  $migrationRunnerSource.Contains("DefaultImportMaxGasLimit") -and
  $migrationRunnerSource.Contains("chain.AdjustEVMGasLimit") -and
  $migrationRunnerSource.Contains("BlockByNumber") -and
  $evmGasSource.Contains("func AdjustEVMGasLimit(") -and
  $evmGasTests.Contains("TestAdjustEVMGasLimitRejectsTrueOverflowWithoutIntermediateWrap") -and
  $migrationRunnerTests.Contains("TestExecuteImportJournalsEveryTransitionAndResumesCompletedRun") -and
  $migrationRunnerTests.Contains("TestExecuteImportStopsOnRevertedOrInvalidReceiptWithBroadcastDurable") -and
  $migrationRunnerTests.Contains("TestExecuteImportRevalidatesRevertedReceiptEvidence") -and
  $migrationRunnerTests.Contains("TestIsAlreadyKnownTransactionUsesWordBoundaries") -and
  $migrationRunnerTests.Contains("TestExecuteImportRejectsIdentityGasAndReorgMismatches") -and
  $migrationRunnerCommand.Contains("confirm-plan-sha256") -and
  $migrationRunnerCommand.Contains("acknowledge-core-only-import") -and
  $migrationRunnerCommand.Contains("checkOnly") -and
  $migrationRunnerCommand.Contains("statusOnly") -and
  $migrationRunnerCommand.Contains("NewEVMKeystoreSigner") -and
  $migrationRunnerCommandTests.Contains("TestRunRequiresExactPlanHashBeforeConfigOrExecution") -and
  $migrationRunnerCommandTests.Contains("TestRunRequiresExplicitCoreOnlyAcknowledgement") -and
  $migrationRunnerCommandTests.Contains("TestRunCheckValidatesArtifactsWithoutJournalKeyOrRPC") -and
  $migrationRunnerCommandTests.Contains("TestRunStatusInspectsJournalWithoutKeyOrRPC") -and
  $migrationRunnerCommandTests.Contains("TestRunRejectsTamperedManifestBeforeConfirmation")
) {
  Pass-Check "confirmed V2 admin runner durably journals signed raw transactions and receipt-checked ordered execution"
} else {
  Fail-Check "V2 migration runner, immutable receipt journal, confirmation boundary, or recovery tests are incomplete"
}

$evmSource = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/src/RepoRegistryV2.sol") -Raw
$evmTests = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/test/RepoRegistryV2.t.sol") -Raw
$controllerSource = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/src/RepoRegistryV2ImportController.sol") -Raw
$controllerTests = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/test/RepoRegistryV2ImportController.t.sol") -Raw
$badgeSource = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/src/RepoRegistryV2BadgeModule.sol") -Raw
$badgeTests = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/test/RepoRegistryV2BadgeModule.t.sol") -Raw
$v1Tests = Get-Content -LiteralPath (Join-Path $root "contracts/repo-registry/tests/integration.rs") -Raw
$backendSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/backend.go") -Raw
$badgeBackendTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_badge_test.go") -Raw
$economicBackendTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_economic_test.go") -Raw
$migrationPlanSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/migration/plan.go") -Raw
$badgeWebTests = Get-Content -LiteralPath (Join-Path $root "web/test/chain-evm-v2.test.mjs") -Raw
$v1ExportSource = Get-Content -LiteralPath (Join-Path $root "scripts/v1-export-snapshot.sh") -Raw
$evmBackendSource = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_registry.go") -Raw
$evmBackendTests = Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_registry_test.go") -Raw
$cliSource = Get-Content -LiteralPath (Join-Path $root "cli/cmd/igit/main.go") -Raw
if (
  $evmSource.Contains("function createImportSession(") -and
  $evmSource.Contains("function importRepo(") -and
  $evmSource.Contains("function importRefs(") -and
  $evmSource.Contains("function importCollaborators(") -and
  $evmSource.Contains("function importProgress(") -and
  $evmSource.Contains("function finalizeImport(") -and
  $evmSource.Contains("event ImportFinalized(") -and
  $evmTests.Contains("testOrderedImportPreservesHistoricalMetadataAndFinalizes") -and
  $evmTests.Contains("testImportLocksPublicStateAndFailedBatchDoesNotAdvance") -and
  $evmTests.Contains("testImportCountAndFinalizationChecksAreEnforced") -and
  $evmTests.Contains("testNativeRepoPermanentlyClosesImportWindow") -and
  $evmTests.Contains("testImportProgressSurvivesFinalizationAndWindowStaysClosed") -and
  $evmTests.Contains("testImportedDelistedRepoRemainsWritableAndInvalidStatusFailsClosed")
) {
  Pass-Check "V2 snapshot import is ordered, observable, one-shot, count-bound, and explicitly finalized"
} else {
  Fail-Check "V2 snapshot import state machine or regression evidence is incomplete"
}

if (
  $v1Tests.Contains("award_badge_rejects_delisted_repo_like_frozen_repo") -and
  $badgeTests.Contains("testDelistedStatusAlsoRejectsBadgeLikeV1") -and
  $badgeSource.Contains("moderationStatus != STATUS_ACTIVE") -and
  $badgeBackendTests.Contains("TestEVMBadgeRecipientPagesUseOneBlockAndCanonicalStableRepoLookup") -and
  $badgeBackendTests.Contains("TestEVMBadgeRepoReadFallsBackToV1ButAwardNeverWritesV1") -and
  $badgeWebTests.Contains("EVM badge award targets the module, waits for receipt, and never writes V1") -and
  $badgeWebTests.Contains("EVM badge award surfaces a reverted receipt without a V1 write fallback") -and
  $migrationPlanSource.Contains('"badges_by_recipient"')
) {
  Pass-Check "V1 and V2 badge moderation parity is characterized; historical badge import remains explicitly deferred"
} else {
  Fail-Check "Badge moderation parity, CLI/Web path, or deferred historical import evidence is incomplete"
}
if (
  $economicBackendTests.Contains("TestEVMEconomicSponsorTargetsModuleWithExactValueAndReceiptPipeline") -and
  $economicBackendTests.Contains("TestEVMEconomicSplitsEncodeArraysAndReadsStayAtOneBlock") -and
  $economicBackendTests.Contains("TestEVMEconomicLegacyReadFallbackNeverWritesV1") -and
  $badgeWebTests.Contains("EVM economic sponsor sends exact INJ value, waits for receipt, and never writes V1") -and
  $badgeWebTests.Contains("EVM economic sponsor fails closed for missing module and reverted receipt") -and
  $badgeWebTests.Contains("EVM economic split update encodes dynamic arrays, waits for receipt, and never writes V1") -and
  $badgeWebTests.Contains("EVM economic split update fails closed for missing module and reverted receipt") -and
  $v1ExportSource.Contains("(.revenue_splits |") -and
  $v1ExportSource.Contains("(.sponsor_totals |") -and
  $migrationPlanSource.Contains('DeferredSections: []string{"repo_extensions"') -and
  $migrationGuide.Contains("deferred_sections") -and
  $migrationGuide.Contains("--acknowledge-core-only-import") -and
  $migrationGuide.Contains("repo extension/economic/security")
) {
  Pass-Check "Go/Web V2 economic paths are receipt-checked and never write V1; historical economic import remains explicitly deferred"
} else {
  Fail-Check "Economic Go/Web receipt, no-write-fallback, fixed-block, or deferred historical import evidence is incomplete"
}
if (
  $controllerSource.Contains("contract RepoRegistryV2ImportController") -and
  $controllerSource.Contains('BATCH_DOMAIN = "igit:v2:import:batch"') -and
  $controllerSource.Contains("function setRegistry(") -and
  $controllerSource.Contains("function abortImport(") -and
  $controllerSource.Contains("function setFrozen(") -and
  $controllerSource.Contains("bytes32 public registryCodeHash") -and
  $controllerSource.Contains("candidate.codehash") -and
  $evmSource.Contains("function importWindowClosed()") -and
  $controllerTests.Contains("testOrderedCommitmentFinalizesAndPublishes") -and
  $controllerTests.Contains("testPayloadMismatchDoesNotAdvanceControllerCommitment") -and
  $controllerTests.Contains("testAbortMovesToFreshRegistryAndOldCommitmentCannotPublish") -and
  $controllerTests.Contains("testControllerRejectsRegistryThatAlreadyPublishedState") -and
  $controllerTests.Contains("testControllerRejectsRegistryThatAlreadyFinalizedImport") -and
  $controllerTests.Contains("testAbortRejectsDifferentRegistryRuntime")
) {
  Pass-Check "deployment-level import controller binds ordered commitment and explicit recovery"
} else {
  Fail-Check "import controller source or regression evidence is incomplete"
}
if (
  $evmSource.Contains("function updateRepoInfo(") -and
  $evmSource.Contains("event RepoInfoUpdated(") -and
  $evmTests.Contains("testUpdateRepoInfo") -and
  $backendSource.Contains("UpdateRepoInfo(repo string, description, defaultBranch *string) error") -and
  $evmBackendSource.Contains("updateRepoInfo(string,bool,string,bool,string)") -and
  $cliSource.Contains("registry.UpdateRepoInfo(repo, description, branch)")
) {
  Pass-Check "repo metadata is wired through V2 source, tests, backend, and CLI"
} else {
  Fail-Check "repo metadata V2/backend/CLI wiring is incomplete"
}

$webTests = Get-Content -LiteralPath (Join-Path $root "web/test/chain-evm-v2.test.mjs") -Raw
$webChainSource = Get-Content -LiteralPath (Join-Path $root "web/src/lib/chain.ts") -Raw
$webRepoSource = Get-Content -LiteralPath (Join-Path $root "web/src/pages/Repo/index.tsx") -Raw
if (
  $webChainSource.Contains("pendingOwnershipTransferWithEvm") -and
  $webChainSource.Contains("beginOwnershipTransferWithEvm") -and
  $webChainSource.Contains("acceptOwnershipWithEvm") -and
  $webChainSource.Contains("expireOwnershipTransferWithEvm") -and
  $webRepoSource.Contains("Ownership transfer") -and
  $webTests.Contains("EVM ownership transfer query decodes pending state and an empty state") -and
  $webTests.Contains("EVM ownership transfer writes encode every action and wait for receipts")
) {
  Pass-Check "Web ownership transfer uses stable repo IDs and receipt-checked EVM actions"
} else {
  Fail-Check "Web ownership-transfer query, actions, UI, or regression evidence is incomplete"
}
if (
  $evmSource.Contains("function listReposPage(") -and
  $evmSource.Contains("function listRefsPageById(") -and
  $evmSource.Contains("function listCollaboratorsPageById(") -and
  $evmBackendSource.Contains("func (e *EVMRegistryV2) ListRepos(") -and
  $evmBackendSource.Contains("listRefsPageByID") -and
  $evmBackendSource.Contains("listCollaboratorsPageByID") -and
  (Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/client_test.go") -Raw).Contains("TestClientListReposDrainsV1NamePages") -and
  $evmBackendTests.Contains("TestEVMRegistryListReposDrainsBoundedOwnerPages") -and
  $evmBackendTests.Contains("TestEVMRegistryDrainsStableIDPagesWithoutV1Fallback") -and
  $webTests.Contains("EVM owner repository listing drains bounded pages without V1 fallback") -and
  $webTests.Contains("EVM stable repo ID reads drain two ref and collaborator pages without V1 fallback")
) {
  Pass-Check "bounded repository/ref/collaborator pagination is wired through contract, Go, and Web"
} else {
  Fail-Check "repository/ref/collaborator pagination implementation or regression evidence is incomplete"
}

if (
  (Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_rpc.go") -Raw).Contains("func (r *EVMRPC) BlockNumber(") -and
  (Get-Content -LiteralPath (Join-Path $root "cli/internal/chain/evm_rpc.go") -Raw).Contains("CallContractAt") -and
  $evmBackendSource.Contains("snapshotBlockTag") -and
  (Get-Content -LiteralPath (Join-Path $root "web/src/lib/chain.ts") -Raw).Contains("evmSnapshotBlockTag") -and
  $webTests.Contains("eth_blockNumber")
) {
  Pass-Check "multi-page V2 enumeration is pinned to one EVM block tag"
} else {
  Fail-Check "V2 pagination block snapshot implementation or regression evidence is incomplete"
}

$identityAdr = Get-Content -LiteralPath (Join-Path $root "docs/evm-v2-repo-identity.md") -Raw
if (
  $evmSource.Contains("storedRef.packUris.length >= MAX_PACK_URIS") -and
  $evmTests.Contains("testNormalUpdateCannotGrowStoredPackUrisPastLimit") -and
  $identityAdr.Contains("MAX_PACK_URIS = 128")
) {
  Pass-Check "V2 stored pack URI growth is bounded and covered by regression evidence"
} else {
  Fail-Check "V2 stored pack URI cumulative limit or regression evidence is missing"
}

$v1Protocol = Get-Content -LiteralPath (Join-Path $root "docs/cosmwasm-v1-protocol.md") -Raw
if (
  $v1Tests.Contains("transfer_ownership_preserves_and_resets_v1_extension_state") -and
  $v1Tests.Contains("transfer_ownership_is_allowed_while_frozen_or_delisted") -and
  $v1Tests.Contains("fork_copies_only_v1_metadata_and_ref_snapshot") -and
  $v1Tests.Contains("fork_copies_refs_beyond_the_default_query_page") -and
  $v1Protocol.Contains("Badges and moderation")
) {
  Pass-Check "V1 ownership transfer and fork side effects are frozen by characterization tests"
} else {
  Fail-Check "V1 ownership transfer/fork characterization evidence is incomplete"
}

try {
  $abi = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/abi/RepoRegistryV2.json") -Raw | ConvertFrom-Json
  $metadataFunction = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "updateRepoInfo" })
  $metadataEvent = @($abi | Where-Object { $_.type -eq "event" -and $_.name -eq "RepoInfoUpdated" })
  $functionTypes = if ($metadataFunction.Count -eq 1) { @($metadataFunction[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $eventTypes = if ($metadataEvent.Count -eq 1) { @($metadataEvent[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  if (
    $metadataFunction.Count -eq 1 -and
    $metadataFunction[0].stateMutability -eq "nonpayable" -and
    $functionTypes -eq "string,bool,string,bool,string" -and
    $metadataEvent.Count -eq 1 -and
    $eventTypes -eq "bytes32,address,string,uint8,string,string"
  ) {
    Pass-Check "checked-in V2 ABI exposes repo metadata function and event"
  } else {
    Fail-Check "checked-in V2 ABI is missing the repo metadata function or event"
  }
} catch {
  Fail-Check "cannot parse V2 ABI for repo metadata: $($_.Exception.Message)"
}

try {
  $abi = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/abi/RepoRegistryV2.json") -Raw | ConvertFrom-Json
  $createImport = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "createImportSession" })
  $importRepo = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "importRepo" })
  $importRefs = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "importRefs" })
  $importCollaborators = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "importCollaborators" })
  $importProgress = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "importProgress" })
  $importWindowClosed = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "importWindowClosed" })
  $finalizeImport = @($abi | Where-Object { $_.type -eq "function" -and $_.name -eq "finalizeImport" })
  $importFinalized = @($abi | Where-Object { $_.type -eq "event" -and $_.name -eq "ImportFinalized" })
  $createTypes = if ($createImport.Count -eq 1) { @($createImport[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $repoTypes = if ($importRepo.Count -eq 1) { @($importRepo[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $progressInputTypes = if ($importProgress.Count -eq 1) { @($importProgress[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $progressTypes = if ($importProgress.Count -eq 1) { @($importProgress[0].outputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $windowTypes = if ($importWindowClosed.Count -eq 1) { @($importWindowClosed[0].outputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $finalizeTypes = if ($finalizeImport.Count -eq 1) { @($finalizeImport[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  if (
    $createImport.Count -eq 1 -and
    $createTypes -eq "bytes32,string,address,uint64,uint256,uint256,uint256,uint256" -and
    $importRepo.Count -eq 1 -and
    $repoTypes -eq "bytes32,uint256,bytes32,address,string,string,string,uint8,uint64,uint64,uint256,uint256,bytes32" -and
    $importRefs.Count -eq 1 -and $importRefs[0].inputs[3].type -eq "tuple[]" -and
    $importCollaborators.Count -eq 1 -and $importCollaborators[0].inputs[3].type -eq "tuple[]" -and
    $importProgress.Count -eq 1 -and $progressInputTypes -eq "bytes32" -and
    $progressTypes -eq "bool,bool,bool,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256,uint256" -and
    $importWindowClosed.Count -eq 1 -and
    $windowTypes -eq "bool" -and
    $finalizeImport.Count -eq 1 -and $finalizeTypes -eq "bytes32" -and
    $importFinalized.Count -eq 1
  ) {
    Pass-Check "checked-in V2 ABI exposes the ordered snapshot import protocol"
  } else {
    Fail-Check "checked-in V2 ABI is missing or mismatches the snapshot import protocol"
  }
} catch {
  Fail-Check "cannot parse V2 ABI for snapshot import protocol: $($_.Exception.Message)"
}

try {
  $controllerAbi = Get-Content -LiteralPath (Join-Path $root "contracts/evm-v2/abi/RepoRegistryV2ImportController.json") -Raw | ConvertFrom-Json
  $controllerCreate = @($controllerAbi | Where-Object { $_.type -eq "function" -and $_.name -eq "createImportSession" })
  $controllerAbort = @($controllerAbi | Where-Object { $_.type -eq "function" -and $_.name -eq "abortImport" })
  $controllerCodeHash = @($controllerAbi | Where-Object { $_.type -eq "function" -and $_.name -eq "registryCodeHash" })
  $controllerFreeze = @($controllerAbi | Where-Object { $_.type -eq "function" -and $_.name -eq "setFrozen" })
  $controllerPublished = @($controllerAbi | Where-Object { $_.type -eq "event" -and $_.name -eq "ImportPublished" })
  $controllerAborted = @($controllerAbi | Where-Object { $_.type -eq "event" -and $_.name -eq "ImportAborted" })
  $controllerCreateTypes = if ($controllerCreate.Count -eq 1) { @($controllerCreate[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $controllerAbortTypes = if ($controllerAbort.Count -eq 1) { @($controllerAbort[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $controllerCodeHashTypes = if ($controllerCodeHash.Count -eq 1) { @($controllerCodeHash[0].outputs | ForEach-Object { $_.type }) -join "," } else { "" }
  $controllerFreezeTypes = if ($controllerFreeze.Count -eq 1) { @($controllerFreeze[0].inputs | ForEach-Object { $_.type }) -join "," } else { "" }
  if (
    $controllerCreateTypes -eq "bytes32,string,address,uint64,uint256,uint256,uint256,uint256,bytes32" -and
    $controllerAbortTypes -eq "bytes32,address" -and
    $controllerCodeHashTypes -eq "bytes32" -and
    $controllerFreezeTypes -eq "address,string,bool" -and
    $controllerPublished.Count -eq 1 -and
    $controllerAborted.Count -eq 1
  ) {
    Pass-Check "checked-in import controller ABI exposes commitment, code identity, recovery, and moderation forwarding"
  } else {
    Fail-Check "checked-in import controller ABI is missing commitment, code identity, or recovery protocol"
  }
} catch {
  Fail-Check "cannot parse import controller ABI: $($_.Exception.Message)"
}

$node = Find-Command "node"
if ($null -eq $node) {
  if ($Required) {
    Fail-Check "Node.js is required for the structured Web metadata source gate"
  } else {
    Skip-Check "Node.js not found; structured Web metadata source gate was not run"
  }
} else {
  $webMetadataChecker = @'
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
  check(/["']eth_sendTransaction["']/.test(update), "metadata writer must submit an EVM transaction");
  const receiptIndex = update.indexOf("await waitForEvmReceipt");
  const cacheIndex = update.indexOf("clearQueryCache");
  const returnIndex = update.indexOf("return txHash");
  check(
    receiptIndex >= 0 && cacheIndex > receiptIndex && returnIndex > cacheIndex,
    "metadata writer must await a successful receipt before cache invalidation and return",
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
'@
  $previousErrorAction = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    $nodePath = $node.Source
    $checkerOutput = $webMetadataChecker | & $nodePath "-" $root 2>&1
    $checkerExitCode = $LASTEXITCODE
    foreach ($line in $checkerOutput) {
      Write-Host $line
    }
    if ($checkerExitCode -eq 0) {
      Pass-Check "Web repo metadata write path and regression evidence"
    } else {
      Fail-Check "Web repo metadata write path or regression evidence is incomplete"
    }
  } finally {
    $ErrorActionPreference = $previousErrorAction
  }
}

$identityGate = Join-Path $root "scripts/identity-readiness.mjs"
if ($null -eq $node) {
  if ($Required) {
    Fail-Check "Node.js is required for the stable repoId identity gate"
  } else {
    Skip-Check "Node.js not found; stable repoId identity gate was not run"
  }
} elseif (Invoke-Checked "node" @($identityGate, $root) $root) {
  Pass-Check "stable repoId identity, transfer, client, ABI, and documentation evidence"
} else {
  Fail-Check "stable repoId identity evidence is incomplete"
}

$solcCheck = Join-Path $root "scripts/evm-v2-solc-check.mjs"
if ($null -eq $node) {
  if ($Required) {
    Fail-Check "Node.js is required for the Solidity source compilation gate"
  } else {
    Skip-Check "Node.js not found; Solidity source compilation gate was not run"
  }
} elseif (Invoke-Checked "node" @($solcCheck) $root) {
  Pass-Check "Solidity 0.8.24 source and Foundry test compilation"
} else {
  Fail-Check "Solidity 0.8.24 source/test compilation"
}

$depsPath = Join-Path $root "cli/internal/bootstrap/deps.json"
try {
  $deps = Get-Content -LiteralPath $depsPath -Raw | ConvertFrom-Json
  $windowsKubo = $deps.kubo.artifacts."windows-amd64"
  $expectedKuboHash = "f24e4d24445c8abf7bd26bd034cb9f14dac77e30452731a617d2e8e2f2ceb150"
  $urlsAreHTTPS = @($windowsKubo.urls).Count -ge 2
  foreach ($url in @($windowsKubo.urls)) {
    if (-not ([string]$url).StartsWith("https://", [System.StringComparison]::Ordinal)) {
      $urlsAreHTTPS = $false
    }
  }
  if (
    $deps.kubo.version -eq "0.42.0" -and
    $windowsKubo.archive -eq "zip" -and
    $windowsKubo.sha256 -eq $expectedKuboHash -and
    $windowsKubo.files."kubo/ipfs.exe" -eq "ipfs.exe" -and
    $urlsAreHTTPS
  ) {
    Pass-Check "Windows native Kubo artifact is pinned and structurally valid"
  } else {
    Fail-Check "Windows native Kubo manifest entry is missing or invalid"
  }
} catch {
  Fail-Check "cannot parse Kubo dependency manifest: $($_.Exception.Message)"
}

$isWindows = ($env:OS -eq "Windows_NT")
$bash = Find-Command "bash"
if ($isWindows) {
  # A Windows bash may be WSL-backed and cannot consume a native D:\ path.
  # Shell syntax is covered by Linux CI; keep this gate native and explicit.
  Skip-Check "bash syntax checks are delegated to Linux CI on Windows"
} elseif ($null -ne $bash) {
  $shellFailed = $false
  Get-ChildItem -LiteralPath (Join-Path $root "scripts") -Filter "*.sh" -File | Sort-Object FullName | ForEach-Object {
    if (-not (Invoke-Checked "bash" @("-n", $_.FullName) $root)) {
      $shellFailed = $true
      Write-Host "FAIL: bash syntax $($_.FullName.Substring($root.Length + 1))" -ForegroundColor Red
    }
  }
  if (-not $shellFailed) {
    Pass-Check "maintained shell scripts parse"
  } else {
    $fail++
  }
} elseif ($Required) {
  Fail-Check "bash is required to parse maintained shell scripts"
} else {
  Skip-Check "bash is not installed; shell syntax checks were not run"
}

if ($SourceOnly) {
  Skip-Check "toolchain tests disabled by -SourceOnly"
  Skip-Check "Web tests disabled by -SourceOnly"
  Skip-Check "CosmWasm V1 tests disabled by -SourceOnly"
  Skip-Check "EVM V2 Foundry checks disabled by -SourceOnly"
} else {
  $go = Find-Command "go"
  if ($null -eq $go) {
    if ($Required) { Fail-Check "Go toolchain not found" } else { Skip-Check "Go toolchain not found" }
  } else {
    $goTest = Invoke-Checked "go" @("test", "./...") (Join-Path $root "cli")
    $goVet = Invoke-Checked "go" @("vet", "./...") (Join-Path $root "cli")
    if ($goTest -and $goVet) { Pass-Check "Go CLI tests and vet" } else { Fail-Check "Go CLI tests and vet" }
  }

  $npm = Find-Command "npm"
  if ($null -eq $npm) {
    if ($Required) { Fail-Check "npm toolchain not found" } else { Skip-Check "npm toolchain not found" }
  } else {
    $api = Invoke-Checked "npm" @("run", "test:api") (Join-Path $root "web")
    $build = Invoke-Checked "npm" @("run", "build") (Join-Path $root "web")
    if ($api -and $build) { Pass-Check "Web API tests and production build" } else { Fail-Check "Web API tests and production build" }
  }

  $cargo = Find-Command "cargo"
  $rustup = Find-Command "rustup"
  if ($null -eq $cargo -and $null -eq $rustup) {
    if ($Required) { Fail-Check "Rust/Cargo toolchain not found" } else { Skip-Check "Rust/Cargo toolchain not found" }
  } elseif ($null -ne $rustup) {
    $v1RustToolchain = $env:IGIT_V1_RUST_TOOLCHAIN
    if ([string]::IsNullOrWhiteSpace($v1RustToolchain)) {
      $activeRustHost = ""
      $rustc = Find-Command "rustc"
      if ($null -ne $rustc) {
        $previousErrorAction = $ErrorActionPreference
        try {
          $ErrorActionPreference = "Continue"
          $rustDetails = (& rustc -vV 2>&1 | Out-String)
          $hostMatch = [regex]::Match($rustDetails, '(?m)^host:\s*(\S+)\s*$')
          if ($hostMatch.Success) {
            $activeRustHost = $hostMatch.Groups[1].Value
          }
        } finally {
          $ErrorActionPreference = $previousErrorAction
        }
      }
      $v1RustToolchain = if ($activeRustHost) { "1.81.0-$activeRustHost" } else { "1.81.0" }
    }

    $previousErrorAction = $ErrorActionPreference
    try {
      $ErrorActionPreference = "Continue"
      $rustVersion = (& rustup run $v1RustToolchain rustc --version 2>&1 | Out-String).Trim()
      $rustVersionExitCode = $LASTEXITCODE
    } finally {
      $ErrorActionPreference = $previousErrorAction
    }
    if ($rustVersionExitCode -ne 0 -or $rustVersion -notmatch '^rustc 1\.81\.') {
      Fail-Check "CosmWasm V1 rustup toolchain $v1RustToolchain is unavailable or not rustc 1.81.x"
    } else {
      $v1 = Invoke-Checked "rustup" @(
        "run", $v1RustToolchain, "cargo", "test", "--locked", "--manifest-path",
        (Join-Path $root "contracts/repo-registry/Cargo.toml")
      ) $root
      if ($v1) {
        Pass-Check "CosmWasm V1 behavior tests (rustup $v1RustToolchain)"
      } else {
        Fail-Check "CosmWasm V1 behavior tests (rustup $v1RustToolchain)"
      }
    }
  } else {
    $rustc = Find-Command "rustc"
    if ($null -eq $rustc) {
      Fail-Check "rustc is required to verify the CosmWasm V1 toolchain"
    } else {
      $rustVersion = (& rustc --version 2>&1 | Out-String).Trim()
      if ($rustVersion -notmatch '^rustc 1\.81\.') {
        Fail-Check "CosmWasm V1 requires rustc 1.81.x (found: $rustVersion)"
      } else {
        $v1 = Invoke-Checked "cargo" @("test", "--locked", "--manifest-path", (Join-Path $root "contracts/repo-registry/Cargo.toml")) $root
        if ($v1) { Pass-Check "CosmWasm V1 behavior tests" } else { Fail-Check "CosmWasm V1 behavior tests" }
      }
    }
  }

  $forge = Find-Command "forge"
  if ($null -eq $forge) {
    if ($Required) { Fail-Check "Foundry (forge) not found" } else { Skip-Check "Foundry (forge) not found" }
  } else {
    $evmDir = Join-Path $root "contracts/evm-v2"
    $build = Invoke-Checked "forge" @("build") $evmDir
    $tests = Invoke-Checked "forge" @("test", "-vvv") $evmDir
    $gas = Invoke-Checked "forge" @("test", "--gas-report") $evmDir
    if ($build -and $tests -and $gas) { Pass-Check "EVM V2 Foundry build, tests, and gas report" } else { Fail-Check "EVM V2 Foundry checks" }
  }
}

if ($fail -gt 0) {
  Write-Host "MIGRATION SOURCE READINESS: FAIL (pass=$pass skip=$skip fail=$fail)" -ForegroundColor Red
  exit 1
}
Write-Host "MIGRATION SOURCE READINESS: PASS (pass=$pass skip=$skip fail=$fail)"
Write-Host "CUTOVER READINESS: NOT EVALUATED (run scripts/migration-cutover-readiness.ps1 with reviewed evidence and expected commit)"
exit 0
