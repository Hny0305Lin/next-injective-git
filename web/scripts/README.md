# `web/scripts` vs `scripts/`

- `web/scripts/` — Web-specific (e.g., `release-profile-check.mjs`)
- `scripts/` — Repository-wide gates (suite-readiness, evm checks, deployment, ops)

Do not duplicate cross-cutting gates.
