# Release pipeline (as implemented)

The tag workflow in `.github/workflows/release.yml` runs for tags beginning
with `v` and accepts semantic versions such as `v0.4.0` (including prerelease
or build metadata). Invalid tags fail before any build starts.

Each accepted tag builds the Go CLI and remote helper for these targets:

- `linux/amd64`, `linux/arm64`
- `darwin/amd64`, `darwin/arm64`
- `windows/amd64`

The `igit` binaries embed the complete tag (for example, `igit v0.4.0`),
while local builds without linker flags report `igit dev`. The workflow runs
the same version test with and without the `-X main.version=...` linker
override, then executes the Linux amd64 release binary as an additional check.

The `repo-registry.wasm` artifact is built with Rust `1.81.0`, the committed
`Cargo.lock`, and `wasm32-unknown-unknown`. The build is run from
`contracts/repo-registry` so its `.cargo/config.toml` is applied. The release
checks the Wasm magic (`00 61 73 6d`) and generates a SHA-256 `checksums.txt`
covering exactly the eleven published artifacts.

## Checksum publication status

`checksums.txt` is published as a GitHub Release asset. The release workflow
does not hold the production admin key and therefore does not automatically
write to Injective. After the release is published, an administrator can
register the exact asset hashes on-chain from a configured `igit` client:

```bash
bash scripts/register-release.sh release v0.5.0
```

The command sends one `register_release` transaction. Registration is
immutable for a `(version, platform)` pair; a corrected build must use a new
version. Anyone can then verify a downloaded file without signing:

```bash
igit release verify v0.5.0 igit-linux-amd64 ./igit-linux-amd64
```

The chain record, not the GitHub asset metadata, is the authoritative checksum
source. The contract address and admin key used for registration must be
configured separately and must never be committed to the repository.

## Upgrade governance

The contract admin should be a production multisig. Before a version upgrade,
the multisig submits `schedule_upgrade` with the exact Wasm SHA-256. The
contract exposes this proposal through `upgrade_security` and rejects
`migrate` until 14 days have elapsed and the migration message contains the
same hash. `scripts/schedule-upgrade.sh` creates the announcement; the
`scripts/testnet-migrate.sh` flow accepts the hash explicitly when executing
the delayed migration. Cancelling a proposal is available through the
`cancel_upgrade` execute message.

Before enabling a production reaper or registering the first release, run the
read-only mainnet governance preflight with the deployed contract and the
expected multisig addresses:

```bash
LCD=https://lcd.injective.network \
CONTRACT=inj1... \
EXPECTED_ADMIN=inj1... \
EXPECTED_COMMITTEE=inj1... \
EXPECTED_WASM_SHA256=<release-wasm-sha256> \
bash scripts/mainnet-governance-check.sh
```

The check confirms that the LCD reports `injective-1`, the Wasm and
in-contract admin are aligned, the moderation committee is separate, the
treasury and username policy are configured, the platform fee is at most 500
basis points, and the upgrade timelock is exactly 14 days. It is a configuration
gate only. When `EXPECTED_WASM_SHA256` is supplied, it also queries the LCD
`data_hash` for the contract's `code_id` and compares it to the release
artifact. It does not create multisigs or substitute for on-chain transaction
evidence.

The reusable local validator is `scripts/verify-release-assets.sh`; run it
from Linux, WSL, or another POSIX shell. It checks asset names and sizes,
version output, Wasm magic, and exact checksum contents.

Local WSL verification for `v0.5.0` has passed for all eleven release assets.
This is a build-pipeline result only; the first production
`RegisterRelease` transaction remains pending until a mainnet contract and
authorized admin signer are available.
