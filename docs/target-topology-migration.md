# Runtime Topology

The supported topology has one immutable EVM Suite and an IPFS data plane.
CLI, Web, and the remote helper resolve contracts from `SuiteDirectory` and
share the same verification rules. Kubo is push-only on the client; replicated
pins and HTTPS gateways serve clone/fetch. The chain never stores Git objects.

The old chain is outside this runtime topology. Its source and operational
material live under `archive/cosmwasm-v1`; `igit archive` exposes fixed-height
read-only evidence commands for migration and audit.

See [architecture](architecture.md), [push setup](push-setup.md), and
[migration](evm-v2-migration.md).
