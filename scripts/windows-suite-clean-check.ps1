# Compatibility shim — moved to scripts/ci/windows-suite-clean-check.ps1 (remove after one release cycle)
# Forwarding to new location.
$oldPath = Split-Path -Parent $MyInvocation.MyCommand.Path
$newPath = Join-Path $oldPath "ci/windows-suite-clean-check.ps1"
& $newPath @args
exit $LASTEXITCODE
