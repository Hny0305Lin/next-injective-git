# Runtime Topology

The supported topology has one immutable EVM Suite and, today, an IPFS data
plane.
CLI, Web, and the remote helper resolve contracts from `SuiteDirectory` and
share the same verification rules. Kubo is push-only on the client; replicated
pins and HTTPS gateways serve clone/fetch. The chain never stores Git objects.

IPFS/Kubo is the Suite v3 adapter rather than the permanent product core. The
supported topology now dispatches by suite version: v3 repositories use the
IPFS data plane (Kubo push-only, replicated pins and HTTPS gateways for
clone/fetch), while v4 repositories use user-owned Amazon S3 / Cloudflare R2
buckets (BYOS, delivered — no Kubo) through the successor URI commitment
contract described in
[ADR 0002](adr/0002-pluggable-pack-storage.md) and
[ADR 0004](adr/0004-mainnet-storage-neutral-successor-and-byos-scope.md).

The old chain is outside this runtime topology. Its source and operational
material live under `archive/cosmwasm-v1`; `igit archive` exposes fixed-height
read-only evidence commands for migration and audit.

See [architecture](architecture.md), [push setup](push-setup.md),
[migration](evm-v2-migration.md), and
[ADR 0001](adr/0001-evm-v2-runtime-and-migration-scope.md).
