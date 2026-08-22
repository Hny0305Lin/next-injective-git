# Injective USDC Integration

Public skill for integrating native USDC on Injective into apps, wallets,
bridge widgets, and trading systems.

Use it when you need to:

- migrate an Injective app from USDT or bridged USDC to native USDC
- add Circle CCTP V2 deposits or withdrawals
- resolve the native USDC EVM address and Cosmos bank denom
- select USDC derivative markets
- debug CCTP attestation or mint recovery

## Installation

Install via NPM into the current project:

```bash
npx skills add InjectiveLabs/agent-skills --skill injective-usdc-integration
```

Install globally:

```bash
npx skills add InjectiveLabs/agent-skills --global --skill injective-usdc-integration
```

## Start Here

Read these in order:

1. [SKILL.md](./SKILL.md) - the integration playbook.
2. [references/constants.md](./references/constants.md) - chain IDs, domains,
   contracts, denoms, and source-chain constants.
3. [references/cctp-v2.md](./references/cctp-v2.md) - CCTP burn, attest, mint
   flow and recovery.
4. [references/frontend-integration.md](./references/frontend-integration.md) -
   browser app checklist and migration gotchas.

## Status

This skill is intentionally docs-only. It does not ship a private-key CLI or
custodial bridge code. Browser apps should use the connected user's wallet for
source-chain burn and destination-chain mint transactions.
