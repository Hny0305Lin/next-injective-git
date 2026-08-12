# Push environment setup

> **Migration note:** this page documents the current CosmWasm V1 testnet
> signer path. It is retained for legacy repositories and migration tests. The
> V2 product path is `igit setup` with an EVM signer and native Kubo; it must not
> require WSL2, `injectived`, or a user-managed Cosmos keyring. Track the cutover
> gates in [evm-v2-migration.md](./evm-v2-migration.md).

Clone and Fetch remain lightweight: they require Git, `igit`, the remote
helper, LCD access, and one healthy HTTPS read gateway. Push additionally
requires a local signer and temporary Kubo block serving.

## Recommended commands

```bash
igit setup
igit doctor --clone
igit doctor --push
igit doctor --push --json
igit setup push
igit setup push --yes
igit setup push --no-kubo
igit setup push --force
igit setup push --create-key dev
igit setup status --json
igit setup upgrade --yes
```

`setup push` selects one of two internal paths. An EVM V2 profile validates the
reviewed contract address first, then installs or reuses Kubo only. It never
checks or installs `injectived`. A legacy V1 profile installs or reuses both
`injectived` and Kubo. Both paths then:

1. Download only artifacts pinned in `cli/internal/bootstrap/deps.json` and
   verify SHA-256 before extraction. Upstream mirrors provide the same pinned
   bytes; none can bypass the hash.
2. Extract only manifest-listed files. The V1 Injective package additionally
   contains the required `libwasmvm` library.
3. Create user-local wrappers under `~/.igit/bin`; the Injective wrapper
   provides its private library path without modifying `/usr/lib`.
4. Initialize Kubo with the `server` profile when no repository exists, enable
   a user systemd service where available, and wait for its RPC API. Windows
   and systems without a user systemd manager use a detached daemon.
5. Persist the resolved absolute Kubo path and backend profile in
   `~/.igit/config.json` without storing a private key.
6. Optionally create a key only when `--create-key NAME` is explicit.
7. Run the complete Push doctor and print remaining actions, including a
   zero-balance faucet warning.

The installer never replaces a working user-managed binary by default.
`--force` downloads and extracts into a staging directory before atomically
replacing only the versioned directory beneath `~/.igit/deps`; a download or
verification failure leaves the working version intact. It does not modify
`/usr/local`, `/usr`, or another package manager's files. Kubo logs are written
to `~/.igit/kubo.log`.

## Push preflight

The Git remote helper runs a lightweight preflight once per Push batch before
resolving refs or generating a pack. A normal update requires the signer,
Injective RPC, local Kubo API, and upload configuration. A ref deletion does
not require Kubo because it sends only an on-chain transaction.

Failures are returned through the remote-helper protocol as one-line errors
with a repair command. `git push` and `igit push` therefore have the same
behavior.

## Windows native V2 and legacy WSL2

The V2 source path now includes a pinned Kubo v0.42.0 `windows-amd64` ZIP,
extracts `kubo/ipfs.exe`, starts it as a detached native process, and stores its
absolute path. Unit tests prove the Kubo-only setup path does not inspect
`injectived`. The opt-in native smoke test downloads the pinned ZIP, verifies
it, initializes an isolated repository, starts the daemon, probes its API, and
cleans up the exact PID it created; this has passed on Windows and is included
in native Windows CI:

```powershell
cd cli
$env:IGIT_RUN_NATIVE_KUBO_INTEGRATION = "1"
go test -run '^TestNativeKuboLifecycleIntegration$' -count=1 -v ./internal/bootstrap
Remove-Item Env:IGIT_RUN_NATIVE_KUBO_INTEGRATION
```

This is native Kubo readiness, not a live V2 testnet claim: the checked-in
network profile still has no reviewed V2 contract address, so product setup
stops before downloading or creating a key.

The current public CosmWasm V1 testnet still requires the entire Linux
toolchain inside WSL2. From a source checkout:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\bootstrap-push.ps1 `
  -Distro Ubuntu-24.04 -LinuxUser alice -Yes -CreateKey dev
```

When the distribution is first registered, the script creates a non-root
Linux user (derived from the Windows username unless `-LinuxUser` is given),
grants that user passwordless sudo for unattended package installation, and
sets it as the distribution default. Existing distributions and their default
users are never changed.

From a tagged Windows release:

```powershell
.\igit-windows-amd64.exe setup push `
  --wsl Ubuntu-24.04 --yes --create-key dev
```

The release executable checks the Linux CLI version in WSL. If absent or
different, it downloads matching Linux assets and verifies them against the
same release's `checksums.txt` before invoking Linux `igit setup push`.

`wsl.exe --unregister <distribution>` permanently deletes the distribution,
including `~/.injectived` keyrings, Kubo data, SSH keys, and all files not
stored on Windows mounts. Inventory and back up unique credentials before
using that command. Reinstalling dependencies cannot recover a mnemonic or
private SSH key.

## Doctor exit status

`OK` means the requirement is ready. `WARN` is actionable but does not make
the command fail, such as a zero key balance. `FAIL` makes `doctor` exit
nonzero. `SKIP` means a prerequisite for that check was absent.

JSON output has a stable top-level `mode` and `checks` array so install scripts
can inspect status without parsing aligned terminal text.

The pinned Injective npm platform package contains a Zstandard-compressed tar
payload. The CLI uses its Go zstd dependency directly; Node.js, npm, `zstd`,
and a system package installation are not required on the target machine.

## V2 setup contract

When the EVM backend is enabled, `igit setup` must use a named network profile
and encrypted signer store. A successful setup has these observable properties:

- `igit config list` contains profile metadata, never a mnemonic or raw private
  key;
- `igit key new dev` creates or imports an EVM key without invoking
  `injectived`;
- `igit key show` prints only the canonical `inj1...` address;
- `igit setup` installs/reuses native Kubo through the Kubo-only bootstrap and
  never probes `injectived`;
- `igit doctor` checks registry JSON-RPC, receipt polling, signer availability,
  read gateways and Kubo as one backend-neutral report;
- WSL2 and Cosmos keyring checks are shown only when a user explicitly selects
  the legacy V1 compatibility path.

The V2 keystore defaults to `~/.igit/keystore` and contains encrypted geth
keystore JSON plus a non-secret key-name index. `config.json` never stores a
private key or mnemonic. `IGIT_EVM_KEY_PASSWORD` is an automation-only input
for isolated tests; normal `igit key new NAME` and transaction signing read a
password without echoing it. Prompts open the controlling terminal directly
(`/dev/tty` on POSIX, `CONIN$`/`CONOUT$` on Windows), so Git remote-helper
stdin/stdout remain dedicated to the Git protocol. A missing or unreviewed `evm_contract_address`
causes setup and writes to stop before any transaction is signed. The legacy
CosmWasm `contract_address` is never substituted for that field.

The automatic V2 compatibility profile can read a typed locator miss from V1,
but that result is Clone/Fetch-only: Push and Delete stop before Kubo, signing,
or RPC broadcast. An explicit `evm/v2` profile has no LCD fallback. Selecting a
different named network resets all profile-owned transport, chain, explorer,
and contract values together; apply an operator override only after selection.

The current repository does not publish a V2 deployment address yet. Therefore
the V2 setup contract is a guarded readiness path, not a claim that testnet V2
writes are live. The native installer, checksum, extraction, no-`injectived`
boundary, setup selection, and isolated Windows daemon smoke are covered; the
release gate must still add the reviewed address and complete real Push on clean
Windows/Linux machines before promoting the profile from V1 compatibility to
V2 writes.
