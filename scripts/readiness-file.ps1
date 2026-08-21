# Compatibility shim — moved to scripts/migration/readiness-file.ps1 (remove after one release cycle)
$oldPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$newPath = Join-Path $oldPath "migration/readiness-file.ps1"
& $newPath @args
exit $LASTEXITCODE
