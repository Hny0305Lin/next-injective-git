$ErrorActionPreference = "Stop"

$evidenceDirectory = Join-Path $PSScriptRoot "..\evidence\testnet-deployment-2026-08-25"
$metadataDirectory = Join-Path $evidenceDirectory "metadata"
$rpcUrl = "https://k8s.testnet.json-rpc.injective.network"
$explorerUrl = "https://testnet.blockscout.injective.network"

$contracts = [ordered]@{
    "SuiteDirectory" = "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334"
    "BootstrapCoordinator" = "0x329921023FCf6E337E924686b92f7521549B5970"
    "RepositoryCore" = "0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4"
    "RecoveryModule" = "0xa7249cE20B54C4Be440838F0eF403d2a6a07E532"
    "ModerationModule" = "0xb957B65634931dD0613abAC296D5a3837BF979Bd"
    "EconomicModule" = "0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd"
    "UsernameModule" = "0xc7C164E46788b37e98D27aCe908C489999c09853"
    "BadgeModule" = "0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D"
    "ReleaseModule" = "0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f"
}

function Get-ContractCode {
    param([string]$Address)

    $body = @{
        jsonrpc = "2.0"
        method = "eth_getCode"
        params = @($Address, "latest")
        id = 1
    } | ConvertTo-Json -Compress
    $response = Invoke-RestMethod -Uri $rpcUrl -Method Post -Body $body -ContentType "application/json"
    if ($null -ne $response.error) {
        throw "RPC error for $Address`: $($response.error.message)"
    }
    if ($response.result -isnot [string]) {
        throw "RPC returned no code for $Address"
    }
    return $response.result
}

New-Item -ItemType Directory -Force -Path $metadataDirectory | Out-Null
$missing = [System.Collections.Generic.List[string]]::new()
foreach ($contractName in $contracts.Keys) {
    $address = $contracts[$contractName]
    Write-Host "Checking $contractName at $address..."
    try {
        $code = Get-ContractCode -Address $address
    } catch {
        Write-Error $_
        $missing.Add($contractName)
        continue
    }
    if ($code -eq "0x" -or $code.Length -le 10) {
        Write-Error "No deployed bytecode found for $contractName at $address"
        $missing.Add($contractName)
        continue
    }
    $metadata = [ordered]@{
        metadata_type = "on-chain-code"
        contract_name = $contractName
        address = $address
        deployer = "0x85eAc7bC081488AA77D1D82f9cB8e053De1e4Fa8"
        network = "injective-testnet"
        chain_id = 1439
        rpc_url = $rpcUrl
        explorer_url = "$explorerUrl/address/$address"
        code_present = $true
        code_size_bytes = ($code.Length / 2) - 1
        collected_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    } | ConvertTo-Json -Depth 10
    [IO.File]::WriteAllText((Join-Path $metadataDirectory "$contractName.json"), "$metadata`n", [Text.UTF8Encoding]::new($false))
    Write-Host "  On-chain bytecode present ($(($code.Length / 2) - 1) bytes)"
}

if ($missing.Count -gt 0) {
    throw "Metadata collection failed for: $($missing -join ', ')"
}

Write-Host "Metadata saved to: $metadataDirectory"
