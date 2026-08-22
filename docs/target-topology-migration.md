# Runtime Topology

The supported topology has one immutable EVM Suite and, today, an IPFS data
plane.
CLI, Web, and the remote helper resolve contracts from `SuiteDirectory` and
share the same verification rules. Kubo is push-only on the client; replicated
pins and HTTPS gateways serve clone/fetch. The chain never stores Git objects.

IPFS/Kubo is the current adapter rather than the permanent product core. The
planned topology allows storage profiles such as Amazon S3 and Cloudflare R2 so
users of those profiles do not install Kubo. This is not implemented by the
current Suite, which accepts only `ipfs://`; it requires a successor URI
contract and migration described in
[ADR 0002](adr/0002-pluggable-pack-storage.md).

The old chain is outside this runtime topology. Its source and operational
material live under `archive/cosmwasm-v1`; `igit archive` exposes fixed-height
read-only evidence commands for migration and audit.

See [architecture](architecture.md), [push setup](push-setup.md),
[migration](evm-v2-migration.md), and
[ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md).
