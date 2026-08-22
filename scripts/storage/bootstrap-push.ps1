[CmdletBinding()]
param(
    [string]$Distro = "Ubuntu-24.04",
    [string]$LinuxUser = "",
    [switch]$Yes,
    [switch]$NoKubo,
    [switch]$Force,
    [string]$CreateKey = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$bootstrap = Join-Path $PSScriptRoot "bootstrap-push.sh"
if (-not (Test-Path -LiteralPath $bootstrap)) {
    throw "Missing bootstrap script: $bootstrap"
}

$installed = @(& wsl.exe --list --quiet 2>$null) | ForEach-Object { ($_ -replace "`0", "").Trim() }
$registeredNow = $false
if ($installed -notcontains $Distro) {
    Write-Host "Installing WSL2 distribution $Distro..."
    $launcherName = switch ($Distro) {
        "Ubuntu-24.04" { "ubuntu2404.exe" }
        "Ubuntu-22.04" { "ubuntu2204.exe" }
        default { "" }
    }
    $launcher = if ($launcherName) { Get-Command $launcherName -ErrorAction SilentlyContinue } else { $null }
    if ($launcher) {
        Write-Host "Registering the installed $launcherName rootfs..."
        & $launcher.Path install --root
        if ($LASTEXITCODE -ne 0) {
            throw "$launcherName failed to register $Distro."
        }
    } else {
        & wsl.exe --install --distribution $Distro --no-launch --web-download
        if ($LASTEXITCODE -ne 0) {
            throw "WSL web download failed. Check network access or install $Distro from Microsoft Store, then rerun this script."
        }
    }
    $registeredNow = $true
}

if ($registeredNow) {
    if (-not $LinuxUser) {
        $LinuxUser = ($env:USERNAME.ToLowerInvariant() -replace '[^a-z0-9_-]', '')
        if ($LinuxUser -notmatch '^[a-z_]') {
            $LinuxUser = "igit-$LinuxUser"
        }
    }
    if ($LinuxUser -notmatch '^[a-z_][a-z0-9_-]{0,31}$') {
        throw "Invalid Linux user '$LinuxUser'. Use 1-32 lowercase letters, digits, underscores, or hyphens, starting with a letter or underscore."
    }
    Write-Host "Creating non-root WSL user $LinuxUser..."
    & wsl.exe -d $Distro --user root -- useradd --create-home --shell /bin/bash $LinuxUser
    if ($LASTEXITCODE -ne 0) { throw "Could not create WSL user $LinuxUser." }
    & wsl.exe -d $Distro --user root -- usermod --append --groups sudo $LinuxUser
    if ($LASTEXITCODE -ne 0) { throw "Could not add $LinuxUser to the sudo group." }
    $sudoers = "$LinuxUser ALL=(ALL:ALL) NOPASSWD:ALL`n"
    $sudoersBase64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($sudoers))
    $sudoersCommand = "printf %s $sudoersBase64 | base64 -d > /etc/sudoers.d/90-$LinuxUser; chmod 0440 /etc/sudoers.d/90-$LinuxUser"
    & wsl.exe -d $Distro --user root -- sh -c $sudoersCommand
    if ($LASTEXITCODE -ne 0) { throw "Could not configure non-interactive sudo for $LinuxUser." }

    $defaultLauncher = if ($launcherName) { Get-Command $launcherName -ErrorAction SilentlyContinue } else { $null }
    if ($defaultLauncher) {
        & $defaultLauncher.Path config --default-user $LinuxUser
    } else {
        $userConfig = "`n[user]`ndefault=$LinuxUser`n"
        $userConfigBase64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($userConfig))
        $userConfigCommand = "printf %s $userConfigBase64 | base64 -d >> /etc/wsl.conf"
        & wsl.exe -d $Distro --user root -- sh -c $userConfigCommand
    }
    if ($LASTEXITCODE -ne 0) { throw "Could not set $LinuxUser as the default WSL user." }
    & wsl.exe --terminate $Distro
}

& wsl.exe -d $Distro -- true
if ($LASTEXITCODE -ne 0) {
    throw "WSL distribution '$Distro' could not start. Complete any pending Windows restart, then rerun this script."
}

$resolvedBootstrap = (Resolve-Path -LiteralPath $bootstrap).Path
if ($resolvedBootstrap -match '^([A-Za-z]):\\(.*)$') {
    $drive = $Matches[1].ToLowerInvariant()
    $tail = $Matches[2] -replace '\\', '/'
    $wslBootstrap = "/mnt/$drive/$tail"
} else {
    throw "The bootstrap script must be on a Windows drive mounted by WSL: $resolvedBootstrap"
}

$setupArgs = @()
if ($Yes) { $setupArgs += "--yes" }
if ($NoKubo) { $setupArgs += "--no-kubo" }
if ($Force) { $setupArgs += "--force" }
if ($CreateKey) { $setupArgs += @("--create-key", $CreateKey) }

Write-Host "Bootstrapping igit push tooling in $Distro..."
& wsl.exe -d $Distro -- bash $wslBootstrap @setupArgs
if ($LASTEXITCODE -ne 0) {
    throw "igit WSL bootstrap failed with exit code $LASTEXITCODE"
}

Write-Host ""
Write-Host "WSL setup completed. Open it with: wsl.exe -d $Distro"
