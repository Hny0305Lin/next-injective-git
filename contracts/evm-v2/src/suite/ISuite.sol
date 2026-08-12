// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

library SuiteIds {
    bytes32 internal constant CORE = keccak256("igit.module.repository-core");
    bytes32 internal constant RECOVERY = keccak256("igit.module.recovery");
    bytes32 internal constant MODERATION = keccak256("igit.module.moderation");
    bytes32 internal constant ECONOMIC = keccak256("igit.module.economic");
    bytes32 internal constant USERNAME = keccak256("igit.module.username");
    bytes32 internal constant BADGE = keccak256("igit.module.badge");
    bytes32 internal constant RELEASE = keccak256("igit.module.release");
    uint256 internal constant REQUIRED_MODULES = 7;
}

interface ISuiteDirectory {
    function state() external view returns (uint8);
    function suiteVersion() external view returns (uint64);
    function configuredChainId() external view returns (uint256);
    function snapshotRoot() external view returns (bytes32);
    function bootstrapCoordinator() external view returns (address);
    function moduleAddress(bytes32 id) external view returns (address);
    function moduleCodeHash(bytes32 id) external view returns (bytes32);
}

interface ISuiteBound {
    function suiteDirectory() external view returns (address);
    function bootstrapCoordinator() external view returns (address);
    function moduleId() external pure returns (bytes32);
}

interface IBootstrapModule is ISuiteBound {
    function beginBootstrap(
        uint256 expectedCount,
        uint256 expectedBatches,
        bytes32 expectedRoot,
        bytes32 snapshotRoot
    ) external;

    function bootstrapImport(
        uint256 sequence,
        uint256 count,
        bytes32 payloadHash,
        bytes calldata payload
    ) external;

    function finalizeBootstrap() external;
    function bootstrapFinalized() external view returns (bool);
}

interface IBootstrapCoordinator {
    function suiteDirectory() external view returns (address);
    function snapshotRoot() external view returns (bytes32);
    function readyForActivation() external view returns (bool);
}

interface IModerationPolicy {
    function requireRefMutation(bytes32 repoId, address actor) external view;
    function requireEconomicAction(bytes32 repoId, address actor) external view;
}

interface IRepositoryCore {
    struct Repository {
        bytes32 id;
        address owner;
        string name;
        string description;
        string defaultBranch;
        bytes32 forkedFrom;
        uint64 createdAt;
        uint64 updatedAt;
        bool exists;
    }

    function getRepository(bytes32 repoId) external view returns (Repository memory);
    function canMaintain(bytes32 repoId, address actor) external view returns (bool);
    function recoverOwnership(bytes32 repoId, address newOwner) external;
}
