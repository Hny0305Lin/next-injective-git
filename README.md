# Next Injective Git (`igit`)

Next Injective Git currently stores Git packfiles through an IPFS/Kubo data
plane and stores repository identity, refs, permissions, recovery, moderation,
sponsorship, usernames, badges, and release checksums in a non-upgradeable
Injective EVM Suite.

The ordinary CLI, Git remote helper, and Web app have one chain trust root: a
`SuiteDirectory` address. Clients verify the EVM chain ID, suite version,
`Active` state, module runtime code hashes, and module-to-directory bindings
before use. There is no proxy, diamond, `delegatecall`, legacy write fallback,
or mixed backend mode.

> The checked-in public profiles intentionally contain no SuiteDirectory yet.
> Commands that need the chain fail closed until reviewed deployment, migration,
> fixed-block verification, and cutover evidence have passed. No deployment or
> public testnet availability is claimed by this repository state.

## Why EVM V2

The V1 control plane used CosmWasm. Its Push workflow ran natively on Linux,
while the supported Windows path required WSL2 to host the Linux CLI,
`injectived`, and Kubo. This successor is called **EVM V2** because moving the
control plane to Injective EVM is the mechanism used to remove that
Windows-only compatibility environment. The target path uses native Windows or
native Linux tooling; ordinary EVM V2 users do not install WSL2 or
`injectived`.

This remains an acceptance target, not a completed deployment claim. A clean
Windows machine must complete `init`, `push`, `clone`, `fetch`, `pull`, and ref
deletion without WSL2, and clean Linux must do the same without `injectived`.
See [the runtime and migration ADR](docs/adr/0001-evm-v2-runtime-and-migration-scope.md).

Kubo/IPFS is the current storage adapter, not the long-term product core. The
roadmap calls for pluggable pack storage, including Amazon S3 and Cloudflare R2,
so an object-storage profile can operate without a local Kubo daemon. That
support is not implemented in this repository state: the current Suite and
clients accept only `ipfs://` pack URIs. See
[the storage ADR](docs/adr/0002-pluggable-pack-storage.md).

## Components

| Path | Purpose |
|---|---|
| `contracts/evm-v2/` | Immutable Solidity Suite, checked ABIs/artifacts, and Foundry tests |
| `cli/` | `igit`, `git-remote-igit`, deployment and offline migration tools |
| `web/` | React/Vite browser UI using viem for Suite reads and legacy EVM transactions |
| `scripts/` | Source, release, migration evidence, IPFS replication, and operations gates |
| `archive/cosmwasm-v1/` | Isolated read-only V1 source, protocol material, and evidence tools |

The Suite consists of `SuiteDirectory`, `BootstrapCoordinator`,
`RepositoryCore`, `RecoveryModule`, `ModerationModule`, `EconomicModule`,
`UsernameModule`, `BadgeModule`, and `ReleaseModule`. See
[architecture](docs/architecture.md) for their boundaries.

## User Setup

The currently implemented IPFS profile requires Git and native Kubo for push.
It does not require WSL2 or `injectived`. Clone and fetch use configured HTTPS
IPFS gateways and do not require a local Kubo daemon. Planned S3/R2 profiles
will require separate implementation and acceptance before they can replace
this setup.

```sh
cd cli
go build -o igit ./cmd/igit
go build -o git-remote-igit ./cmd/git-remote-igit

./igit setup push
./igit key import dev
./igit config set key_name dev
./igit config set evm_suite_directory_address 0x...
./igit suite verify
```

`igit key import` reads the private key without terminal echo and writes an
scrypt-encrypted local keystore. Never use the previously published testnet
private key. A Directory address may be configured only from approved cutover
evidence.

After a profile has been activated and verified:

```sh
igit init my-repo "hello chain"
igit push inj main
igit clone igit://alice/my-repo
```

The remote helper supports normal Git push, clone, fetch, pull, and ref delete.
Historical locators resolve through immutable aliases to the canonical repo ID.

## Development

```sh
# Go
(cd cli && go vet ./... && go test ./...)

# Web
(cd web && npm ci && npm run test:api && npm run typecheck && npm run build)

# Solidity, fixed solc 0.8.24
npm ci --prefix contracts/evm-v2
bash scripts/evm-v2-check.sh --required

# Full immutable-Suite source gate
bash scripts/suite-readiness.sh --required
```

Foundry is pinned by CI. All Injective EVM writes use estimated gas, explicit
legacy transaction type, and a gas price of at least `160000000 wei`.

## Deployment And Migration

Deployment is an explicit operator workflow and is restricted to rotated,
encrypted testnet keys. It writes `deployment.json` with no-clobber semantics:

```sh
(cd cli && go run ./cmd/igit-deploy-suite --check \
  --artifacts ../contracts/evm-v2/artifacts)
```

The broadcast form additionally requires the reviewed source commit, snapshot
root, exact confirmation, output path, and encrypted key. It deploys but does
not import or activate.

`igit-suite-migrate build` creates a deterministic unsigned bootstrap plan and
calldata manifest from a complete hash-bound snapshot. `igit-suite-migrate
verify` verifies those files without RPC, keys, signing, or broadcast.

The default profile is changed only after this gate passes against real,
reviewed evidence:

```sh
bash scripts/migration-cutover-readiness.sh EVIDENCE_DIR EXPECTED_COMMIT
```

The gate checks evidence hashes, approval binding, deployment receipts,
compiler/source identity, nine runtime code hashes, final active Directory
state, seven module bindings, fixed-block state comparison, clean-environment
Git E2E, Web receipts, and security review. Fixture output is never deployment
evidence. See [release and cutover](docs/release.md).

## V1 Archive

The old chain remains unchanged as an archival fact source. It has no ordinary
CLI, Web, remote-helper, CI, release, or write path. Read and verify explicit
fixed-height evidence with:

```sh
igit archive query --lcd URL --contract inj1... --height N '{"config":{}}'
igit archive inventory --tx-search txs.json --block-evidence block.json \
  --chain-id injective-888 --contract inj1... --height N --output inventory.json
igit archive verify --snapshot snapshot.json --inventory inventory.json \
  --tx-search txs.json --block-evidence block.json
```

Archive maintenance instructions live only in
[`archive/cosmwasm-v1`](archive/cosmwasm-v1/README.md).

## License

Apache-2.0. See [LICENSE](LICENSE).
