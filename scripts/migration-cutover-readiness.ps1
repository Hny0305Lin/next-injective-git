[CmdletBinding()]
param(
  [Parameter(Mandatory = $true, Position = 0)]
  [string]$EvidenceDirectory,
  [Parameter(Mandatory = $true, Position = 1)]
  [ValidatePattern('^[0-9a-fA-F]{40}$')]
  [string]$ExpectedCommit
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
try {
  $evidenceItem = Get-Item -LiteralPath $EvidenceDirectory -Force -ErrorAction Stop
} catch {
  Write-Error "CUTOVER READINESS: FAIL (evidence directory does not exist)"
  exit 1
}
if ($evidenceItem.PSProvider.Name -ne "FileSystem" -or -not $evidenceItem.PSIsContainer) {
  Write-Error "CUTOVER READINESS: FAIL (evidence path is not a directory)"
  exit 1
}
if (($evidenceItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
  Write-Error "CUTOVER READINESS: FAIL (evidence directory must not be a symbolic link, junction, or reparse point)"
  exit 1
}
$evidence = $evidenceItem.FullName

$required = @(
  "deployment.json",
  "foundry-test.txt",
  "foundry-invariant.txt",
  "foundry-gas.txt",
  "admin-dry-run-journal.tar",
  "migration-plan.json",
  "migration-manifest.json",
  "migration-receipt-journal.tar",
  "imported-state.json",
  "windows-clean-e2e.txt",
  "linux-clean-e2e.txt",
  "web-receipt-e2e.txt",
  "security-review.pdf",
  "finality-runbook.md",
  "cutover-approval.txt"
)
$manifestPath = Join-Path $evidence "cutover-evidence.sha256"
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf) -or (Get-Item -LiteralPath $manifestPath).Length -eq 0) {
  Write-Error "CUTOVER READINESS: FAIL (missing cutover-evidence.sha256)"
  exit 1
}
if (((Get-Item -LiteralPath $manifestPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
  Write-Error "CUTOVER READINESS: FAIL (checksum manifest must not be a symbolic link or reparse point)"
  exit 1
}

function Test-NoEvidenceReparsePoint([string]$Base, [string]$Relative) {
  $current = $Base
  foreach ($component in ($Relative -split '[\\/]')) {
    if ([string]::IsNullOrEmpty($component) -or $component -eq ".") {
      continue
    }
    $current = Join-Path $current $component
    try {
      $item = Get-Item -LiteralPath $current -Force -ErrorAction Stop
    } catch {
      return $false
    }
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
      return $false
    }
  }
  return $true
}

$records = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::Ordinal)
$foldedRecords = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::OrdinalIgnoreCase)
foreach ($line in Get-Content -LiteralPath $manifestPath -Encoding UTF8) {
  if ($line -notmatch '^([0-9a-fA-F]{64})\s+\*?(.+)$') {
    Write-Error "CUTOVER READINESS: FAIL (malformed checksum record)"
    exit 1
  }
  $name = $Matches[2]
  if ($name -match '(^|[\\/])\.\.([\\/]|$)' -or [IO.Path]::IsPathRooted($name)) {
    Write-Error "CUTOVER READINESS: FAIL (checksum path escapes evidence directory: $name)"
    exit 1
  }
  if ($records.ContainsKey($name)) {
    Write-Error "CUTOVER READINESS: FAIL (duplicate checksum record: $name)"
    exit 1
  }
  if ($foldedRecords.ContainsKey($name)) {
    Write-Error "CUTOVER READINESS: FAIL (case-folded checksum path collision: $name)"
    exit 1
  }
  $records[$name] = $Matches[1].ToLowerInvariant()
  $foldedRecords[$name] = $name
}

foreach ($name in $required) {
  $path = Join-Path $evidence $name
  if (-not (Test-Path -LiteralPath $path -PathType Leaf) -or (Get-Item -LiteralPath $path).Length -eq 0) {
    Write-Error "CUTOVER READINESS: FAIL (missing evidence: $name)"
    exit 1
  }
  if (-not $records.ContainsKey($name)) {
    Write-Error "CUTOVER READINESS: FAIL (evidence hash is not bound: $name)"
    exit 1
  }
}

foreach ($name in $records.Keys) {
  $path = Join-Path $evidence $name
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
    Write-Error "CUTOVER READINESS: FAIL (checksum record names a missing file: $name)"
    exit 1
  }
  if (-not (Test-NoEvidenceReparsePoint $evidence $name)) {
    Write-Error "CUTOVER READINESS: FAIL (checksum path contains a symbolic link or reparse point: $name)"
    exit 1
  }
  $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $records[$name]) {
    Write-Error "CUTOVER READINESS: FAIL (evidence checksum mismatch: $name)"
    exit 1
  }
}

$approvalFields = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::Ordinal)
$approvalValid = $true
foreach ($line in Get-Content -LiteralPath (Join-Path $evidence "cutover-approval.txt") -Encoding UTF8) {
  $separator = $line.IndexOf('=')
  if ($separator -lt 1) {
    $approvalValid = $false
    break
  }
  $key = $line.Substring(0, $separator)
  if ($key -notin @("decision", "reviewer", "reviewed_commit", "reviewed_at")) {
    continue
  }
  if ($approvalFields.ContainsKey($key)) {
    $approvalValid = $false
    break
  }
  $approvalFields[$key] = $line.Substring($separator + 1)
}

$decision = if ($approvalFields.ContainsKey("decision")) { $approvalFields["decision"] } else { "" }
$reviewer = if ($approvalFields.ContainsKey("reviewer")) { $approvalFields["reviewer"] } else { "" }
$reviewedCommit = if ($approvalFields.ContainsKey("reviewed_commit")) { $approvalFields["reviewed_commit"] } else { "" }
$reviewedAt = if ($approvalFields.ContainsKey("reviewed_at")) { $approvalFields["reviewed_at"] } else { "" }
if (-not $approvalValid -or
    $decision -ne "approved" -or
    $reviewer -notmatch '^\S.*$' -or
    $reviewedCommit.ToLowerInvariant() -ne $ExpectedCommit.ToLowerInvariant() -or
    $reviewedAt -notmatch '^[0-9]{4}-[0-9]{2}-[0-9]{2}T.*Z$') {
  Write-Error "CUTOVER READINESS: FAIL (cutover approval is incomplete)"
  exit 1
}

Write-Host "CUTOVER READINESS: PASS (reviewed, hash-bound evidence verified)"
Write-Host "evidence directory: $evidence"
Write-Host "source gate: $(Join-Path $root 'scripts/migration-readiness.ps1') -Required"
exit 0
