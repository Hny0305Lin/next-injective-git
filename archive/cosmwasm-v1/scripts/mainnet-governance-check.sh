#!/usr/bin/env bash
# Read-only preflight for the mainnet repo-registry governance configuration.
# This script never broadcasts a transaction and never prints private material.
set -euo pipefail
export LC_ALL=C

LCD="${LCD:-https://lcd.injective.network}"
CHAIN_ID="${CHAIN_ID:-injective-1}"
CONTRACT="${CONTRACT:?set CONTRACT to the deployed mainnet repo-registry address}"
CURL_TIMEOUT="${CURL_TIMEOUT:-20}"
MAX_PLATFORM_FEE_BPS="${MAX_PLATFORM_FEE_BPS:-500}"
USERNAME_FEE_DENOM="${USERNAME_FEE_DENOM:-inj}"
USERNAME_FEE_MIN_AMOUNT="${USERNAME_FEE_MIN_AMOUNT:-1}"
EXPECTED_UPGRADE_TIMELOCK_SECONDS="${EXPECTED_UPGRADE_TIMELOCK_SECONDS:-1209600}"
REQUIRED_RESERVED_USERNAMES="${REQUIRED_RESERVED_USERNAMES:-admin api git help injective owner root support www}"
EXPECTED_WASM_SHA256="${EXPECTED_WASM_SHA256:-}"

# EXPECTED_ADMIN is a convenient shorthand when the Wasm admin and the
# in-contract admin are intentionally the same production multisig.
EXPECTED_WASM_ADMIN="${EXPECTED_WASM_ADMIN:-${EXPECTED_ADMIN:-}}"
EXPECTED_CONTRACT_ADMIN="${EXPECTED_CONTRACT_ADMIN:-${EXPECTED_ADMIN:-}}"
EXPECTED_COMMITTEE="${EXPECTED_COMMITTEE:-${MODERATION_COMMITTEE:-}}"

die() {
  echo "mainnet governance check: $*" >&2
  exit 1
}

