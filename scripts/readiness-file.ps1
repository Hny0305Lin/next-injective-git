function Test-IgitNonEmptyFile {
  [CmdletBinding()]
  param(
    [Parameter(Mandatory = $true)]
    [string]$LiteralPath
  )

  if (-not (Test-Path -LiteralPath $LiteralPath -PathType Leaf)) {
    return $false
  }
  try {
    return (Get-Item -LiteralPath $LiteralPath -Force -ErrorAction Stop).Length -gt 0
  } catch {
    return $false
  }
}
