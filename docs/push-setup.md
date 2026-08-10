# Push environment setup

Clone and Fetch remain lightweight: they require Git, `igit`, the remote
helper, LCD access, and one healthy HTTPS read gateway. Push additionally
requires a local signer and temporary Kubo block serving.

## Recommended commands

```bash
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

`setup push` performs these operations:

1. Reuses working `injectived` and Kubo commands unless `--force` is set.
2. Otherwise downloads the platform artifact pinned in
   `cli/internal/bootstrap/deps.json` and verifies its SHA-256 before
   extraction. Content-addressed gateways and upstream release hosts are
   ordered download sources for the same bytes; none can bypass the hash.
3. Extracts only the named binary and required `libwasmvm` library. Other
   archive content is ignored.
4. Creates user-local wrappers under `~/.igit/bin`; the Injective wrapper
   provides the private library path without modifying `/usr/lib`.
5. Initializes Kubo with the `server` profile when no repository exists,
   enables a user systemd service where available, and waits for its RPC API.
   Systems without a user systemd manager use a detached daemon fallback.
6. Writes the deployed testnet contract and resolved executable paths to
   `~/.igit/config.json`.
7. Optionally creates a key only when `--create-key NAME` is explicit.
8. Runs the complete Push doctor and prints remaining actions, including a
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

## Windows and WSL2

The supported Windows Push topology is an entire Linux toolchain inside WSL2.
From a source checkout:

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
