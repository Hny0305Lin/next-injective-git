# Legacy Documentation Paths

During the file-structure standardization, documentation is being reorganized into
subdirectories:

| Old path | New canonical (planned) |
|---|---|
| `docs/architecture.md` | `docs/architecture/overview.md` |
| `docs/infrastructure.md` | `docs/operations/infrastructure.md` |
| `docs/monitor.md` | `docs/operations/monitor.md` |
| `docs/pinning-infrastructure.md` | `docs/operations/pinning.md` |
| `docs/feegrant-policy.md` | `docs/operations/feegrant-policy.md` |
| `docs/evm-v2-migration.md` | `docs/migration/evm-v2-migration.md` |
| `docs/target-topology-migration.md` | `docs/migration/target-topology-migration.md` |
| `docs/archived-repositories.md` | `docs/migration/archived-repositories.md` |
| `docs/frontend-improvement-analysis.md` | `docs/frontend/improvement-analysis.md` |
| `docs/gogs-frontend-redesign.md` | `docs/frontend/gogs-redesign.md` |
| `docs/ci-web-publishing.md` | `docs/frontend/ci-web-publishing.md` |
| `docs/delivery-roadmap.md` | `docs/product/delivery-roadmap.md` |
| `docs/release.md` | `docs/product/release.md` |
| `docs/project-status.md` | `docs/product/project-status.md` |
| `docs/backlog.md` | `docs/product/backlog.md` |
| `docs/p0-evidence.md` | `docs/archive/p0-evidence.md` |
| `docs/acceptance-evidence.md` | `docs/archive/acceptance-evidence.md` |

Old paths remain readable during the shim period; new contributions should use
the canonical locations. The canonical index is `docs/README.md:1`.
