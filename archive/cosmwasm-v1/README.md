# CosmWasm V1 Archive

This directory preserves the deployed CosmWasm V1 contract, frozen protocol
documentation, historical operator scripts, and offline snapshot fixtures.
It is not part of the default iGit build, CI, release, CLI runtime, Web runtime,
or Git remote helper.

Ordinary V1 access is read-only through `igit archive`. The archived scripts
are retained as historical evidence and are not supported deployment or write
paths. The on-chain V1 contract is unchanged.

To validate the frozen source independently:

```sh
cargo +1.81.0 test --locked \
  --manifest-path archive/cosmwasm-v1/contracts/repo-registry/Cargo.toml
bash archive/cosmwasm-v1/scripts/v1-export-snapshot-test.sh
```