[[ "$CHAIN_ID" == injective-1 ]] || die "CHAIN_ID must be injective-1"
[[ "$CONTRACT" =~ ^inj1[0-9a-z]{38}$ ]] || die "CONTRACT must be a 42-character lowercase inj1... address"
[[ "$LCD" == https://* ]] || die "LCD must use HTTPS"
case "${LCD,,}" in
  *testnet*|*localhost*|*127.0.0.1*|*0.0.0.0*)
    die "LCD must point to an Injective mainnet endpoint"
    ;;
esac
[[ "$CURL_TIMEOUT" =~ ^[1-9][0-9]*$ ]] || die "CURL_TIMEOUT must be a positive integer"
[[ "$MAX_PLATFORM_FEE_BPS" =~ ^[0-9]{1,4}$ ]] || die "MAX_PLATFORM_FEE_BPS must be an integer"
(( MAX_PLATFORM_FEE_BPS <= 500 )) || die "MAX_PLATFORM_FEE_BPS cannot exceed the contract hard cap of 500"
[[ "$USERNAME_FEE_MIN_AMOUNT" =~ ^[1-9][0-9]*$ ]] || die "USERNAME_FEE_MIN_AMOUNT must be positive"
address_re='^inj1[0-9a-z]{38}$'
hash_re='^[[:xdigit:]]{64}$'

[[ "$EXPECTED_UPGRADE_TIMELOCK_SECONDS" =~ ^[1-9][0-9]*$ ]] || die "EXPECTED_UPGRADE_TIMELOCK_SECONDS must be positive"
[[ "$USERNAME_FEE_DENOM" =~ ^[a-z][a-z0-9/-]*$ ]] || die "USERNAME_FEE_DENOM is invalid"
[[ -z "$EXPECTED_WASM_SHA256" || "$EXPECTED_WASM_SHA256" =~ $hash_re ]] || die "EXPECTED_WASM_SHA256 must be a 64-character SHA-256"

validate_expected_address() {
  local label="$1" value="$2"
  [[ -z "$value" || "$value" =~ $address_re ]] || die "$label must be a lowercase inj1... address"
}

validate_expected_address EXPECTED_WASM_ADMIN "$EXPECTED_WASM_ADMIN"
validate_expected_address EXPECTED_CONTRACT_ADMIN "$EXPECTED_CONTRACT_ADMIN"
validate_expected_address EXPECTED_COMMITTEE "$EXPECTED_COMMITTEE"

[[ "$REQUIRED_RESERVED_USERNAMES" != *$'\n'* ]] || die "REQUIRED_RESERVED_USERNAMES must be space-separated"
read -r -a required_reserved <<< "$REQUIRED_RESERVED_USERNAMES"
(( ${#required_reserved[@]} > 0 )) || die "REQUIRED_RESERVED_USERNAMES must not be empty"

curl_json() {
  curl -fsS --max-time "$CURL_TIMEOUT" "$1"
}

node_info="$(curl_json "${LCD%/}/cosmos/base/tendermint/v1beta1/node_info")" || {
  die "failed to query node_info"
}
network="$(printf '%s\n' "$node_info" | jq -er '.default_node_info.network // .node_info.network // empty')" || {
  die "node_info response has no network"
}
[[ "$network" == "$CHAIN_ID" ]] || die "LCD network is $network, expected $CHAIN_ID"

contract_info="$(curl_json "${LCD%/}/cosmwasm/wasm/v1/contract/${CONTRACT}")" || {
  die "failed to query contract info"
}
wasm_admin="$(printf '%s\n' "$contract_info" | jq -er '.contract_info.admin // empty')" || {
  die "contract info response has no Wasm admin"
}
code_id="$(printf '%s\n' "$contract_info" | jq -er '.contract_info.code_id | tostring')" || {
  die "contract info response has no code_id"
}
[[ "$code_id" =~ ^[1-9][0-9]*$ ]] || die "contract code_id is invalid"
[[ "$wasm_admin" =~ $address_re ]] || die "Wasm contract admin is not a valid inj1... address"
[[ -z "$EXPECTED_WASM_ADMIN" || "$wasm_admin" == "$EXPECTED_WASM_ADMIN" ]] || {
  die "Wasm contract admin does not match EXPECTED_WASM_ADMIN"
}

if [[ -n "$EXPECTED_WASM_SHA256" ]]; then
  code_response="$(curl_json "${LCD%/}/cosmwasm/wasm/v1/code/${code_id}")" || {
    die "failed to query code info for code_id=${code_id}"
  }
  deployed_wasm_sha256="$(printf '%s\n' "$code_response" | jq -er '.code_info.data_hash // empty')" || {
    die "code info has no data_hash for code_id=${code_id}"
  }
  [[ "$deployed_wasm_sha256" =~ $hash_re ]] || die "code info data_hash is not a 64-character SHA-256"
  [[ "${deployed_wasm_sha256,,}" == "${EXPECTED_WASM_SHA256,,}" ]] || {
    die "deployed code_id=${code_id} SHA-256 ${deployed_wasm_sha256} does not match EXPECTED_WASM_SHA256"
  }
fi

smart_query() {
  local payload="$1" encoded response
  encoded="$(printf '%s' "$payload" | base64 | tr -d '\n' | jq -sRr @uri)"
  response="$(curl_json "${LCD%/}/cosmwasm/wasm/v1/contract/${CONTRACT}/smart/${encoded}")" || return 1
  printf '%s\n' "$response" | jq -ce '.data | objects'
}

config="$(smart_query '{"config":{}}')" || die "failed to query contract config"
contract_admin="$(printf '%s\n' "$config" | jq -er '.admin // empty')" || die "config has no in-contract admin"
committee="$(printf '%s\n' "$config" | jq -er '.moderation_committee // empty')" || die "config has no moderation committee"
treasury="$(printf '%s\n' "$config" | jq -er '.treasury // empty')" || die "config has no treasury"

[[ "$contract_admin" =~ $address_re ]] || die "in-contract admin is not a valid inj1... address"
[[ "$committee" =~ $address_re ]] || die "moderation committee is not a valid inj1... address"
[[ "$treasury" =~ $address_re ]] || die "treasury is not a valid inj1... address"
[[ "$wasm_admin" == "$contract_admin" ]] || die "Wasm admin and in-contract admin must be the same production multisig"
[[ "$contract_admin" != "$committee" ]] || die "technical admin and moderation committee must be separate"
[[ -z "$EXPECTED_CONTRACT_ADMIN" || "$contract_admin" == "$EXPECTED_CONTRACT_ADMIN" ]] || {
  die "in-contract admin does not match EXPECTED_CONTRACT_ADMIN"
}
[[ -z "$EXPECTED_COMMITTEE" || "$committee" == "$EXPECTED_COMMITTEE" ]] || {
  die "moderation committee does not match EXPECTED_COMMITTEE"
}

platform_fee_bps="$(printf '%s\n' "$config" | jq -er '.platform_fee_bps | tostring')" || die "config has no platform_fee_bps"
[[ "$platform_fee_bps" =~ ^[0-9]+$ ]] || die "platform_fee_bps is not an integer"
(( platform_fee_bps <= MAX_PLATFORM_FEE_BPS )) || {
  die "platform_fee_bps=${platform_fee_bps} exceeds configured cap ${MAX_PLATFORM_FEE_BPS}"
}

fee_denom="$(printf '%s\n' "$config" | jq -er '.username_fee.denom // empty')" || die "config has no username fee denom"
fee_amount="$(printf '%s\n' "$config" | jq -er '.username_fee.amount // empty')" || die "config has no username fee amount"
[[ "$fee_denom" == "$USERNAME_FEE_DENOM" ]] || die "username fee denom is ${fee_denom}, expected ${USERNAME_FEE_DENOM}"
[[ "$fee_amount" =~ ^[1-9][0-9]*$ ]] || die "username fee must be non-zero"

strip_leading_zeros() {
  local value="$1"
  value="${value#"${value%%[!0]*}"}"
  printf '%s' "${value:-0}"
}

fee_amount_normalized="$(strip_leading_zeros "$fee_amount")"
min_fee_normalized="$(strip_leading_zeros "$USERNAME_FEE_MIN_AMOUNT")"
if (( ${#fee_amount_normalized} < ${#min_fee_normalized} )) ||
   { (( ${#fee_amount_normalized} == ${#min_fee_normalized} )) && [[ "$fee_amount_normalized" < "$min_fee_normalized" ]]; }; then
  die "username fee ${fee_amount} is below minimum ${USERNAME_FEE_MIN_AMOUNT}"
fi

jq -e '.reserved_usernames | arrays' <<< "$config" >/dev/null || die "reserved_usernames is not an array"
for name in "${required_reserved[@]}"; do
  jq -e --arg name "$name" '.reserved_usernames | index($name) != null' <<< "$config" >/dev/null || {
    die "required reserved username is missing: $name"
  }
done

upgrade="$(smart_query '{"upgrade_security":{}}')" || die "failed to query upgrade security"
timelock="$(printf '%s\n' "$upgrade" | jq -er '.timelock_seconds | tostring')" || die "upgrade_security has no timelock_seconds"
[[ "$timelock" == "$EXPECTED_UPGRADE_TIMELOCK_SECONDS" ]] || {
  die "upgrade timelock is ${timelock}s, expected ${EXPECTED_UPGRADE_TIMELOCK_SECONDS}s"
}

if jq -e '.proposal != null' <<< "$upgrade" >/dev/null; then
  proposal_hash="$(printf '%s\n' "$upgrade" | jq -er '.proposal.wasm_sha256 // empty')" || die "pending upgrade has no Wasm hash"
  proposed_at="$(printf '%s\n' "$upgrade" | jq -er '.proposal.proposed_at | tostring')" || die "pending upgrade has no proposed_at"
  execute_after="$(printf '%s\n' "$upgrade" | jq -er '.proposal.execute_after | tostring')" || die "pending upgrade has no execute_after"
  [[ "$proposal_hash" =~ $hash_re ]] || die "pending upgrade hash is not a 64-character SHA-256"
  [[ "$proposed_at" =~ ^[0-9]+$ && "$execute_after" =~ ^[0-9]+$ ]] || die "pending upgrade timestamps are invalid"
  (( ${#proposed_at} <= 18 && ${#execute_after} <= 18 )) || die "pending upgrade timestamps are out of range"
  (( execute_after > proposed_at )) || die "pending upgrade execute_after must be in the future"
  (( execute_after - proposed_at == EXPECTED_UPGRADE_TIMELOCK_SECONDS )) || {
    die "pending upgrade does not have the exact 14-day delay"
  }
fi

pending="$(jq -r 'if .proposal == null then "none" else "scheduled" end' <<< "$upgrade")"
printf 'mainnet governance check: network=%s contract=%s admin=%s committee=%s platform_fee_bps=%s username_fee=%s%s timelock=%ss\n' \
  "$network" "$CONTRACT" "$contract_admin" "$committee" "$platform_fee_bps" \
  "${fee_amount}${fee_denom}" " pending=${pending}" "$timelock"
