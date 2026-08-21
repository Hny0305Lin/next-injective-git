# External Agent Skills (Read-only)

This directory contains **externally sourced** [Agent Skills](https://github.com/InjectiveLabs/agent-skills) vendored via `skills-lock.json`.

- Source: `InjectiveLabs/agent-skills` (GitHub)
- Lockfile: `skills-lock.json` at repository root
- Updates: `npx skills add <skill>` then commit both `skills-lock.json` and the vendored directory

## Rules

- **Do not hand-edit** files under `.agents/skills/` — they are overwritten on skill updates.
- **Do not add project source** here — project code lives in `cli/`, `contracts/`, `web/`, `scripts/`, `docs/`.
- **CI does not depend** on this directory for build gates; it is developer assistance only.

## Current Skills

19 skills are vendored (see `skills-lock.json` for hashes):

- `injective-ai-cost-optimization`, `injective-cli`, `injective-core-detect-changes`
- `injective-evm-developer`, `injective-faucet`, `injective-frontend-wallet`
- `injective-funding`, `injective-mcp-servers`, `injective-rfq-integrations`
- `injective-trading-*` (6 variants), `injective-usdc-integration`, `injective-wallet-ops`

See `skills-lock.json:1` for exact versions.
