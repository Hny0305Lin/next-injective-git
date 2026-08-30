# ADR 0001: Adopt Injective EVM V2 And An Immutable Suite Runtime

- Status: Accepted for source architecture
- Decision date: 2026-08-12
- Documented: 2026-08-14
- Delivery status: Cutover pending; not released
- Supersedes: Foundation assumptions recorded in `cbd3d44`

> [!IMPORTANT]
> [ADR 0003](0003-fresh-evm-suite-and-v1-archive-preview.md) supersedes
> this ADR's mandatory V1 import requirements for the current testnet cutover.
> The accepted current scope starts the EVM Suite empty and keeps V1 as an
> explicit read-only archive preview.

## Context

The CosmWasm V1 Push workflow ran natively on Linux. The supported Windows
workflow depended on WSL2 to host the Linux CLI, `injectived`, and Kubo. This
platform asymmetry was the primary product reason for a successor built on
Injective EVM rather than an execution-environment-neutral V2 rewrite.

The initial EVM foundation proposed V2-first reads with a narrowly scoped V1
read fallback during migration, V2-only writes, no V1 write fallback, and no
double write. It also contained a signed migration runner coupled to the
initial monolithic contract layout. The later immutable Suite rewrite changed
these assumptions enough that the decisions and their user-visible
consequences must be explicit.

## Decision

1. The successor remains named **EVM V2** because the execution-environment
   change is the mechanism used to remove WSL2 and `injectived` from ordinary
   Windows operation while preserving a native Linux path.
2. The control plane is a non-upgradeable nine-contract Suite rooted at one
   `SuiteDirectory`. Ordinary clients accept no direct module address, proxy,
   mixed backend, or legacy fallback.
3. Ordinary CLI, Web, and remote-helper operation is EVM-only. The former V1
   read fallback is superseded by explicit fixed-height archive evidence and a
   verified one-time import into the Suite.
4. The initial monolithic EVM registry alpha is superseded by
   `SuiteDirectory`, `BootstrapCoordinator`, `RepositoryCore`, and six bounded
   business modules with immutable code-hash-verified bindings.
5. The foundation-era migration runner is not reused against the Suite. Plan
   and calldata generation remain unsigned and offline until a new operator
   runner receives independent review and implements append-only journaling,
   safe resume, receipt verification, and fixed-block imported-state evidence.
6. EVM V2 source readiness does not satisfy the platform goal. Acceptance
   requires a clean native Windows Git workflow without WSL2 or `injectived`, a
   clean native Linux workflow without `injectived`, Web receipts, imported
   state parity, finality handling, and independent security approval.

The **EVM V2** product-generation name does not imply Suite protocol version 2.
The current `SuiteDirectory` protocol version is 3.

## Consequences

### Positive

- Windows and Linux converge on EVM JSON-RPC, an encrypted EVM keystore, and
  operating-system-native tooling.
- Ordinary clients have one contract trust root and cannot silently cross from
  an EVM error into a legacy backend.
- No V1 write fallback or double-write state can occur.
- Immutable module bindings, ordered imports, and evidence-gated activation
  make partial or substituted deployment state detectable.

### Breaking And Operational

- An unimported V1 repository is not transparently cloneable through the
  ordinary client. Archive access is an evidence workflow, not a compatibility
  backend.
- Every V1 repository intended to remain supported must be inventoried,
  imported, compared, and exposed through verified historical aliases before
  cutover.
- Deployment requires nine contracts, complete inventory, deterministic
  imports, module parity, activation evidence, and fixed-block verification.
- No current operator runner signs or broadcasts the Suite migration manifest;
  this is a delivery blocker, not an optional enhancement.
- Future incompatible protocol changes require a successor Suite and explicit
  migration rather than an in-place upgrade.

## Acceptance

Public profiles remain empty until all release evidence passes. In particular,
CI configuration is not acceptance evidence: the exact reviewed commit must
have retained Linux and native Windows results, real migration and Web receipts,
and an independent hash-bound approval.

Pack-storage portability is a related but separate decision. See
[ADR 0002](0002-pluggable-pack-storage.md).
