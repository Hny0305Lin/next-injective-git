# Compatibility shim — moved to scripts/migration/migration-cutover-readiness-test.ps1 (remove after one release cycle)
$oldPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$newPath = Join-Path $oldPath "migration/migration-cutover-readiness-test.ps1"
& $newPath @args
exit $LASTEXITCODE
