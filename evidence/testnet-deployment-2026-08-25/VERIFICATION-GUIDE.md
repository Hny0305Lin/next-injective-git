# Blockscout Contract Verification Guide

## Current Status

All nine deployed contract verification results are recorded in
`blockscout-verification.json`. Verification was performed through the project’s
own workflow rather than the incomplete automation that was previously present.
No automated Blockscout verifier is part of this repository.

## Contract Inventory

| Contract | Address | Evidence |
|---|---|---|
| SuiteDirectory | `0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334` | `verified` |
| BootstrapCoordinator | `0x329921023FCf6E337E924686b92f7521549B5970` | `verified` |
| RepositoryCore | `0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4` | `verified` |
| RecoveryModule | `0xa7249cE20B54C4Be440838F0eF403d2a6a07E532` | `verified` |
| ModerationModule | `0xb957B65634931dD0613abAC296D5a3837BF979Bd` | `verified` |
| EconomicModule | `0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd` | `verified` |
| UsernameModule | `0xc7C164E46788b37e98D27aCe908C489999c09853` | `verified` |
| BadgeModule | `0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D` | `verified` |
| ReleaseModule | `0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f` | `verified` |

## Manual Recheck Settings

When independently rechecking a result in Blockscout, use the EVM build
settings that produced the deployment artifacts:

- Solidity `0.8.24`
- Optimization enabled with `1` run
- Via IR enabled
- The source and constructor data from `contracts/evm-v2/`

The recheck is informational unless the canonical evidence is deliberately
updated through a new reviewed deployment record.

## Evidence Boundary

This document does not authorize V1 migration or EVM writes. The current scope
is `fresh-empty-suite`; V1 remains an isolated read-only archive preview.
