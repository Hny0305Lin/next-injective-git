// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

import {RepoRegistryV2} from "../src/RepoRegistryV2.sol";

interface VmInvariant {
    function prank(address sender) external;
    function warp(uint256 newTimestamp) external;
}

/// @dev Dependency-free subset of forge-std's StdInvariant targeting API.
/// Forge reads targetContracts() before beginning each invariant campaign.
abstract contract InvariantTargeting {
    address[] private _targetedContracts;

    function targetContract(address target) internal {
        _targetedContracts.push(target);
    }

    function targetContracts() external view returns (address[] memory targets) {
        return _targetedContracts;
    }
}

/// @dev Stateful Foundry handler. Every public action bounds its input and
/// either performs a valid registry transition or returns without changing
/// state, so invariant.fail_on_revert can remain enabled.
contract RepoRegistryV2Handler {
    VmInvariant private constant vm =
        VmInvariant(address(uint160(uint256(keccak256("hevm cheat code")))));

    uint256 private constant ACTOR_COUNT = 4;
    uint256 private constant MAX_REPOSITORIES = 6;
    uint8 private constant REF_SLOT_COUNT = 4;

    string private constant SHA1 = "0123456789abcdef0123456789abcdef01234567";
    string private constant SHA2 =
        "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789";

    RepoRegistryV2 public immutable registry;
    address public immutable deploymentAdmin;

    bool private _initialized;
    bytes32[] private _repoIds;
    mapping(bytes32 repoId => bool tracked) private _tracked;
    mapping(bytes32 repoId => address owner) private _owners;
    mapping(bytes32 repoId => address initialOwner) private _initialOwners;
    mapping(bytes32 repoId => string name) private _names;
    mapping(bytes32 repoId => bool frozen) private _frozen;
    mapping(bytes32 repoId => address target) private _pendingTargets;
    mapping(bytes32 repoId => mapping(address actor => bool known)) private _knownLocators;
    mapping(bytes32 repoId => mapping(uint8 slot => bool exists)) private _refExists;
    mapping(bytes32 repoId => mapping(uint8 slot => uint8 variant)) private _refVariants;

    constructor(RepoRegistryV2 target, address admin_) {
        registry = target;
        deploymentAdmin = admin_;
    }

    function initialize() external {
        if (_initialized) return;
        _initialized = true;
        _createRepo(0);
    }

    function createRepo(uint256 ownerSeed) external {
        if (!_initialized || _repoIds.length >= MAX_REPOSITORIES) return;
        _createRepo(ownerSeed);
    }

    function updateRef(
        uint256 repoSeed,
        uint256 refSeed,
        uint256 shaSeed,
        bool force
    ) external {
        if (_repoIds.length == 0) return;
        bytes32 repoId = _repoIds[repoSeed % _repoIds.length];
        if (_frozen[repoId]) return;

        uint8 slot = uint8(refSeed % REF_SLOT_COUNT);
        string memory expectedSha;
        if (_refExists[repoId][slot]) {
            expectedSha = _sha(_refVariants[repoId][slot]);
        }
        uint8 nextVariant = uint8(shaSeed % 2);
        string[] memory uris = new string[](1);
        uris[0] = _packUri(slot);

        address owner = _owners[repoId];
        vm.prank(owner);
        registry.updateRef(
            owner,
            _names[repoId],
            _refName(slot),
            _sha(nextVariant),
            uris,
            expectedSha,
            force
        );
        _refExists[repoId][slot] = true;
        _refVariants[repoId][slot] = nextVariant;
    }

    function deleteRef(uint256 repoSeed, uint256 refSeed) external {
        if (_repoIds.length == 0) return;
        bytes32 repoId = _repoIds[repoSeed % _repoIds.length];
        uint8 slot = uint8(refSeed % REF_SLOT_COUNT);
        if (_frozen[repoId] || !_refExists[repoId][slot]) return;

        address owner = _owners[repoId];
        vm.prank(owner);
        registry.deleteRef(owner, _names[repoId], _refName(slot));
        _refExists[repoId][slot] = false;
    }

    function beginOwnershipTransfer(uint256 repoSeed, uint256 targetSeed) external {
        if (_repoIds.length == 0) return;
        bytes32 repoId = _repoIds[repoSeed % _repoIds.length];
        if (_pendingTargets[repoId] != address(0)) return;

        address owner = _owners[repoId];
        address target = _differentActor(targetSeed, owner);
        vm.prank(owner);
        registry.beginOwnershipTransfer(repoId, target);
        _pendingTargets[repoId] = target;
    }

    function cancelOwnershipTransfer(uint256 repoSeed) external {
        if (_repoIds.length == 0) return;
        bytes32 repoId = _repoIds[repoSeed % _repoIds.length];
        if (_pendingTargets[repoId] == address(0)) return;

        vm.prank(_owners[repoId]);
        registry.cancelOwnershipTransfer(repoId);
        _pendingTargets[repoId] = address(0);
    }

    function acceptOwnershipTransfer(uint256 repoSeed) external {
        if (_repoIds.length == 0) return;
        bytes32 repoId = _repoIds[repoSeed % _repoIds.length];
        address target = _pendingTargets[repoId];
        if (target == address(0)) return;

        RepoRegistryV2.PendingOwnershipTransfer memory pending =
            registry.pendingOwnershipTransfer(repoId);
        if (block.timestamp > pending.expiresAt) return;
        if (block.timestamp < pending.executeAfter) vm.warp(pending.executeAfter);

        vm.prank(target);
        registry.acceptOwnership(repoId);
        _owners[repoId] = target;
        _knownLocators[repoId][target] = true;
        _pendingTargets[repoId] = address(0);
    }

    function setFrozen(uint256 repoSeed, bool frozen) external {
        if (_repoIds.length == 0) return;
        bytes32 repoId = _repoIds[repoSeed % _repoIds.length];
        if (_frozen[repoId] == frozen) return;

        address owner = _owners[repoId];
        vm.prank(owner);
        registry.setFrozen(owner, _names[repoId], frozen);
        _frozen[repoId] = frozen;
    }

    function assertRegistryMatchesModel() external view {
        require(registry.admin() == deploymentAdmin, "admin changed");
        require(_repoIds.length > 0 && _repoIds.length <= MAX_REPOSITORIES, "repo bound");

        for (uint256 i; i < _repoIds.length; ++i) {
            bytes32 repoId = _repoIds[i];
            RepoRegistryV2.Repo memory repository = registry.getRepoById(repoId);
            require(_tracked[repoId] && repository.exists, "repo missing");
            require(repository.owner == _owners[repoId], "owner mismatch");
            require(_same(repository.name, _names[repoId]), "name mismatch");
            require(
                registry.computeRepoId(_initialOwners[repoId], repository.name) == repoId,
                "stable repo id mismatch"
            );
            require(repository.moderationStatus == (_frozen[repoId] ? 1 : 0), "frozen mismatch");

            _assertLocators(repoId, repository);
            _assertRefs(repoId);

            RepoRegistryV2.PendingOwnershipTransfer memory pending =
                registry.pendingOwnershipTransfer(repoId);
            require(pending.newOwner == _pendingTargets[repoId], "pending target mismatch");
            if (pending.newOwner != address(0)) {
                require(pending.executeAfter >= pending.proposedAt, "transfer delay order");
                require(pending.expiresAt > pending.executeAfter, "transfer expiry order");
            }
        }

        _assertOwnerIndexes();
    }

    function _createRepo(uint256 ownerSeed) private {
        address owner = _actor(ownerSeed % ACTOR_COUNT);
        string memory name = _repoName(_repoIds.length);
        vm.prank(owner);
        bytes32 repoId = registry.createRepo(name, "stateful invariant repository", "main");

        require(!_tracked[repoId], "duplicate repo id");
        _tracked[repoId] = true;
        _repoIds.push(repoId);
        _owners[repoId] = owner;
        _initialOwners[repoId] = owner;
        _names[repoId] = name;
        _knownLocators[repoId][owner] = true;
    }

    function _assertLocators(bytes32 repoId, RepoRegistryV2.Repo memory repository) private view {
        for (uint256 i; i < ACTOR_COUNT; ++i) {
            address actor = _actor(i);
            bool expected = _knownLocators[repoId][actor];
            try registry.resolveRepo(actor, repository.name) returns (
                bytes32 resolvedId,
                bool isCanonical,
                RepoRegistryV2.Repo memory resolved
            ) {
                require(expected, "unexpected locator");
                require(resolvedId == repoId, "locator identity mismatch");
                require(isCanonical == (actor == repository.owner), "canonical locator mismatch");
                require(resolved.owner == repository.owner, "resolved owner mismatch");
            } catch {
                require(!expected, "known locator missing");
            }
        }
    }

    function _assertRefs(bytes32 repoId) private view {
        uint256 expectedCount;
        for (uint8 slot; slot < REF_SLOT_COUNT; ++slot) {
            bool expected = _refExists[repoId][slot];
            if (expected) ++expectedCount;
            try registry.resolveRef(_owners[repoId], _names[repoId], _refName(slot)) returns (
                RepoRegistryV2.Ref memory resolved
            ) {
                require(expected, "deleted ref resolved");
                require(resolved.exists, "resolved ref missing");
                require(
                    keccak256(bytes(resolved.commitSha)) ==
                        keccak256(bytes(_sha(_refVariants[repoId][slot]))),
                    "ref sha mismatch"
                );
            } catch {
                require(!expected, "modeled ref missing");
            }
        }

        (uint256 nextCursor, bool hasMore, string[] memory names, RepoRegistryV2.Ref[] memory refs) =
            registry.listRefsPageById(repoId, 0, registry.MAX_QUERY_PAGE_SIZE());
        require(!hasMore && nextCursor == expectedCount, "ref page cursor mismatch");
        require(names.length == expectedCount && refs.length == expectedCount, "ref page count mismatch");
        uint256 seenMask;
        for (uint256 i; i < names.length; ++i) {
            bool matched;
            for (uint8 slot; slot < REF_SLOT_COUNT; ++slot) {
                if (_same(names[i], _refName(slot))) {
                    require(_refExists[repoId][slot], "page contains deleted ref");
                    uint256 slotMask = uint256(1) << slot;
                    require(seenMask & slotMask == 0, "duplicate ref in page");
                    seenMask |= slotMask;
                    require(
                        keccak256(bytes(refs[i].commitSha)) ==
                            keccak256(bytes(_sha(_refVariants[repoId][slot]))),
                        "page sha mismatch"
                    );
                    matched = true;
                    break;
                }
            }
            require(matched, "unknown ref in page");
        }
        uint256 expectedMask;
        for (uint8 slot; slot < REF_SLOT_COUNT; ++slot) {
            if (_refExists[repoId][slot]) expectedMask |= uint256(1) << slot;
        }
        require(seenMask == expectedMask, "ref page membership mismatch");
    }

    function _assertOwnerIndexes() private view {
        for (uint256 actorIndex; actorIndex < ACTOR_COUNT; ++actorIndex) {
            address owner = _actor(actorIndex);
            uint256 expectedCount;
            for (uint256 i; i < _repoIds.length; ++i) {
                if (_owners[_repoIds[i]] == owner) ++expectedCount;
            }

            (
                uint256 nextCursor,
                bool hasMore,
                bytes32[] memory ids,
                RepoRegistryV2.Repo[] memory repositories
            ) = registry.listReposPage(owner, 0, registry.MAX_QUERY_PAGE_SIZE());
            require(!hasMore && nextCursor == expectedCount, "owner page cursor mismatch");
            require(ids.length == expectedCount && repositories.length == expectedCount, "owner page count mismatch");
            for (uint256 i; i < ids.length; ++i) {
                require(_tracked[ids[i]], "owner page has unknown repo");
                require(_owners[ids[i]] == owner && repositories[i].owner == owner, "owner index mismatch");
                for (uint256 j; j < i; ++j) require(ids[j] != ids[i], "duplicate owner repo");
            }
            for (uint256 i; i < _repoIds.length; ++i) {
                if (_owners[_repoIds[i]] != owner) continue;
                bool found;
                for (uint256 j; j < ids.length; ++j) {
                    if (ids[j] == _repoIds[i]) {
                        found = true;
                        break;
                    }
                }
                require(found, "owner repo omitted");
            }
        }
    }

    function _differentActor(uint256 seed, address owner) private pure returns (address) {
        uint256 actorIndex = seed % ACTOR_COUNT;
        address target = _actor(actorIndex);
        if (target == owner) target = _actor((actorIndex + 1) % ACTOR_COUNT);
        return target;
    }

    function _actor(uint256 index) private pure returns (address) {
        if (index == 0) return address(0xA11CE);
        if (index == 1) return address(0xB0B);
        if (index == 2) return address(0xCA701);
        return address(0xD00D);
    }

    function _repoName(uint256 index) private pure returns (string memory) {
        return string(abi.encodePacked("repo-", bytes1(uint8(0x30 + index))));
    }

    function _refName(uint8 slot) private pure returns (string memory) {
        return string(abi.encodePacked("refs/heads/state-", bytes1(uint8(0x30 + slot))));
    }

    function _packUri(uint8 slot) private pure returns (string memory) {
        return string(abi.encodePacked("ipfs://bafyinvariant", bytes1(uint8(0x30 + slot))));
    }

    function _sha(uint8 variant) private pure returns (string memory) {
        return variant == 0 ? SHA1 : SHA2;
    }

    function _same(string memory left, string memory right) private pure returns (bool) {
        return keccak256(bytes(left)) == keccak256(bytes(right));
    }
}

contract RepoRegistryV2InvariantTest is InvariantTargeting {
    RepoRegistryV2 private registry;
    RepoRegistryV2Handler private handler;

    function setUp() public {
        registry = new RepoRegistryV2(address(this));
        handler = new RepoRegistryV2Handler(registry, address(this));
        handler.initialize();
        targetContract(address(handler));
    }

    function invariantStatefulRegistryMatchesBoundedModel() public view {
        handler.assertRegistryMatchesModel();
    }
}
