# `cli/tools` — Non-release demo and one-off utilities

Binaries here are **not** part of the release toolchain (`igit`, `git-remote-igit`).

Moved from `cli/cmd/*-demo*`:

- `derive-address-demo/`
- `evm-demo-*`
- `evm-key-import-demo/`
- `simple-key-import/`

They are excluded from `cli/dist` build matrix and `go vet` release gates.
