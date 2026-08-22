# Immutable EVM Suite

The current EVM control plane is a non-upgradeable suite rooted at one
`SuiteDirectory` address. `BootstrapCoordinator` binds one snapshot root,
imports seven modules in a fixed order, and activates the directory only after
every module verifies its item count, batch count, and rolling commitment.

Production contracts:

- `RepositoryCore`: stable repository identity, locators and aliases,
  metadata, refs, collaborators, ownership transfers, bounded forks, and the
  Recovery-only ownership capability.
- `RecoveryModule`: guardians and timelocked ownership recovery.
- `ModerationModule`: status, reports, appeals, trails, and mandatory policy
  hooks used by Core and Economic.
- `EconomicModule`: native INJ sponsorship, revenue splits, fees, treasury
  settlement, and queryable migrated totals for every historical denom.
- `UsernameModule`: reserved names and limited original-owner claims without
  migrating V1 escrow liabilities.
- `BadgeModule`: non-transferable badges with recipient and repository indexes.
- `ReleaseModule`: immutable `(version, platform) -> sha256` records.

There are no proxies, upgrade entry points, diamonds, or `delegatecall` paths.
Module addresses and runtime code hashes are configured once and frozen when
the directory becomes active.

## Bootstrap

Deployment order is Directory, Coordinator, Core, Recovery, Moderation,
Economic, Username, Badge, and Release. The bootstrap operator registers each
module with its runtime code hash, then imports modules in exactly that order.
Every batch is bounded, ordered, payload-hashed, and folded into a module-local
rolling commitment. Username finalization additionally requires a hash-bound
attestation that every V1 username deposit was refunded and the V1 escrow
balance is zero. Directory activation is atomic and permanent.

Normal Core ref changes and Economic sponsorship call the Directory-bound
Moderation policy before changing state. Core ownership recovery accepts calls
only from the Directory-bound Recovery module. These are enforced capabilities,
not advisory client conventions.

## Build And Test

Install the locked Solidity compiler and run the portable gate:

```sh
npm ci
npm run check
```

`npm run abi` refreshes the nine checked-in suite ABIs. The gate fixes Solidity
at `0.8.24`, uses the same optimizer/viaIR settings as Foundry, compiles the
suite state-machine tests, rejects forbidden upgrade primitives, and enforces
EIP-170 and EIP-3860 limits. Foundry additionally executes bootstrap ordering,
double-finalize, abandoned-suite, malicious binding, policy hook, capability,
gas, and stateful invariant checks:

```sh
forge build
forge test -vvv
forge test --match-contract '^SuiteArchitectureTest$' -vvv
forge test --gas-report
```

The undeployed `RepoRegistryV2` alpha contracts, ABIs, tests, and client codecs
have been removed. Network profiles accept only a `SuiteDirectory` address.
