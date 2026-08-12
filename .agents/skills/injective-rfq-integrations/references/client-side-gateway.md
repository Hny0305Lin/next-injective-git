# Client-Side Gateway Mode

Use this reference when a browser app moves RFQ gateway/indexer duties into the
client to reduce click-time latency. Keep the guidance generic: do not depend
on private app routes, private maker lists, or project-specific flags.

## Mental Model

The gateway is not only a transaction helper. It behaves like a short-lived RFQ
coordinator:

- opens RFQ requests
- maps local client ids to acknowledged RFQ ids
- collects signed maker quotes
- filters invalid or stale quotes
- selects executable quotes
- prepares or signs settlement context
- broadcasts quickly enough for quote TTLs
- reconciles selected quotes against chain and indexer state

Client-side gateway mode is safe only when the frontend owns those same duties.
Opening a TakerStream and displaying quotes is not enough.

## When To Use It

Use client-side gateway mode for:

- latency-sensitive browser trading where managed gateway prepare is too slow
- maker-routed flows that need explicit allowlists, denylists, or selection
  policy
- products that need immediate optimistic fills from selected quotes
- operational experiments where quote ACK timing and maker response timing are
  part of the product signal

Prefer the managed RFQ gateway when:

- the product does not need custom quote selection
- the app cannot own account sequence and timeout-height readiness
- monitoring cannot detect quote-hit drops, maker failures, or reconciliation
  drift
- the team wants backend-owned transaction assembly and fee-payer behavior

## Request Lifecycle

1. Build canonical RFQ input from market metadata, quantity, margin, direction,
   and worst-price guardrail.
2. Attach a unique `clientId` for local tracking.
3. Connect one taker stream per wallet/session.
4. Send the prequote or firm request.
5. Track the ACK. Replace local `clientId` with acknowledged `rfqId` when the
   stream returns one.
6. Accept quotes for both the original `clientId` and acknowledged `rfqId`
   during the transition because quotes can arrive before ACK.
7. Start the quote collection window on ACK or first matching quote.
8. Filter quotes before sorting.
9. Select enough quotes to fill the requested quantity.
10. Broadcast without post-selection RPCs when quote TTLs are tight.
11. Emit optimistic state only after quote selection or match, then reconcile
    against chain/indexer state.

## Quote Validation Checklist

Reject a quote unless all required fields match the active request:

- signature is present
- maker address is present
- taker address matches the active wallet/session
- market id matches
- direction matches
- RFQ id or client id matches the active request
- Cosmos chain id is `injective-1`
- EVM chain id, when present, is `1776`
- contract address is the canonical RFQ contract
- quantity is positive and can fill the requested amount after aggregation
- price is inside the worst-price guardrail
- expiry leaves enough TTL for broadcast and inclusion
- maker is allowed by the current routing policy

Sort valid quotes deterministically:

- long taker: lower price first
- short taker: higher price first
- equal price: preserve stable ordering or use explicit secondary criteria such
  as quantity or TTL

## Price And Slippage Semantics

Keep these concepts separate:

- **display quote**: best current maker quote shown in the UI
- **fallback display estimate**: index price plus or minus user slippage when no
  maker quote is present
- **prequote worst price**: quote-discovery guardrail sent to makers
- **final submit worst price**: user-facing execution guardrail

Do not assume prequote worst price equals user slippage. A latency-focused app
can start prequote discovery with tighter RFQ-specific tiers, then widen after
misses. User slippage still belongs in final submit validation and can affect
derived sizing when the user enters a quote notional instead of a base
quantity.

## Sequence, Timeout, And TTL

For manual accept paths, quote TTL can be shorter than the time needed to fetch
account state after selection. Prepare the broadcast path before collecting:

- account number
- sequence
- timeout height or timeout timestamp
- gas limit
- fee payer or broadcaster context
- signer order and pubkey normalization

If quote TTLs are sub-second or low-second, avoid fetching sequence or block
height after quote selection. Select, sign, and broadcast immediately.

## State And Concurrency

Serialize firm accepts per wallet. Per-market locks are not enough because two
parallel accepts can consume or invalidate the same account sequence.

Track active state explicitly:

- stream connection generation
- latest request ids by market and direction
- in-flight firm accept id
- selected quote ids
- optimistic fills
- rollback timeout
- reconciliation status

Optimistic UI should update from selected or matched quote data, not requested
input alone. Clear provisional state on real indexer insert/update/delete,
explicit rollback, account reset, or expiry.

## Portability Pattern

Before moving any gateway/indexer behavior into a client, write down what the
gateway currently owns and split it into three buckets:

- protocol correctness: signing bytes, signer order, chain ids, decimal
  canonicalization, AuthZ grants
- latency path: prequote, quote collection, quote selection, sequence
  readiness, broadcast timing
- observability: ACK timing, quote-hit rate, maker failures, stale state, user
  copy

Move only the latency-critical pieces client-side first. Keep a managed gateway
or backend fallback until the client path has parity tests and production
metrics.

## Tests To Add

Cover these before calling the client-side gateway production-ready:

- quote before ACK is accepted for the active request
- ACK replacement updates latest RFQ id without dropping matching quotes
- long quotes sort lowest price first
- short quotes sort highest price first
- invalid chain id, market id, direction, contract, maker, stale expiry, and
  outside-worst-price quotes are rejected
- collection window closes on timeout and selects enough quantity
- account sequence race is blocked by per-wallet serialization
- selected quote data drives optimistic filled state
- failed post-selection broadcast rolls back optimistic state once
- managed gateway fallback or retry path is reachable when no valid quotes are
  collected
