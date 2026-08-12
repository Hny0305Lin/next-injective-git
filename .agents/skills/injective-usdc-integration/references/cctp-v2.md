# Circle CCTP V2 Flow

CCTP is a burn-and-mint protocol for moving native Circle USDC between chains.
It does not lock liquidity in a bridge pool and it does not mint wrapped USDC.

## Deposit Into Injective

For source chain to Injective EVM:

1. Source chain: approve `TokenMessengerV2` to spend native USDC.
2. Source chain: call `depositForBurn`.
3. Off-chain: poll Circle's iris API for message and attestation.
4. Destination chain: switch wallet to Injective EVM.
5. Destination chain: call `MessageTransmitterV2.receiveMessage`.

The V2 `depositForBurn` signature is:

```solidity
function depositForBurn(
  uint256 amount,
  uint32 destinationDomain,
  bytes32 mintRecipient,
  address burnToken,
  bytes32 destinationCaller,
  uint256 maxFee,
  uint32 minFinalityThreshold
) returns (uint64 nonce)
```

For standard transfer into Injective:

```ts
const destinationDomain = 29
const mintRecipient = zeroPadValue(recipientEvmAddress, 32)
const burnToken = sourceChain.usdc
const destinationCaller = '0x' + '0'.repeat(64)
const maxFee = 0n
const minFinalityThreshold = 2000
```

## Withdraw From Injective

For Injective EVM to another CCTP chain, the same flow runs in reverse:

1. Injective EVM: approve Injective USDC to `TokenMessengerV2`.
2. Injective EVM: call `depositForBurn` with the destination domain.
3. Poll Circle iris with source domain `29` and the Injective burn tx hash.
4. Switch wallet to the destination chain.
5. Call destination `MessageTransmitterV2.receiveMessage`.

## Attestation Polling

Use:

```text
GET https://iris-api.circle.com/v2/messages/{sourceDomain}?transactionHash={burnTxHash}
```

Treat the transfer as mintable only when the response contains a message whose:

- `status` is `complete`
- `message` is present
- `attestation` is present and is not `PENDING`

Poll every few seconds. Standard finality waits are source-chain dependent.
Ethereum can take around 13 minutes; L2s and Avalanche are usually shorter.

## Recovery

If burn confirmed and mint did not happen, funds are not lost. Re-fetch the
message and attestation with the burn hash, switch to the destination chain, and
call `receiveMessage` again.

This is why apps should persist or display the burn hash immediately. A user
support flow can resume from that hash without asking the user to burn again.

## Frontend State Model

Recommended statuses:

```ts
type CctpStatus =
  | 'idle'
  | 'approving'
  | 'burning'
  | 'waiting-for-attestation'
  | 'ready-to-mint'
  | 'minting'
  | 'complete'
  | 'failed'
```

Persist enough context after burn to recover:

```ts
interface PendingCctpTransfer {
  sourceDomain: number
  destinationDomain: number
  sourceChainId: number
  destinationChainId: number
  amountBaseUnits: string
  burnTxHash: string
  recipient: `0x${string}`
}
```

## Common Failures

- Wrong domain: chain ID and CCTP domain are different numbers.
- Wrong USDC: `burnToken` must be native Circle USDC on the source chain.
- Missing gas: user needs source native gas for burn and destination native gas
  for mint.
- Lost tab: recover from the burn hash.
- Regulatory hook gas on Injective: a `restricted action` style error can mean
  the EVM hook ran out of gas. Retry with a higher gas limit before assuming
  the transfer is blocked.
