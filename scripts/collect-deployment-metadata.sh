#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EVIDENCE_DIR="$SCRIPT_DIR/../evidence/testnet-deployment-2026-08-25"
METADATA_DIR="$EVIDENCE_DIR/metadata"
RPC_URL="https://k8s.testnet.json-rpc.injective.network"
EXPLORER_URL="https://testnet.blockscout.injective.network"
DEPLOYER="0x85eAc7bC081488AA77D1D82f9cB8e053De1e4Fa8"

declare -A CONTRACTS=(
    ["SuiteDirectory"]="0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334"
    ["BootstrapCoordinator"]="0x329921023FCf6E337E924686b92f7521549B5970"
    ["RepositoryCore"]="0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4"
    ["RecoveryModule"]="0xa7249cE20B54C4Be440838F0eF403d2a6a07E532"
    ["ModerationModule"]="0xb957B65634931dD0613abAC296D5a3837BF979Bd"
    ["EconomicModule"]="0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd"
    ["UsernameModule"]="0xc7C164E46788b37e98D27aCe908C489999c09853"
    ["BadgeModule"]="0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D"
    ["ReleaseModule"]="0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f"
)

mkdir -p "$METADATA_DIR"
missing=()
for contract_name in "${!CONTRACTS[@]}"; do
    address="${CONTRACTS[$contract_name]}"
    response="$(curl --fail-with-body --silent --show-error -X POST "$RPC_URL" \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getCode\",\"params\":[\"$address\",\"latest\"],\"id\":1}")"
    if jq -e '.error' >/dev/null <<<"$response"; then
        echo "RPC error for $contract_name: $(jq -r '.error.message // "unknown error"' <<<"$response")" >&2
        missing+=("$contract_name")
        continue
    fi
    code="$(jq -er '.result' <<<"$response")"
    if [[ "$code" == "0x" || ${#code} -le 10 ]]; then
        echo "No deployed bytecode found for $contract_name at $address" >&2
        missing+=("$contract_name")
        continue
    fi
    code_size=$(( ${#code} / 2 - 1 ))
    jq -n \
        --arg metadata_type "on-chain-code" \
        --arg contract_name "$contract_name" \
        --arg address "$address" \
        --arg deployer "$DEPLOYER" \
        --arg network "injective-testnet" \
        --arg rpc_url "$RPC_URL" \
        --arg explorer_url "$EXPLORER_URL/address/$address" \
        --arg collected_at "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
        --argjson chain_id 1439 \
        --argjson code_present true \
        --argjson code_size_bytes "$code_size" \
        '{metadata_type:$metadata_type,contract_name:$contract_name,address:$address,deployer:$deployer,network:$network,chain_id:$chain_id,rpc_url:$rpc_url,explorer_url:$explorer_url,code_present:$code_present,code_size_bytes:$code_size_bytes,collected_at:$collected_at}' \
        > "$METADATA_DIR/$contract_name.json"
done

if ((${#missing[@]} > 0)); then
    printf 'Metadata collection failed for: %s\n' "${missing[*]}" >&2
    exit 1
fi

echo "Metadata saved to: $METADATA_DIR"
