$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "readiness-file.ps1")

$temp = Join-Path ([IO.Path]::GetTempPath()) ("igit-readiness-file-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temp | Out-Null
try {
  $missing = Join-Path $temp "missing.txt"
  if (Test-IgitNonEmptyFile -LiteralPath $missing) {
    throw "missing file unexpectedly passed"
  }

  $directory = Join-Path $temp "directory"
  New-Item -ItemType Directory -Path $directory | Out-Null
  if (Test-IgitNonEmptyFile -LiteralPath $directory) {
    throw "directory unexpectedly passed"
  }

  $empty = Join-Path $temp "empty.txt"
  [IO.File]::WriteAllBytes($empty, [byte[]]@())
  if (Test-IgitNonEmptyFile -LiteralPath $empty) {
    throw "empty file unexpectedly passed"
  }

  $nonEmpty = Join-Path $temp "non-empty.txt"
  [IO.File]::WriteAllText($nonEmpty, "evidence", [Text.UTF8Encoding]::new($false))
  if (-not (Test-IgitNonEmptyFile -LiteralPath $nonEmpty)) {
    throw "non-empty file did not pass"
  }

  Write-Host "PowerShell non-empty required-file test: pass"
} finally {
  Remove-Item -LiteralPath $temp -Recurse -Force
}
