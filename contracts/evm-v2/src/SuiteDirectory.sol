// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {
    IBootstrapCoordinator,
    IBootstrapModule,
    ISuiteBound,
    SuiteIds
} from "./suite/ISuite.sol";

contract SuiteDirectory {
    uint64 public constant suiteVersion = 3;

    enum State {
        Bootstrapping,
        Active
    }

    State public state;
    uint256 public immutable configuredChainId;
    bytes32 public immutable snapshotRoot;
    address public bootstrapAuthority;
    address public bootstrapCoordinator;
    bytes32 public bootstrapCoordinatorCodeHash;

    mapping(bytes32 id => address module) public moduleAddress;
    mapping(bytes32 id => bytes32 codeHash) public moduleCodeHash;
    uint256 public registeredModuleCount;

    error InvalidBootstrapAuthority();
    error InvalidChainId(uint256 configured, uint256 actual);
    error InvalidSnapshotRoot();
    error Unauthorized(address caller);
    error DirectoryAlreadyActive();
    error CoordinatorAlreadyConfigured();
    error InvalidCoordinator();
    error InvalidModuleId(bytes32 id);
    error ModuleAlreadyConfigured(bytes32 id);
    error InvalidModule(address module);
    error CodeHashMismatch(bytes32 expected, bytes32 actual);
    error ModuleBindingMismatch(bytes32 id, address module);
    error MissingModule(bytes32 id);
    error CoordinatorNotReady();

    event CoordinatorConfigured(address indexed coordinator, bytes32 indexed codeHash);
    event ModuleConfigured(bytes32 indexed id, address indexed module, bytes32 indexed codeHash);
    event SuiteActivated(uint64 indexed version, uint256 indexed chainId, bytes32 indexed snapshotRoot);

    constructor(address authority, uint256 chainId, bytes32 snapshotRoot_) {
        if (authority == address(0)) revert InvalidBootstrapAuthority();
        if (chainId == 0 || chainId != block.chainid) revert InvalidChainId(chainId, block.chainid);
        if (snapshotRoot_ == bytes32(0)) revert InvalidSnapshotRoot();
        bootstrapAuthority = authority;
        configuredChainId = chainId;
        snapshotRoot = snapshotRoot_;
    }

    function setBootstrapCoordinator(address coordinator, bytes32 expectedCodeHash) external {
        if (state != State.Bootstrapping) revert DirectoryAlreadyActive();
        if (msg.sender != bootstrapAuthority) revert Unauthorized(msg.sender);
        if (bootstrapCoordinator != address(0)) revert CoordinatorAlreadyConfigured();
        if (coordinator == address(0) || coordinator.code.length == 0) revert InvalidCoordinator();
        bytes32 actualCodeHash = coordinator.codehash;
        if (expectedCodeHash == bytes32(0) || actualCodeHash != expectedCodeHash) {
            revert CodeHashMismatch(expectedCodeHash, actualCodeHash);
        }
        if (
            IBootstrapCoordinator(coordinator).suiteDirectory() != address(this)
                || IBootstrapCoordinator(coordinator).snapshotRoot() != snapshotRoot
        ) revert InvalidCoordinator();

        bootstrapCoordinator = coordinator;
        bootstrapCoordinatorCodeHash = actualCodeHash;
        bootstrapAuthority = address(0);
        emit CoordinatorConfigured(coordinator, actualCodeHash);
    }

    function configureModule(bytes32 id, address module, bytes32 expectedCodeHash) external {
        if (state != State.Bootstrapping) revert DirectoryAlreadyActive();
        if (msg.sender != bootstrapCoordinator || msg.sender == address(0)) revert Unauthorized(msg.sender);
        if (!_isRequiredModule(id)) revert InvalidModuleId(id);
        if (moduleAddress[id] != address(0)) revert ModuleAlreadyConfigured(id);
        if (module == address(0) || module.code.length == 0) revert InvalidModule(module);
        bytes32 actualCodeHash = module.codehash;
        if (expectedCodeHash == bytes32(0) || actualCodeHash != expectedCodeHash) {
            revert CodeHashMismatch(expectedCodeHash, actualCodeHash);
        }

        ISuiteBound bound = ISuiteBound(module);
        if (
            bound.suiteDirectory() != address(this) || bound.bootstrapCoordinator() != bootstrapCoordinator
                || bound.moduleId() != id
        ) revert ModuleBindingMismatch(id, module);

        moduleAddress[id] = module;
        moduleCodeHash[id] = actualCodeHash;
        unchecked {
            registeredModuleCount++;
        }
        emit ModuleConfigured(id, module, actualCodeHash);
    }

    function activate() external {
        if (state != State.Bootstrapping) revert DirectoryAlreadyActive();
        if (msg.sender != bootstrapCoordinator || msg.sender == address(0)) revert Unauthorized(msg.sender);
        if (block.chainid != configuredChainId) revert InvalidChainId(configuredChainId, block.chainid);
        for (uint256 i; i < SuiteIds.REQUIRED_MODULES; ++i) {
            bytes32 id = requiredModuleAt(i);
            address module = moduleAddress[id];
            if (module == address(0)) revert MissingModule(id);
            if (module.codehash != moduleCodeHash[id]) {
                revert CodeHashMismatch(moduleCodeHash[id], module.codehash);
            }
            if (!IBootstrapModule(module).bootstrapFinalized()) revert CoordinatorNotReady();
        }
        if (!IBootstrapCoordinator(bootstrapCoordinator).readyForActivation()) {
            revert CoordinatorNotReady();
        }
        state = State.Active;
        emit SuiteActivated(suiteVersion, configuredChainId, snapshotRoot);
    }

    function requiredModuleAt(uint256 index) public pure returns (bytes32) {
        if (index == 0) return SuiteIds.CORE;
        if (index == 1) return SuiteIds.RECOVERY;
        if (index == 2) return SuiteIds.MODERATION;
        if (index == 3) return SuiteIds.ECONOMIC;
        if (index == 4) return SuiteIds.USERNAME;
        if (index == 5) return SuiteIds.BADGE;
        if (index == 6) return SuiteIds.RELEASE;
        revert InvalidModuleId(bytes32(index));
    }

    function verifyModule(bytes32 id) external view returns (bool) {
        address module = moduleAddress[id];
        return module != address(0) && module.code.length != 0 && module.codehash == moduleCodeHash[id]
            && ISuiteBound(module).suiteDirectory() == address(this)
            && ISuiteBound(module).bootstrapCoordinator() == bootstrapCoordinator
            && ISuiteBound(module).moduleId() == id;
    }

    function _isRequiredModule(bytes32 id) private pure returns (bool) {
        return id == SuiteIds.CORE || id == SuiteIds.RECOVERY || id == SuiteIds.MODERATION
            || id == SuiteIds.ECONOMIC || id == SuiteIds.USERNAME || id == SuiteIds.BADGE
            || id == SuiteIds.RELEASE;
    }
}
