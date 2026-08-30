$ErrorActionPreference = "Stop"
$gate = Join-Path $PSScriptRoot "migration-cutover-readiness.ps1"
$temp = Join-Path ([IO.Path]::GetTempPath()) ("igit-cutover-gate-" + [Guid]::NewGuid().ToString("N"))
$utf8 = [Text.UTF8Encoding]::new($false)
$unicodeReviewer = -join (0x53D1, 0x5E03, 0x5BA1, 0x6838, 0x5458 | ForEach-Object { [char]$_ })
$expectedCommit = if ($env:EXPECTED_CUTOVER_COMMIT -match '^[0-9a-fA-F]{40}$') {
  $env:EXPECTED_CUTOVER_COMMIT.ToLowerInvariant()
} else {
  "0123456789abcdef0123456789abcdef01234567"
}
$mismatchedCommit = if ($expectedCommit -eq "fedcba9876543210fedcba9876543210fedcba98") {
  "0123456789abcdef0123456789abcdef01234567"
} else {
  "fedcba9876543210fedcba9876543210fedcba98"
}
New-Item -ItemType Directory -Path $temp | Out-Null
try {
  $powerShellExe = (Get-Process -Id $PID).Path
  $savedErrorActionPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    & $powerShellExe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $gate $temp *> $null
    $missingCommitExitCode = $LASTEXITCODE
  } finally {
    $ErrorActionPreference = $savedErrorActionPreference
  }
  if ($missingCommitExitCode -eq 0) { throw "missing expected commit unexpectedly passed" }

  $malformedCommitRejected = $false
  try {
    & $gate $temp "not-a-commit" 2>$null
    $malformedCommitRejected = ($LASTEXITCODE -ne 0)
  } catch {
    $malformedCommitRejected = $true
  }
  if (-not $malformedCommitRejected) { throw "malformed expected commit unexpectedly passed" }

  $failedClosed = $false
  try {
    & $gate $temp $expectedCommit 2>$null
    $failedClosed = ($LASTEXITCODE -ne 0)
  } catch {
    $failedClosed = $true
  }
  if (-not $failedClosed) { throw "empty evidence unexpectedly passed" }

  $required = @(
    "cutover-scope.json", "deployment.json", "suite-verification.json", "blockscout-verification.json", "foundry-test.txt", "foundry-invariant.txt", "foundry-gas.txt",
    "empty-username-escrow-attestation.json", "empty-suite-activation.log", "windows-clean-e2e.txt",
    "linux-clean-e2e.txt", "web-receipt-e2e.txt", "security-review.pdf",
    "finality-runbook.md", "cutover-approval.txt"
  )
  foreach ($name in $required) {
    $content = if ($name -eq "cutover-approval.txt") {
      "decision=approved`nreviewer=$unicodeReviewer`nreviewed_commit=$expectedCommit`nreviewed_at=2026-08-12T00:00:00Z`n"
    } elseif ($name -notin @("cutover-scope.json", "deployment.json", "suite-verification.json", "blockscout-verification.json", "empty-username-escrow-attestation.json")) {
      "fixture for $name`n"
    } else {
      $null
    }
    if ($null -ne $content) {
      [IO.File]::WriteAllText((Join-Path $temp $name), $content, $utf8)
    }
  }
  function Address([int]$value) { "0x" + $value.ToString("x40") }
  function CodeHash([int]$value) { "0x" + $value.ToString("x64") }
  function Digest([int]$value) { $value.ToString("x64") }
  $contractNames = @(
    "SuiteDirectory", "BootstrapCoordinator", "RepositoryCore", "RecoveryModule", "ModerationModule",
    "EconomicModule", "UsernameModule", "BadgeModule", "ReleaseModule"
  )
  $contracts = for ($index = 0; $index -lt $contractNames.Count; $index++) {
    $order = if ($index -lt 2) { $index + 1 } else { 2 * $index }
    $transactionHash = CodeHash ($index + 100)
    $address = Address ($index + 1)
    [ordered]@{
      transaction_order = $order
      contract_name = $contractNames[$index]
      address = $address
      transaction_hash = $transactionHash
      receipt = [ordered]@{ transactionHash = $transactionHash; blockNumber = "0x123"; blockHash = CodeHash 900; contractAddress = $address; status = "0x1" }
      runtime = [ordered]@{ code_hash_keccak256 = CodeHash ($index + 200); template_match_verified = $true }
    }
  }
  $purposes = @(
    "bind_bootstrap_coordinator", "register_repositorycore", "register_recoverymodule", "register_moderationmodule",
    "register_economicmodule", "register_usernamemodule", "register_badgemodule", "register_releasemodule"
  )
  $configurations = for ($index = 0; $index -lt $purposes.Count; $index++) {
    $transactionHash = CodeHash ($index + 300)
    [ordered]@{
      transaction_order = 3 + (2 * $index); purpose = $purposes[$index]
      target = if ($index -eq 0) { Address 1 } else { Address 2 }
      calldata = "0x01"; calldata_sha256 = Digest ($index + 400); transaction_hash = $transactionHash
      receipt = [ordered]@{ transactionHash = $transactionHash; blockNumber = "0x123"; blockHash = CodeHash 900; status = "0x1" }
    }
  }
  $deployment = [ordered]@{
    schema = "igit.evm-suite.deployment.v1"; status = "bootstrapping"; evidence_mode = "live-broadcast"
    compiler = [ordered]@{ version = "0.8.24" }
    source = [ordered]@{ commit = $expectedCommit }
    chain = [ordered]@{ chain_id = 1439 }
    snapshot_root = CodeHash 700
    contracts = @($contracts)
    configuration_transactions = @($configurations)
    directory_binding_verification = [ordered]@{ active = $false; registered_module_count = 7 }
  }
  $modules = for ($index = 2; $index -lt $contractNames.Count; $index++) {
    $root = CodeHash ($index + 500)
    [ordered]@{
      contract_name = $contractNames[$index]; module_id = CodeHash ($index + 600)
      bootstrap_started = $true; expected_count = 0; expected_batches = 0; imported_count = 0; next_sequence = 0
      expected_root = $root; rolling_root = $root; address = Address ($index + 1)
      observed_code_hash = CodeHash ($index + 200); directory_code_hash = CodeHash ($index + 200)
      directory_verified = $true; module_directory = Address 1; bootstrap_finalized = $true
    }
  }
  $activation = [ordered]@{
    schema = "igit.evm-suite.activation-verification.v1"; source_commit = $expectedCommit; chain_id = 1439
    suite_version = 3; state = 1; active = $true; activation_mode = "fresh-empty-suite"
    v1_runtime_policy = "archive-preview-only"; v1_migration_performed = $false
    coordinator_activated = $true; registered_module_count = 7
    block_number = "0x123"; block_hash = CodeHash 900; snapshot_root = CodeHash 700; directory = Address 1
    modules = @($modules)
  }
  $expectedImported = [ordered]@{}
  foreach ($name in $contractNames[2..8]) { $expectedImported[$name] = 0 }
  $scope = [ordered]@{
    schema = "igit.evm-suite.cutover-scope.v1"; source_commit = $expectedCommit; mode = "fresh-empty-suite"
    v1_runtime_policy = "archive-preview-only"; v1_migration_performed = $false; migration_evidence_required = $false
    activation_journal = "empty-suite-activation.log"; chain_id = 1439; directory = Address 1; snapshot_root = CodeHash 700
    expected_imported_records = $expectedImported
  }
  $attestation = [ordered]@{
    schema = "igit.evm-suite.empty-username-escrow-attestation.v1"; source_commit = $expectedCommit
    activation_mode = "fresh-empty-suite"; v1_migration_performed = $false; imported_username_records = 0; username_escrow_liability = "none"
    attestation_transaction_hash = CodeHash 999
  }
  [IO.File]::WriteAllText((Join-Path $temp "cutover-scope.json"), (($scope | ConvertTo-Json -Depth 8 -Compress) + "`n"), $utf8)
  [IO.File]::WriteAllText((Join-Path $temp "deployment.json"), (($deployment | ConvertTo-Json -Depth 8 -Compress) + "`n"), $utf8)
  [IO.File]::WriteAllText((Join-Path $temp "suite-verification.json"), (($activation | ConvertTo-Json -Depth 8 -Compress) + "`n"), $utf8)
  [IO.File]::WriteAllText((Join-Path $temp "empty-username-escrow-attestation.json"), (($attestation | ConvertTo-Json -Depth 8 -Compress) + "`n"), $utf8)
  $blockscout = [ordered]@{
    schema = "igit.evm-suite.blockscout-verification.v1"; source_commit = $expectedCommit; chain_id = 1439
    explorer = "https://fixture.invalid"
    contracts = @($contracts | ForEach-Object { [ordered]@{ contract_name = $_.contract_name; address = $_.address; status = "verified"; creation_transaction_hash = $_.transaction_hash } })
  }
  [IO.File]::WriteAllText((Join-Path $temp "blockscout-verification.json"), (($blockscout | ConvertTo-Json -Depth 8 -Compress) + "`n"), $utf8)
  $records = foreach ($name in $required) {
    $hash = (Get-FileHash -LiteralPath (Join-Path $temp $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $name"
  }
  $manifestPath = Join-Path $temp "cutover-evidence.sha256"
  [IO.File]::WriteAllText($manifestPath, ($records -join "`n"), $utf8)

  & $gate $temp $expectedCommit
  if ($LASTEXITCODE -ne 0) { throw "complete fixture did not pass" }

  $commitMismatchRejected = $false
  try {
    & $gate $temp $mismatchedCommit 2>$null
    $commitMismatchRejected = ($LASTEXITCODE -ne 0)
  } catch {
    $commitMismatchRejected = $true
  }
  if (-not $commitMismatchRejected) { throw "mismatched expected commit unexpectedly passed" }

  $approvalPath = Join-Path $temp "cutover-approval.txt"
  [IO.File]::WriteAllText(
    $approvalPath,
    "decision=approved`r`nreviewer=$unicodeReviewer`r`nreviewed_commit=$expectedCommit`r`nreviewed_at=2026-08-12T00:00:00Z`r`n",
    $utf8
  )
  $records = foreach ($name in $required) {
    $hash = (Get-FileHash -LiteralPath (Join-Path $temp $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $name"
  }
  [IO.File]::WriteAllText($manifestPath, (($records -join "`r`n") + "`r`n"), $utf8)
  & $gate $temp $expectedCommit
  if ($LASTEXITCODE -ne 0) { throw "UTF-8 CRLF fixture did not pass" }

  $validManifest = Get-Content -LiteralPath $manifestPath -Raw
  [IO.File]::AppendAllText(
    $manifestPath,
    "$('0' * 64)  missing-extra.txt`r`n",
    $utf8
  )
  $extraRecordRejected = $false
  try {
    & $gate $temp $expectedCommit 2>$null
    $extraRecordRejected = ($LASTEXITCODE -ne 0)
  } catch {
    $extraRecordRejected = $true
  }
  if (-not $extraRecordRejected) { throw "missing extra checksum record unexpectedly passed" }
  [IO.File]::WriteAllText($manifestPath, $validManifest, $utf8)

  [IO.File]::AppendAllText($manifestPath, "$('0' * 64)  DEPLOYMENT.JSON`r`n", $utf8)
  $caseCollisionRejected = $false
  try {
    & $gate $temp $expectedCommit 2>$null
    $caseCollisionRejected = ($LASTEXITCODE -ne 0)
  } catch {
    $caseCollisionRejected = $true
  }
  if (-not $caseCollisionRejected) { throw "case-folded checksum collision unexpectedly passed" }
  [IO.File]::WriteAllText($manifestPath, $validManifest, $utf8)

  $outside = Join-Path ([IO.Path]::GetTempPath()) ("igit-cutover-outside-" + [Guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $outside | Out-Null
  $junction = Join-Path $temp "linked"
  try {
    $outsidePath = Join-Path $outside "outside.txt"
    [IO.File]::WriteAllText($outsidePath, "outside evidence`n", $utf8)
    New-Item -ItemType Junction -Path $junction -Target $outside | Out-Null
    $outsideHash = (Get-FileHash -LiteralPath $outsidePath -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::AppendAllText($manifestPath, "$outsideHash  linked/outside.txt`r`n", $utf8)
    $junctionRejected = $false
    try {
      & $gate $temp $expectedCommit 2>$null
      $junctionRejected = ($LASTEXITCODE -ne 0)
    } catch {
      $junctionRejected = $true
    }
    if (-not $junctionRejected) { throw "junction/reparse-point evidence path unexpectedly passed" }
  } finally {
    if (Test-Path -LiteralPath $junction) {
      # Windows PowerShell 5.1 can throw a NullReferenceException when
      # Remove-Item targets a junction. Non-recursive Directory.Delete removes
      # the reparse-point entry without traversing or deleting its target.
      [IO.Directory]::Delete($junction, $false)
    }
    Remove-Item -LiteralPath $outside -Recurse -Force
  }
  [IO.File]::WriteAllText($manifestPath, $validManifest, $utf8)

  $rootJunction = Join-Path ([IO.Path]::GetTempPath()) ("igit-cutover-root-link-" + [Guid]::NewGuid().ToString("N"))
  try {
    New-Item -ItemType Junction -Path $rootJunction -Target $temp | Out-Null
    $rootJunctionRejected = $false
    try {
      & $gate $rootJunction $expectedCommit 2>$null
      $rootJunctionRejected = ($LASTEXITCODE -ne 0)
    } catch {
      $rootJunctionRejected = $true
    }
    if (-not $rootJunctionRejected) { throw "root junction/reparse-point evidence directory unexpectedly passed" }
  } finally {
    if (Test-Path -LiteralPath $rootJunction) {
      [IO.Directory]::Delete($rootJunction, $false)
    }
  }

  Add-Content -LiteralPath (Join-Path $temp "deployment.json") -Value "tampered" -Encoding UTF8
  $tamperRejected = $false
  try {
    & $gate $temp $expectedCommit 2>$null
    $tamperRejected = ($LASTEXITCODE -ne 0)
  } catch {
    $tamperRejected = $true
  }
  if (-not $tamperRejected) { throw "tampered evidence unexpectedly passed" }
  Write-Host "cutover readiness gate test: pass"
} finally {
  Remove-Item -LiteralPath $temp -Recurse -Force
}
