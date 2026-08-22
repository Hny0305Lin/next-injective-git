# Injective EVM Demo Showcase

This repository is a live `igit` demo published from an Injective EVM Testnet
Suite. It exercises ordinary Git history, an on-chain `main` ref, and a real
IPFS-backed Git pack.

The demo intentionally starts with an empty synthetic Suite snapshot. The
history below is new demo data and is unrelated to the legacy CosmWasm
registry or its username escrow.

## What is included

- a small deterministic source module;
- a manifest describing the deployed demo Directory;
- multiple commits so the explorer can show real history;
- a `main` ref backed by a real `ipfs://` pack URI.

## Network

- Injective EVM Testnet, chain ID `1439`;
- SuiteDirectory `0xf8844F90887731FFd607E1f59e39a3918F6eAb35`;
- source verification and runtime binding are visible in Blockscout.

## Demo history

The repository is intentionally updated through ordinary Git commits.
