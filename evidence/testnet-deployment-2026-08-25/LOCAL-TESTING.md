# Local Testing Guide for Testnet Deployment

## Current Status

P1.2 deployment evidence and P1.3 fresh-empty activation are complete. The
checked-in CLI and Web public profiles remain unchanged until the common P1.4–P1.6
evidence gate and independent approval pass.

**Network:** Injective Testnet (Chain ID 1439)
**SuiteDirectory:** `0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334`

See `contract-addresses.json` for all nine contract addresses and
`suite-verification.json` for the active empty-suite state.

## Local CLI Configuration

Edit the local, uncommitted `~/.igit/config.json`:

```json
{
  "network": "injective-testnet",
  "evm_suite_directory_address": "0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334",
  "evm_rpc": "https://k8s.testnet.json-rpc.injective.network",
  "evm_chain_id": 1439
}
```

For one-off tests, use the local environment override:

```bash
export IGIT_SUITE_DIRECTORY=0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334
```

Never place private keys, passphrases, or activation logs in the repository.

## Web Testing

The deployed testnet address may be selected through the local Web settings or
browser storage. The V1 archive preview must remain read-only and isolated from
ordinary EVM write paths.

## Remaining P1 Gate

1. Clean Linux and Windows Git E2E.
2. MetaMask/Web write receipts and finality checks.
3. Independent security review and hash-bound approval.
4. Final checksum manifest and readiness-gate pass.

Until those steps pass, the testnet deployment is evidence-backed but is not a
public production cutover.
