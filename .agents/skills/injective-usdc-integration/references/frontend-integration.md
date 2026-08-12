# Frontend Integration Checklist

Use this checklist when adding native USDC to an Injective browser app.

## Token Metadata

Add one shared constant module:

```ts
export const NATIVE_USDC_EVM_ADDRESS =
  '0xa00C59fF5a080D2b954d0c75e46E22a0c371235a'

export const NATIVE_USDC_DENOM =
  `erc20:${NATIVE_USDC_EVM_ADDRESS.toLowerCase()}`

export const USDC_DECIMALS = 6
```

Normalize denoms case-insensitively. Cosmos denom strings should use the
lowercase EVM address after `erc20:`.

## Amount Math

USDC has 6 decimals. Convert display amounts with decimal or integer math:

```ts
function usdcToBaseUnits(input: string): bigint {
  if (!/^\d+(\.\d{1,6})?$/.test(input)) throw new Error('Invalid USDC amount')
  const [whole, frac = ''] = input.split('.')
  return BigInt(whole) * 1_000_000n + BigInt((frac + '000000').slice(0, 6))
}
```

Do not use JavaScript floating point for signed amounts, margin, allowance, or
base-unit conversion.

## Balance Display

When using Injective indexer or bank APIs:

- Native USDC bank denom is `erc20:<lowercase-address>`.
- Display as `USDC` with 6 decimals.
- Do not label Peggy USDC and native USDC as the same asset without showing the
  route or denom somewhere in debug UI.

When using EVM RPC:

- Read ERC-20 `balanceOf` on `0xa00C59fF5a080D2b954d0c75e46E22a0c371235a`.
- Display the same token as native USDC.

## Market Selection

When migrating perps from USDT to USDC:

1. Query active derivative markets.
2. Prefer markets whose quote token or ticker is USDC.
3. Match by explicit denom when available:
   `erc20:0xa00c59ff5a080d2b954d0c75e46e22a0c371235a`.
4. Fall back to ticker checks like `/USDC`, but only after confirming market
   status is active.
5. Update UI labels and validation copy from USDT to USDC.

Avoid silently selecting an inactive USDC market or a legacy USDT market when
the user asked for USDC collateral.

## Bridge Widget UX

A production CCTP widget should show:

- source chain selector
- amount input capped by balance and product limits
- destination fixed to Injective for deposits, or selectable for withdrawals
- source native gas requirement
- destination INJ gas requirement
- approval tx hash when approval was needed
- burn tx hash as soon as burn is submitted
- attestation waiting state
- mint tx hash
- resume from burn hash

Recommended source labels for the common set:

```ts
const SOURCE_ALIASES = {
  arb: 'arbitrum',
  'arbitrum-one': 'arbitrum',
  eth: 'ethereum',
  mainnet: 'ethereum',
  op: 'optimism',
  'op-mainnet': 'optimism',
  matic: 'polygon',
  poly: 'polygon',
  avax: 'avalanche',
  'avalanche-c-chain': 'avalanche',
}
```

## Wallet Switching

Use `wallet_switchEthereumChain` for known chains. If the wallet reports code
`4902`, call `wallet_addEthereumChain` with data from your `CctpChain` config.

Do not hardcode Arbitrum in helper functions. Every source-dependent operation
should receive the selected source config:

- wallet switch target
- USDC approval contract
- `burnToken`
- `sourceDomain` for iris polling
- gas symbol in UI copy

## Tests To Add

Add focused tests for:

- denom normalization
- 6-decimal amount conversion and rejection of over-precise input
- source chain alias resolution
- quote or preview route using selected source chain
- CCTP poll URL uses selected `source.domain`
- UI copy changes gas token when source changes
- USDC market filtering prefers active USDC markets
