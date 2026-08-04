# Fee Grant Policy

Status: policy gate implemented; automatic issuance and a real gasless Push
remain deployment work in the P1 backlog.

## Default limits

- Each verified platform identity receives at most one active grant.
- A grant covers the first **3 successful `update_ref` Push transactions**.
- The total allowance is capped at **0.03 INJ** and expires after **7 days**.
- An address/identity pair cannot receive another grant for **30 days**.
- The treasury issuer caps grants at **100 per rolling 24 hours** until a
  production budget is approved.

The exact INJ allowance and daily cap are governance configuration, not secrets;
changing them requires an operator review and a new acceptance run.

## Eligibility and anti-Sybil checks

1. The wallet signs a server nonce, proving control of the `inj1...` address.
2. The platform verifies a unique identity record and stores only its
   64-character salted identity hash in the policy state.
3. One identity hash cannot be associated with a second address during the
   cooldown window; denylisted addresses are rejected before broadcast.
4. A grant is recorded only after the feegrant transaction is confirmed with
   code `0`. A Push is recorded only after its `update_ref` transaction is
   confirmed with code `0`.
5. After the third successful Push, the issuer revokes the allowance or lets
   the short TTL expire. Duplicate requests are idempotent by address and
   identity hash.

`scripts/feegrant-policy-gate.sh` enforces these checks in a locked TSV state
file. It deliberately never broadcasts a transaction: the operator must run
the chain-specific Cosmos feegrant command, then record its transaction hash.
This keeps the grant key outside the repository and makes the policy testable
on testnet before enabling automatic issuance.

The repository regression fixture is run with:

```bash
bash scripts/feegrant-policy-gate-test.sh
```

It covers the three-Push limit, duplicate identity, revoke, cooldown expiry
followed by a fresh grant, the daily cap, and malformed-state fail-closed
behavior. It uses temporary state and never broadcasts a transaction.

For the production issuer, `scripts/feegrant-issue.sh` serializes the
eligibility check, `injectived tx feegrant grant`, and state recording under an
issuer lock. It accepts only a successful chain response (`code=0`), restricts
the allowance to at most `30000000000000000inj` (0.03 INJ) and only permits
`/cosmwasm.wasm.v1.MsgExecuteContract`, and leaves the policy state untouched
on broadcast or parsing failure. The treasury key, chain endpoint, and keyring
backend are supplied through environment variables; none are stored in the
repository. Its fake-signer regression test is:

```bash
bash scripts/feegrant-issue-test.sh
```

The wrapper still requires a production identity service to verify the wallet
nonce and produce the salted identity hash, and a live testnet/mainnet run is
needed to prove the granter pays for a real Push.

After the third successful Push, `scripts/feegrant-revoke.sh <inj-address>`
uses the same lock and records the chain revoke transaction only after a
successful `injectived tx feegrant revoke`. Repeated revoke calls are
idempotent at the wrapper level.

`scripts/feegrant-record-push.sh <inj-address> <tx-hash>` is the only production
path for incrementing the Push counter. It queries the transaction first,
requires code `0`, and requires exactly one
`/cosmwasm.wasm.v1.MsgExecuteContract` message containing `update_ref`; failed
or unrelated transactions are not recorded.

## Acceptance

- Testnet: one eligible identity receives one grant; a duplicate identity,
  second address, expired grant, and fourth Push are rejected.
- Query the feegrant allowance after each Push and record the grant/revoke tx
  hashes in the as-built evidence.
- A real gasless Push must show the platform granter paying fees and the user
  paying no INJ; without that chain evidence this item remains incomplete.
