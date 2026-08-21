param(
  [string]$Source = $env:GITHUB_WORKSPACE
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Source)) {
  $Source = (Get-Location).Path
}

$sourcePath = (Resolve-Path -LiteralPath $Source).Path
$sourceHead = (& git -C $sourcePath rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $sourceHead -notmatch '^[0-9a-f]{40}$') {
  throw "could not resolve source repository HEAD"
}

$cleanPath = Join-Path ([System.IO.Path]::GetTempPath()) ("igit-clean-" + [guid]::NewGuid().ToString("N"))
try {
  # A local clone keeps the check deterministic and avoids depending on a
  # second network checkout, while -c core.autocrlf=true exercises checkout
  # conversion from a genuinely empty worktree.
  & git -c core.autocrlf=true clone --no-local $sourcePath $cleanPath
  if ($LASTEXITCODE -ne 0) { throw "clean clone failed" }

  $cleanHead = (& git -C $cleanPath rev-parse HEAD).Trim()
  if ($LASTEXITCODE -ne 0 -or $cleanHead -ne $sourceHead) {
    throw "clean clone HEAD $cleanHead does not match source HEAD $sourceHead"
  }
  $status = (& git -C $cleanPath status --porcelain)
  if ($LASTEXITCODE -ne 0) { throw "could not inspect clean clone status" }
  if (-not [string]::IsNullOrWhiteSpace(($status -join "`n"))) {
    throw "clean clone is not clean: $($status -join '; ')"
  }

  $suiteFiles = @(
    & git -C $cleanPath ls-files contracts/evm-v2 cli/internal/chain/abi |
      Where-Object {
        $_ -match '\.sol$' -or
        $_ -match '^contracts/evm-v2/(abi|artifacts)/[^/]+\.json$' -or
        $_ -match '^cli/internal/chain/abi/[^/]+\.json$'
      }
  )
  if ($LASTEXITCODE -ne 0) { throw "could not enumerate Suite files" }
  if ($suiteFiles.Count -eq 0) { throw "no Suite LF-bound files were found" }

  foreach ($relativePath in $suiteFiles) {
    $attribute = (& git -C $cleanPath check-attr eol -- $relativePath).Trim()
    if ($LASTEXITCODE -ne 0) { throw "could not inspect eol attribute for $relativePath" }
    if ($attribute -notmatch ': eol: lf$') {
      throw "$relativePath is not pinned to LF: $attribute"
    }
    $absolutePath = Join-Path $cleanPath $relativePath
    $content = [System.IO.File]::ReadAllText($absolutePath)
    if ($content.Contains("`r`n")) {
      throw "$relativePath was checked out with CRLF"
    }
  }

  & npm ci --prefix (Join-Path $cleanPath 'contracts/evm-v2')
  if ($LASTEXITCODE -ne 0) { throw "npm ci failed in clean checkout" }
  & npm run check --prefix (Join-Path $cleanPath 'contracts/evm-v2')
  if ($LASTEXITCODE -ne 0) { throw "Solidity/ABI/artifact check failed in clean checkout" }
  & go -C (Join-Path $cleanPath 'cli') run ./cmd/igit-deploy-suite --artifacts ../contracts/evm-v2/artifacts --check
  if ($LASTEXITCODE -ne 0) { throw "deployment artifact check failed in clean checkout" }

  Write-Host "WINDOWS CLEAN SUITE CHECK: PASS"
  Write-Host "source_head=$sourceHead"
  Write-Host "clean_checkout=$cleanPath"
}
finally {
  if (Test-Path -LiteralPath $cleanPath) {
    Remove-Item -LiteralPath $cleanPath -Recurse -Force -ErrorAction SilentlyContinue
  }
  if (Test-Path -LiteralPath $cleanPath) {
    throw "clean checkout directory was not removed: $cleanPath"
  }
}
