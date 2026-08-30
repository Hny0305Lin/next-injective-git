# Contract Verification Record

## Current Status

The nine-contract verification result was handled by the project’s own
verification workflow. It is recorded in
`evidence/testnet-deployment-2026-08-25/blockscout-verification.json`.

The old automated Blockscout script was incomplete and has been removed. This
document is retained only as a manual recheck reference; it is not a pending
task for the current cutover.

## Contract Addresses

1. SuiteDirectory — `0x24124cb60F9EF02F7DeB5BC868c028Fb412F5334`
2. BootstrapCoordinator — `0x329921023FCf6E337E924686b92f7521549B5970`
3. RepositoryCore — `0x20269Fb750D77E7c6c812290E7A7B99E0a3780D4`
4. RecoveryModule — `0xa7249cE20B54C4Be440838F0eF403d2a6a07E532`
5. ModerationModule — `0xb957B65634931dD0613abAC296D5a3837BF979Bd`
6. EconomicModule — `0x608380F355bf3D7cb0D3DCD58Fc4DBe695dAF3dd`
7. UsernameModule — `0xc7C164E46788b37e98D27aCe908C489999c09853`
8. BadgeModule — `0x5e4EA68e31f89977BF056BE9cb3d4F1c43AB995D`
9. ReleaseModule — `0xaa0dD566Eb2b21FeD0Ba9426c0e30887cE2de63f`

## Manual Recheck

If a reviewer needs to recheck a contract, open its Injective Testnet
Blockscout address and confirm that the source/bytecode status matches the
canonical JSON evidence. The build settings used by the EVM artifacts are:

- Solidity compiler `0.8.24`
- Optimization enabled, runs `1`
- Via IR enabled

Any new verification result must update the canonical evidence through the
project workflow; do not add a speculative automation script.
