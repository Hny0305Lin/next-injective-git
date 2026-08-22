# Push Setup

The currently implemented IPFS path requires Git, `igit`, `git-remote-igit`,
and native Kubo. Windows, Linux, and macOS run their respective native binaries.
The Windows and Linux paths do not install WSL2, `injectived`, a chain daemon,
or a legacy signer.

Sensitive local state is owner-only on POSIX (`0600` files and `0700`
directories). Native Windows uses a protected DACL that grants full access
only to the current user and `SYSTEM`; POSIX mode bits are not used as Windows
acceptance evidence. Config and keystore-index writes are staged under that
policy before any bytes are written and are then atomically published. EVM key
files are created directly inside the protected keystore directory. Existing
config and EVM keystore files are re-protected before reads; final-path
symbolic links and Windows reparse points fail closed, and indexed key files
outside or below the keystore directory are rejected. Protected replacement
does not write through an existing symbolic-link or hard-link destination.

```sh
igit setup push
igit key import dev
igit config set key_name dev
igit config set evm_suite_directory_address 0x...
igit suite verify
```

Kubo downloads and checksums are pinned in `cli/internal/bootstrap/deps.json`.
Each source has separate connect, response-header, body-idle, and total
timeouts, so a stalled mirror advances to the next pinned URL. The Windows
artifact list uses the fixed IPFS distribution CID through Pinata and the
project HK/US gateways before the upstream distribution and GitHub origins;
all downloaded bytes must still match the pinned SHA-256.
The API must be loopback-only. Clone and fetch do not need Kubo: the helper
reads refs from the verified Suite and downloads packs from HTTPS gateways.

Push adds a temporary local pack, obtains CID-bound durable replication, then
submits `updateRef`. Garbage collection happens only after a successful chain
receipt. Transaction uncertainty returns the hash and retains retryable data.

The Directory address must come from approved cutover evidence. A missing,
inactive, wrong-chain, wrong-version, code-hash-mismatched, or incorrectly bound
Suite fails before signing.

Historical fixed-height queries are available only through `igit archive`; they
are not a push configuration or fallback.

Kubo is the current storage adapter, not a permanent product prerequisite.
Amazon S3 and Cloudflare R2 profiles are planned so those users can push and
fetch without a local Kubo daemon. They are not implemented by the current
Suite or clients; see [ADR 0002](adr/0002-pluggable-pack-storage.md).
