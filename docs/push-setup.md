# Push Setup

The normal Windows, Linux, and macOS path requires Git, `igit`,
`git-remote-igit`, and Kubo. It never installs a chain daemon or a legacy
signer.

```sh
igit setup push
igit key import dev
igit config set key_name dev
igit config set evm_suite_directory_address 0x...
igit suite verify
```

Kubo downloads and checksums are pinned in `cli/internal/bootstrap/deps.json`.
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
