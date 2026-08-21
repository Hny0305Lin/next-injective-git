# `src/modules` — Business module contracts (planned)

Will house domain contracts extracted from flat `src/`:

- `RepositoryCore.sol`
- `RecoveryModule.sol`
- `ModerationModule.sol`
- `EconomicModule.sol`
- `UsernameModule.sol`
- `BadgeModule.sol`
- `ReleaseModule.sol`

`SuiteDirectory.sol` and `BootstrapCoordinator.sol` remain at `src/`; `suite/` holds base types.

Migration is `git mv src/<Module>.sol src/modules/<Module>.sol` + `foundry.toml` remap (no logic change).
