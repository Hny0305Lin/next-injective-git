// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "../src/RepoRegistryV2.sol";

interface VmGas {
    function prank(address sender) external;
}

/// @dev Executable regression ceilings for representative user writes. These
/// measure the external contract call after arguments have been prepared; the
/// limits intentionally leave room for transaction intrinsic/calldata gas.
contract RepoRegistryV2GasTest {
    VmGas private constant vm = VmGas(address(uint160(uint256(keccak256("hevm cheat code")))));

    uint256 private constant CREATE_REPO_GAS_CEILING = 600_000;
    uint256 private constant CREATE_REF_FOUR_URIS_GAS_CEILING = 1_200_000;
    uint256 private constant NORMAL_UPDATE_GAS_CEILING = 700_000;
    uint256 private constant FORCE_UPDATE_EIGHT_URIS_GAS_CEILING = 1_800_000;
    uint256 private constant DELETE_REF_GAS_CEILING = 500_000;
    uint256 private constant BEGIN_TRANSFER_GAS_CEILING = 500_000;
    uint256 private constant CANCEL_TRANSFER_GAS_CEILING = 350_000;

    address private constant ALICE = address(0xA11CE);
    address private constant BOB = address(0xB0B);
    string private constant SHA1 = "0123456789abcdef0123456789abcdef01234567";
    string private constant SHA2 =
        "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789";

    RepoRegistryV2 private registry;

    event GasCeilingObserved(bytes32 indexed operation, uint256 used, uint256 ceiling);

    function setUp() public {
        registry = new RepoRegistryV2(address(this));
    }

    function testGasCreateRepoStaysUnderCeiling() public {
        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        registry.createRepo("demo", "representative repository", "main");
        _enforce("createRepo", beforeGas - gasleft(), CREATE_REPO_GAS_CEILING);
    }

    function testGasCreateRefWithFourPackUrisStaysUnderCeiling() public {
        _createDemo();
        string[] memory uris = _uris(4, 0);
        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        registry.updateRef(ALICE, "demo", "refs/heads/main", SHA1, uris, "", false);
        _enforce("updateRef:new", beforeGas - gasleft(), CREATE_REF_FOUR_URIS_GAS_CEILING);
    }

    function testGasNormalUpdateStaysUnderCeiling() public {
        _createDemo();
        vm.prank(ALICE);
        registry.updateRef(ALICE, "demo", "refs/heads/main", SHA1, _uris(1, 0), "", false);

        string[] memory uris = _uris(1, 1);
        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        registry.updateRef(ALICE, "demo", "refs/heads/main", SHA2, uris, SHA1, false);
        _enforce("updateRef:normal", beforeGas - gasleft(), NORMAL_UPDATE_GAS_CEILING);
    }

    function testGasForceUpdateWithEightPackUrisStaysUnderCeiling() public {
        _createDemo();
        vm.prank(ALICE);
        registry.updateRef(ALICE, "demo", "refs/heads/main", SHA1, _uris(1, 0), "", false);

        string[] memory uris = _uris(8, 2);
        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        registry.updateRef(ALICE, "demo", "refs/heads/main", SHA2, uris, "", true);
        _enforce("updateRef:force", beforeGas - gasleft(), FORCE_UPDATE_EIGHT_URIS_GAS_CEILING);
    }

    function testGasDeleteRefStaysUnderCeiling() public {
        _createDemo();
        vm.prank(ALICE);
        registry.updateRef(ALICE, "demo", "refs/heads/main", SHA1, _uris(1, 0), "", false);

        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        registry.deleteRef(ALICE, "demo", "refs/heads/main");
        _enforce("deleteRef", beforeGas - gasleft(), DELETE_REF_GAS_CEILING);
    }

    function testGasBeginOwnershipTransferStaysUnderCeiling() public {
        _createDemo();
        bytes32 repoId = registry.computeRepoId(ALICE, "demo");
        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        registry.beginOwnershipTransfer(repoId, BOB);
        _enforce("transfer:begin", beforeGas - gasleft(), BEGIN_TRANSFER_GAS_CEILING);
    }

    function testGasCancelOwnershipTransferStaysUnderCeiling() public {
        _createDemo();
        bytes32 repoId = registry.computeRepoId(ALICE, "demo");
        vm.prank(ALICE);
        registry.beginOwnershipTransfer(repoId, BOB);

        vm.prank(ALICE);
        uint256 beforeGas = gasleft();
        registry.cancelOwnershipTransfer(repoId);
        _enforce("transfer:cancel", beforeGas - gasleft(), CANCEL_TRANSFER_GAS_CEILING);
    }

    function _createDemo() private {
        vm.prank(ALICE);
        registry.createRepo("demo", "representative repository", "main");
    }

    function _uris(uint256 count, uint256 offset) private pure returns (string[] memory uris) {
        uris = new string[](count);
        for (uint256 i; i < count; ++i) {
            uris[i] = string(abi.encodePacked("ipfs://bafygas", bytes1(uint8(0x30 + offset + i))));
        }
    }

    function _enforce(bytes32 operation, uint256 used, uint256 ceiling) private {
        emit GasCeilingObserved(operation, used, ceiling);
        require(used <= ceiling, "gas ceiling exceeded");
    }
}
