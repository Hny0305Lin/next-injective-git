# Compatibility shim — moved to scripts/storage/bootstrap-push.ps1 (remove after one release cycle)
$oldPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$newPath = Join-Path $oldPath "storage/bootstrap-push.ps1"
& $newPath @args
exit $LASTEXITCODE
