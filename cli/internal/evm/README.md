# `internal/evm` — EVM transport layer (planned)

This package will own pure-EVM concerns extracted from `internal/chain`:

- `evm_rpc.go`, `evm_transactor.go`, `evm_nonce.go`, `evm_gas.go`, `evm_keystore.go`, `evm_address.go`
- `abi/` — generated ABIs (from `contracts/evm-v2/abi`)

During the migration, `internal/chain` remains the canonical import path.
New code should prefer `internal/evm` once the split lands.
See `docs/LEGACY_PATHS.md` and `plan/file-structure-standardization.md:3.3`.
