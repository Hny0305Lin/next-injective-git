# Injective USDC Constants

Verify these values against the linked public docs when making a production
change. Circle can add new source chains over time.

## Injective Mainnet

| Value | Constant |
|---|---|
| Cosmos chain ID | `injective-1` |
| Injective EVM chain ID | `1776` |
| CCTP domain | `29` |
| Native USDC EVM address | `0xa00C59fF5a080D2b954d0c75e46E22a0c371235a` |
| Native USDC Cosmos denom | `erc20:0xa00c59ff5a080d2b954d0c75e46e22a0c371235a` |
| USDC decimals | `6` |
| Injective EVM RPC | `https://sentry.evm-rpc.injective.network` |
| Injective explorer | `https://blockscout.injective.network` |

Injective docs list USDC as a native stablecoin available through Circle CCTP
and MultiVM Token Standard. That means EVM and Cosmos flows reference the same
token, but with different identifiers.

## Injective Testnet

| Value | Constant |
|---|---|
| Injective EVM testnet chain ID | `1439` |
| CCTP domain | `29` |
| Testnet USDC EVM address | `0x0C382e685bbeeFE5d3d9C29e29E341fEE8E84C5d` |
| TokenMessengerV2 | `0x8FE6B999Dc680CcFDD5Bf7EB0974218be2542DAA` |
| MessageTransmitterV2 | `0xE737e5cEBEEBa77EFE34D4aa090756590b1CE275` |
| TokenMinterV2 | `0xb43db544E2c27092c107639Ad201b3dEfAbcF192` |

Testnet tokens have no financial value. Always use Circle's current docs before
shipping testnet constants because testnet deployments can change.

## CCTP V2 Contracts

Mainnet EVM CCTP V2 contract addresses are deterministic across the common EVM
chains listed by Circle, including Injective.

| Contract | Mainnet address |
|---|---|
| TokenMessengerV2 | `0x28b5a0e9C621a5BadaA536219b3a228C8168cf5d` |
| MessageTransmitterV2 | `0x81D40F21F12A8F0E3252Bccb954D722d4c464B64` |
| TokenMinterV2 | `0xfd78EE919681417d192449715b2594ab58f5D002` |
| MessageV2 | `0xec546b6B005471ECf012e5aF77FBeC07e0FD8f78` |

## Core Source Chains

These are the common source chains most browser bridge UIs should start with.
They are widely supported by wallets and have native Circle USDC.
This is also the source-chain set used by current Injective browser apps that
support inbound CCTP.

| Slug | Display | EVM chain ID | Wallet hex | CCTP domain | Native USDC | Gas |
|---|---|---:|---|---:|---|---|
| `ethereum` | Ethereum | `1` | `0x1` | `0` | `0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48` | ETH |
| `avalanche` | Avalanche C-Chain | `43114` | `0xa86a` | `1` | `0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E` | AVAX |
| `optimism` | OP Mainnet | `10` | `0xa` | `2` | `0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85` | ETH |
| `arbitrum` | Arbitrum One | `42161` | `0xa4b1` | `3` | `0xaf88d065e77c8cC2239327C5EDb3A432268e5831` | ETH |
| `base` | Base | `8453` | `0x2105` | `6` | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` | ETH |
| `polygon` | Polygon PoS | `137` | `0x89` | `7` | `0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359` | POL |

Recommended aliases:

```ts
export const CCTP_SOURCE_ALIASES = {
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
} as const
```

## Additional CCTP V2 Domains

Circle's current CCTP V2 EVM docs list additional domains such as Unichain,
Linea, Codex, Sonic, World Chain, Monad, Sei, XDC, HyperEVM, Ink, Plume, Arc,
EDGE, Morph, and Pharos. If a product wants to enable one of these, add it as a
data entry after verifying all of the following from Circle and the chain's
wallet metadata:

- EVM chain ID
- CCTP domain
- native USDC contract address
- RPC URLs that work in browsers
- block explorer URL
- wallet `wallet_addEthereumChain` parameters

Do not infer EVM chain IDs from CCTP domain IDs. They are unrelated.

## TypeScript Shape

Use a single source of truth for chain config:

```ts
export interface CctpChain {
  slug: string
  name: string
  chainId: number
  chainHex: `0x${string}`
  domain: number
  usdc: `0x${string}`
  rpcUrls: string[]
  blockExplorerUrls: string[]
  nativeCurrency: { name: string; symbol: string; decimals: 18 }
}
```

Keep UI labels, wallet switching, allowance calls, `burnToken`, and attestation
polling all driven from this config. A common bug is selecting Base in the UI
but still polling Arbitrum domain `3` or approving the Arbitrum USDC contract.
