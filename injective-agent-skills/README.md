# Injective Agent Skills (Qoder Plugin)

Qoder-native plugin packaging three Injective skills from
[InjectiveLabs/agent-skills](https://github.com/InjectiveLabs/agent-skills)
(sourced from a local pnpm global install of the `agent-skills` package).

## Included Skills

| Skill | Description |
| --- | --- |
| `injective-cli` | Use the `injectived` binary to query and transact against an Injective chain with consistent wallet, endpoint, and gas handling. Includes a CLI command map (`references/injectived-cli-map.md`) and a regeneration script (`scripts/map_injectived_cli.py`). |
| `injective-faucet` | Create and operate an INJ faucet for initializing fresh Injective wallets (solves the "no gas without account, no account without gas" bootstrap problem). |
| `injective-wallet-ops` | Mass create, derive, and manage Injective wallets: BIP-44 HD derivation, ETH/INJ address conversion, batch funding with INJ or USDT. |

## Provenance

- Source: `agent-skills` npm package by InjectiveLabs (Apache-2.0; individual
  skills carry their own LICENSE and TERMS_OF_USE, preserved in each skill
  folder).
- Logo: locally generated placeholder (`assets/avatar.svg`), not an official
  Injective asset.
- Skills copied verbatim; no content was modified.

## Omitted

The upstream package contains 18 more skills (trading, RFQ, EVM developer,
MCP servers, linear-cli, etc.) that were intentionally not packaged per user
selection. Re-run the packaging with additional skill folders to include them.

## Setup Notes

- `injective-cli` expects an `injectived` binary (or `npm i -g injective-core`).
- `injective-faucet` references the `injective-trading-autosign` skill
  (`uses:` frontmatter), which is not included in this plugin; the faucet
  workflow itself works standalone.
- No credentials are bundled. Wallet/faucet operations require your own keys.

## Install

From this directory's parent:

```bash
qodercli plugin install --scope user ./injective-agent-skills
```

Or install via Qoder UI: Plugins → Install from local folder.

## Validation

- `python scripts/validate_qoder_plugin.py injective-agent-skills` (bundled
  create-plugin validator) — see conversion session for result.
