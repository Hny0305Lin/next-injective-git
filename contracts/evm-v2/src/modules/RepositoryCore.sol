// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {
    IModerationPolicy,
    IRepositoryCore,
    IEconomicOwnershipHook,
    IRecoveryState,
    ISuiteDirectory,
    SuiteIds
} from "../suite/ISuite.sol";
import {SuiteModule} from "../suite/SuiteModule.sol";

contract RepositoryCore is SuiteModule, IRepositoryCore {
    uint256 public constant MAX_NAME_LENGTH = 64;
    uint256 public constant MAX_DESCRIPTION_LENGTH = 1024;
    uint256 public constant MAX_REF_NAME_LENGTH = 256;
    uint256 public constant MAX_PACK_URIS = 128;
    uint256 public constant MAX_PACK_URI_LENGTH = 512;
    uint256 public constant MAX_REFS_PER_REPO = 1024;
    uint256 public constant MAX_COLLABORATORS_PER_REPO = 256;
    uint256 public constant MAX_FORK_REFS = 64;
    uint256 public constant MAX_QUERY_PAGE_SIZE = 64;
    uint64 public constant OWNERSHIP_TRANSFER_DELAY = 7 days;
    uint64 public constant OWNERSHIP_TRANSFER_WINDOW = 30 days;

    enum Role {
        None,
        Maintainer,
        Reader
    }

    struct GitRef {
        string commitSha;
        string[] packUris;
        uint64 updatedAt;
        address updatedBy;
        bool exists;
    }

    struct PendingOwnershipTransfer {
        address newOwner;
        uint64 executeAfter;
        uint64 expiresAt;
    }

    struct ImportRepository {
        bytes32 id;
        address owner;
        string name;
        string description;
        string defaultBranch;
        bytes32 forkedFrom;
        uint64 createdAt;
        uint64 updatedAt;
    }

    struct ImportAlias {
        bytes32 repoId;
        address owner;
        string name;
    }

    struct ImportRef {
        bytes32 repoId;
        string refName;
        string commitSha;
        string[] packUris;
        uint64 updatedAt;
        address updatedBy;
    }

    struct ImportCollaborator {
        bytes32 repoId;
        address account;
        Role role;
    }

    mapping(bytes32 repoId => Repository repository) private _repositories;
    mapping(bytes32 locator => bytes32 repoId) private _locators;
    mapping(bytes32 repoId => mapping(bytes32 refId => GitRef gitRef)) private _refs;
    mapping(bytes32 repoId => string[] names) private _refNames;
    mapping(bytes32 repoId => mapping(address account => Role role)) private _roles;
    mapping(bytes32 repoId => address[] accounts) private _collaborators;
    mapping(bytes32 repoId => mapping(address account => uint256 indexPlusOne)) private _collaboratorIndexes;
    mapping(address owner => bytes32[] repoIds) private _ownerRepositories;
    mapping(bytes32 repoId => uint256 indexPlusOne) private _ownerRepositoryIndexes;
    mapping(bytes32 repoId => PendingOwnershipTransfer pending) private _pendingTransfers;
    mapping(bytes32 locator => bytes32 repoId) private _reservedLocators;

    error EmptyInput();
    error InvalidRepositoryName(string name);
    error InvalidRefName(string name);
    error InvalidCommitSha(string commitSha);
    error InvalidPackUri(string uri);
    error InputTooLong(uint256 length, uint256 maximum);
    error TooManyPackUris(uint256 count, uint256 maximum);
    error TooManyRefs(uint256 count, uint256 maximum);
    error TooManyCollaborators(uint256 count, uint256 maximum);
    error ForkTooLarge(uint256 count, uint256 maximum);
    error RepositoryNotFound(bytes32 repoId);
    error LocatorNotFound(address owner, string name);
    error LocatorUnavailable(address owner, string name);
    error RepositoryAlreadyExists(bytes32 repoId);
    error Unauthorized(address caller);
    error InvalidRole(uint8 role);
    error InvalidCollaborator(address account);
    error RefNotFound(bytes32 repoId, bytes32 refId);
    error ShaMismatch(string expected, string actual);
    error InvalidPageSize(uint256 requested, uint256 maximum);
    error InvalidCursor(uint256 cursor, uint256 total);
    error InvalidTransferTarget(address target);
    error TransferAlreadyPending(bytes32 repoId);
    error TransferNotPending(bytes32 repoId);
    error TransferTooEarly(uint64 executeAfter);
    error TransferExpired(uint64 expiresAt);
    error TransferNotExpired(uint64 expiresAt);
    error RecoveryPending(bytes32 repoId);
    error TimestampOverflow(uint256 timestamp);
    error InvalidImportKind(uint8 kind);
    error InvalidImportRecord();

    event RepositoryCreated(bytes32 indexed repoId, address indexed owner, string name, bytes32 indexed forkedFrom);
    event RepositoryMetadataUpdated(
        bytes32 indexed repoId,
        address indexed owner,
        uint8 fieldMask,
        string description,
        string defaultBranch
    );
    event RefUpdated(
        bytes32 indexed repoId,
        string indexed refName,
        string commitSha,
        string[] packUris,
        address indexed updatedBy
    );
    event RefDeleted(bytes32 indexed repoId, string indexed refName, address indexed deletedBy);
    event CollaboratorUpdated(bytes32 indexed repoId, address indexed account, Role role, address indexed updatedBy);
    event OwnershipTransferStarted(
        bytes32 indexed repoId,
        address indexed oldOwner,
        address indexed newOwner,
        uint64 executeAfter,
        uint64 expiresAt
    );
    event OwnershipTransferCancelled(bytes32 indexed repoId, address indexed oldOwner, address indexed newOwner);
    event OwnershipTransferred(
        bytes32 indexed repoId,
        address indexed oldOwner,
        address indexed newOwner,
        string name,
        bool recovered
    );

    constructor(address directory, address coordinator) SuiteModule(directory, coordinator) {}

    function moduleId() public pure override returns (bytes32) {
        return SuiteIds.CORE;
    }

    function createRepository(string calldata name, string calldata description, string calldata defaultBranch)
        external
        onlyActiveSuite
        returns (bytes32 repoId)
    {
        _validateRepositoryName(name);
        _requireLength(description, MAX_DESCRIPTION_LENGTH);
        _requireLength(defaultBranch, MAX_NAME_LENGTH);
        string memory branch = bytes(defaultBranch).length == 0 ? "main" : defaultBranch;
        bytes32 locator = _locator(msg.sender, name);
        _requireLocatorAvailable(locator, msg.sender, name);
        repoId = keccak256(abi.encode("igit:suite:v3:repo", block.chainid, suiteDirectory, msg.sender, name));
        if (_repositories[repoId].exists) revert RepositoryAlreadyExists(repoId);
        uint64 now64 = _now64();
        _storeRepository(
            ImportRepository(repoId, msg.sender, name, description, branch, bytes32(0), now64, now64)
        );
        emit RepositoryCreated(repoId, msg.sender, name, bytes32(0));
    }

    function forkRepository(bytes32 sourceRepoId, string calldata newName)
        external
        onlyActiveSuite
        returns (bytes32 forkId)
    {
        Repository storage source = _requireRepository(sourceRepoId);
        IModerationPolicy(_moderation()).requireFork(sourceRepoId, msg.sender);
        uint256 refCount = _refNames[sourceRepoId].length;
        if (refCount > MAX_FORK_REFS) revert ForkTooLarge(refCount, MAX_FORK_REFS);
        _validateRepositoryName(newName);
        bytes32 locator = _locator(msg.sender, newName);
        _requireLocatorAvailable(locator, msg.sender, newName);
        forkId = keccak256(abi.encode("igit:suite:v3:repo", block.chainid, suiteDirectory, msg.sender, newName));
        if (_repositories[forkId].exists) revert RepositoryAlreadyExists(forkId);
        uint64 now64 = _now64();
        _storeRepository(
            ImportRepository(
                forkId,
                msg.sender,
                newName,
                source.description,
                source.defaultBranch,
                sourceRepoId,
                now64,
                now64
            )
        );
        for (uint256 i; i < refCount; ++i) {
            string memory refName = _refNames[sourceRepoId][i];
            GitRef storage original = _refs[sourceRepoId][keccak256(bytes(refName))];
            GitRef storage copied = _refs[forkId][keccak256(bytes(refName))];
            copied.commitSha = original.commitSha;
            copied.packUris = original.packUris;
            copied.updatedAt = now64;
            copied.updatedBy = msg.sender;
            copied.exists = true;
            _refNames[forkId].push(refName);
        }
        emit RepositoryCreated(forkId, msg.sender, newName, sourceRepoId);
    }

    function updateMetadata(
        bytes32 repoId,
        bool updateDescription,
        string calldata description,
        bool updateDefaultBranch,
        string calldata defaultBranch
    ) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        if (
            msg.sender != repository.owner
                && (updateDescription || updateDefaultBranch || msg.sender != _moderation())
        ) {
            revert Unauthorized(msg.sender);
        }
        uint8 fieldMask;
        if (updateDescription) {
            _requireLength(description, MAX_DESCRIPTION_LENGTH);
            repository.description = description;
            fieldMask |= 1;
        }
        if (updateDefaultBranch) {
            _requireLength(defaultBranch, MAX_NAME_LENGTH);
            repository.defaultBranch = defaultBranch;
            fieldMask |= 2;
        }
        repository.updatedAt = _now64();
        emit RepositoryMetadataUpdated(
            repoId, repository.owner, fieldMask, repository.description, repository.defaultBranch
        );
    }

    function updateRef(
        bytes32 repoId,
        string calldata refName,
        string calldata commitSha,
        string[] calldata packUris,
        string calldata expectedSha,
        bool force
    ) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        if (!_canMaintain(repoId, repository.owner, msg.sender)) revert Unauthorized(msg.sender);
        IModerationPolicy(_moderation()).requireRefMutation(repoId, msg.sender);
        _validateRefName(refName);
        _validateCommitSha(commitSha);
        if (packUris.length == 0 || packUris.length > MAX_PACK_URIS) {
            revert TooManyPackUris(packUris.length, MAX_PACK_URIS);
        }
        for (uint256 i; i < packUris.length; ++i) _validatePackUri(packUris[i]);

        bytes32 refId = keccak256(bytes(refName));
        GitRef storage target = _refs[repoId][refId];
        if (!force && target.exists && keccak256(bytes(target.commitSha)) != keccak256(bytes(expectedSha))) {
            revert ShaMismatch(expectedSha, target.commitSha);
        }
        if (!target.exists) {
            if (_refNames[repoId].length >= MAX_REFS_PER_REPO) {
                revert TooManyRefs(_refNames[repoId].length + 1, MAX_REFS_PER_REPO);
            }
            target.exists = true;
            _refNames[repoId].push(refName);
        }
        target.commitSha = commitSha;
        if (force || target.packUris.length == 0) {
            target.packUris = packUris;
        } else {
            for (uint256 i; i < packUris.length; ++i) {
                if (!_contains(target.packUris, packUris[i])) {
                    if (target.packUris.length >= MAX_PACK_URIS) {
                        revert TooManyPackUris(target.packUris.length + 1, MAX_PACK_URIS);
                    }
                    target.packUris.push(packUris[i]);
                }
            }
        }
        target.updatedAt = _now64();
        target.updatedBy = msg.sender;
        repository.updatedAt = target.updatedAt;
        emit RefUpdated(repoId, refName, commitSha, target.packUris, msg.sender);
    }

    function deleteRef(bytes32 repoId, string calldata refName) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        if (!_canMaintain(repoId, repository.owner, msg.sender)) revert Unauthorized(msg.sender);
        IModerationPolicy(_moderation()).requireRefMutation(repoId, msg.sender);
        bytes32 refId = keccak256(bytes(refName));
        if (!_refs[repoId][refId].exists) revert RefNotFound(repoId, refId);
        delete _refs[repoId][refId];
        string[] storage names = _refNames[repoId];
        for (uint256 i; i < names.length; ++i) {
            if (keccak256(bytes(names[i])) == refId) {
                names[i] = names[names.length - 1];
                names.pop();
                break;
            }
        }
        repository.updatedAt = _now64();
        emit RefDeleted(repoId, refName, msg.sender);
    }

    function setCollaborator(bytes32 repoId, address account, Role role) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        if (msg.sender != repository.owner) revert Unauthorized(msg.sender);
        if (account == address(0) || account == repository.owner) revert InvalidCollaborator(account);
        if (uint8(role) > uint8(Role.Reader)) revert InvalidRole(uint8(role));
        Role previous = _roles[repoId][account];
        if (role == Role.None) {
            if (previous != Role.None) _removeCollaborator(repoId, account);
        } else {
            if (previous == Role.None) {
                if (_collaborators[repoId].length >= MAX_COLLABORATORS_PER_REPO) {
                    revert TooManyCollaborators(_collaborators[repoId].length + 1, MAX_COLLABORATORS_PER_REPO);
                }
                _collaborators[repoId].push(account);
                _collaboratorIndexes[repoId][account] = _collaborators[repoId].length;
            }
            _roles[repoId][account] = role;
        }
        emit CollaboratorUpdated(repoId, account, role, msg.sender);
    }

    function beginOwnershipTransfer(bytes32 repoId, address newOwner) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        if (msg.sender != repository.owner) revert Unauthorized(msg.sender);
        if (newOwner == address(0) || newOwner == repository.owner) revert InvalidTransferTarget(newOwner);
        if (_pendingTransfers[repoId].newOwner != address(0)) revert TransferAlreadyPending(repoId);
        address recovery = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.RECOVERY);
        if (recovery == address(0)) revert SuiteNotActive();
        if (IRecoveryState(recovery).hasPendingRecovery(repoId)) revert RecoveryPending(repoId);
        bytes32 targetLocator = _locator(newOwner, repository.name);
        _requireLocatorAvailable(targetLocator, newOwner, repository.name);
        uint64 executeAfter = _addTime(_now64(), OWNERSHIP_TRANSFER_DELAY);
        uint64 expiresAt = _addTime(executeAfter, OWNERSHIP_TRANSFER_WINDOW);
        _pendingTransfers[repoId] = PendingOwnershipTransfer(newOwner, executeAfter, expiresAt);
        _reservedLocators[targetLocator] = repoId;
        emit OwnershipTransferStarted(repoId, repository.owner, newOwner, executeAfter, expiresAt);
    }

    function cancelOwnershipTransfer(bytes32 repoId) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner == address(0)) revert TransferNotPending(repoId);
        if (msg.sender != repository.owner && msg.sender != pending.newOwner) revert Unauthorized(msg.sender);
        _clearPending(repoId, repository.name, pending.newOwner);
        emit OwnershipTransferCancelled(repoId, repository.owner, pending.newOwner);
    }

    function acceptOwnershipTransfer(bytes32 repoId) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner == address(0)) revert TransferNotPending(repoId);
        if (msg.sender != pending.newOwner) revert Unauthorized(msg.sender);
        uint64 now64 = _now64();
        if (now64 < pending.executeAfter) revert TransferTooEarly(pending.executeAfter);
        if (now64 > pending.expiresAt) revert TransferExpired(pending.expiresAt);
        _transferOwnership(repoId, repository, pending.newOwner, false);
    }

    function expireOwnershipTransfer(bytes32 repoId) external onlyActiveSuite {
        Repository storage repository = _requireRepository(repoId);
        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner == address(0)) revert TransferNotPending(repoId);
        if (_now64() <= pending.expiresAt) revert TransferNotExpired(pending.expiresAt);
        _clearPending(repoId, repository.name, pending.newOwner);
        emit OwnershipTransferCancelled(repoId, repository.owner, pending.newOwner);
    }

    function recoverOwnership(bytes32 repoId, address newOwner) external onlyActiveSuite {
        address recovery = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.RECOVERY);
        if (msg.sender != recovery || recovery == address(0)) revert Unauthorized(msg.sender);
        Repository storage repository = _requireRepository(repoId);
        if (newOwner == address(0) || newOwner == repository.owner) revert InvalidTransferTarget(newOwner);
        _requireLocatorAvailableFor(repoId, _locator(newOwner, repository.name), newOwner, repository.name);
        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner != address(0)) revert TransferAlreadyPending(repoId);
        _transferOwnership(repoId, repository, newOwner, true);
    }

    function getRepository(bytes32 repoId) external view returns (Repository memory) {
        Repository memory repository = _repositories[repoId];
        if (!repository.exists) revert RepositoryNotFound(repoId);
        return repository;
    }

    function resolveRepository(address owner, string calldata name)
        external
        view
        returns (Repository memory repository, bool canonical)
    {
        bytes32 repoId = _locators[_locator(owner, name)];
        if (repoId == bytes32(0)) revert LocatorNotFound(owner, name);
        repository = _repositories[repoId];
        canonical = repository.owner == owner && keccak256(bytes(repository.name)) == keccak256(bytes(name));
    }

    function getRef(bytes32 repoId, string calldata refName) external view returns (GitRef memory) {
        _requireRepository(repoId);
        bytes32 refId = keccak256(bytes(refName));
        GitRef memory gitRef = _refs[repoId][refId];
        if (!gitRef.exists) revert RefNotFound(repoId, refId);
        return gitRef;
    }

    function listRepositoriesPage(address owner, uint256 cursor, uint256 limit)
        external
        view
        returns (Repository[] memory page, uint256 nextCursor)
    {
        bytes32[] storage ids = _ownerRepositories[owner];
        _requirePage(cursor, limit, ids.length);
        uint256 end = _pageEnd(cursor, limit, ids.length);
        page = new Repository[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) page[i - cursor] = _repositories[ids[i]];
        return (page, end);
    }

    function listRefsPage(bytes32 repoId, uint256 cursor, uint256 limit)
        external
        view
        returns (string[] memory names, GitRef[] memory refs, uint256 nextCursor)
    {
        _requireRepository(repoId);
        string[] storage source = _refNames[repoId];
        _requirePage(cursor, limit, source.length);
        uint256 end = _pageEnd(cursor, limit, source.length);
        names = new string[](end - cursor);
        refs = new GitRef[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) {
            names[i - cursor] = source[i];
            refs[i - cursor] = _refs[repoId][keccak256(bytes(source[i]))];
        }
        return (names, refs, end);
    }

    function listCollaboratorsPage(bytes32 repoId, uint256 cursor, uint256 limit)
        external
        view
        returns (address[] memory accounts, Role[] memory roles, uint256 nextCursor)
    {
        _requireRepository(repoId);
        address[] storage source = _collaborators[repoId];
        _requirePage(cursor, limit, source.length);
        uint256 end = _pageEnd(cursor, limit, source.length);
        accounts = new address[](end - cursor);
        roles = new Role[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) {
            accounts[i - cursor] = source[i];
            roles[i - cursor] = _roles[repoId][source[i]];
        }
        return (accounts, roles, end);
    }

    function collaboratorRole(bytes32 repoId, address account) external view returns (Role) {
        _requireRepository(repoId);
        return _roles[repoId][account];
    }

    function pendingOwnershipTransfer(bytes32 repoId) external view returns (PendingOwnershipTransfer memory) {
        _requireRepository(repoId);
        return _pendingTransfers[repoId];
    }

    function canMaintain(bytes32 repoId, address actor) external view returns (bool) {
        Repository storage repository = _requireRepository(repoId);
        return _canMaintain(repoId, repository.owner, actor);
    }

    function _importPayload(bytes calldata payload) internal override returns (uint256 count) {
        (uint8 kind, bytes memory records) = abi.decode(payload, (uint8, bytes));
        if (kind == 0) {
            ImportRepository[] memory repositories = abi.decode(records, (ImportRepository[]));
            count = repositories.length;
            for (uint256 i; i < count; ++i) _storeRepository(repositories[i]);
        } else if (kind == 1) {
            ImportAlias[] memory aliases = abi.decode(records, (ImportAlias[]));
            count = aliases.length;
            for (uint256 i; i < count; ++i) {
                _requireRepository(aliases[i].repoId);
                _validateRepositoryName(aliases[i].name);
                if (aliases[i].owner == address(0)) revert InvalidImportRecord();
                bytes32 locator = _locator(aliases[i].owner, aliases[i].name);
                if (_locators[locator] != bytes32(0) || _reservedLocators[locator] != bytes32(0)) {
                    revert InvalidImportRecord();
                }
                _locators[locator] = aliases[i].repoId;
            }
        } else if (kind == 2) {
            ImportRef[] memory refs = abi.decode(records, (ImportRef[]));
            count = refs.length;
            for (uint256 i; i < count; ++i) _storeImportedRef(refs[i]);
        } else if (kind == 3) {
            ImportCollaborator[] memory collaborators = abi.decode(records, (ImportCollaborator[]));
            count = collaborators.length;
            for (uint256 i; i < count; ++i) _storeImportedCollaborator(collaborators[i]);
        } else {
            revert InvalidImportKind(kind);
        }
    }

    function _storeRepository(ImportRepository memory imported) private {
        if (
            imported.id == bytes32(0) || imported.owner == address(0) || imported.createdAt == 0
                || imported.updatedAt < imported.createdAt
        ) revert InvalidImportRecord();
        if (imported.forkedFrom != bytes32(0) && !_repositories[imported.forkedFrom].exists) {
            revert InvalidImportRecord();
        }
        _validateRepositoryName(imported.name);
        _requireLength(imported.description, MAX_DESCRIPTION_LENGTH);
        _requireLength(imported.defaultBranch, MAX_NAME_LENGTH);
        if (_repositories[imported.id].exists) revert RepositoryAlreadyExists(imported.id);
        bytes32 locator = _locator(imported.owner, imported.name);
        _requireLocatorAvailable(locator, imported.owner, imported.name);
        _repositories[imported.id] = Repository({
            id: imported.id,
            owner: imported.owner,
            name: imported.name,
            description: imported.description,
            defaultBranch: imported.defaultBranch,
            forkedFrom: imported.forkedFrom,
            createdAt: imported.createdAt,
            updatedAt: imported.updatedAt,
            exists: true
        });
        _locators[locator] = imported.id;
        _ownerRepositoryIndexes[imported.id] = _ownerRepositories[imported.owner].length + 1;
        _ownerRepositories[imported.owner].push(imported.id);
    }

    function _storeImportedRef(ImportRef memory imported) private {
        _requireRepository(imported.repoId);
        _validateRefName(imported.refName);
        _validateCommitSha(imported.commitSha);
        if (
            imported.packUris.length == 0 || imported.packUris.length > MAX_PACK_URIS || imported.updatedAt == 0
            || imported.updatedBy == address(0)
        ) {
            revert InvalidImportRecord();
        }
        bytes32 refId = keccak256(bytes(imported.refName));
        if (_refs[imported.repoId][refId].exists) revert InvalidImportRecord();
        for (uint256 i; i < imported.packUris.length; ++i) _validatePackUri(imported.packUris[i]);
        _refs[imported.repoId][refId] = GitRef(
            imported.commitSha, imported.packUris, imported.updatedAt, imported.updatedBy, true
        );
        _refNames[imported.repoId].push(imported.refName);
        if (_refNames[imported.repoId].length > MAX_REFS_PER_REPO) {
            revert TooManyRefs(_refNames[imported.repoId].length, MAX_REFS_PER_REPO);
        }
    }

    function _storeImportedCollaborator(ImportCollaborator memory imported) private {
        Repository storage repository = _requireRepository(imported.repoId);
        if (
            imported.account == address(0) || imported.account == repository.owner || imported.role == Role.None
        ) revert InvalidImportRecord();
        if (_roles[imported.repoId][imported.account] != Role.None) revert InvalidImportRecord();
        _roles[imported.repoId][imported.account] = imported.role;
        _collaborators[imported.repoId].push(imported.account);
        _collaboratorIndexes[imported.repoId][imported.account] = _collaborators[imported.repoId].length;
        if (_collaborators[imported.repoId].length > MAX_COLLABORATORS_PER_REPO) {
            revert TooManyCollaborators(
                _collaborators[imported.repoId].length, MAX_COLLABORATORS_PER_REPO
            );
        }
    }

    function _transferOwnership(
        bytes32 repoId,
        Repository storage repository,
        address newOwner,
        bool recovered
    ) private {
        address oldOwner = repository.owner;
        bytes32 oldLocator = _locator(oldOwner, repository.name);
        bytes32 newLocator = _locator(newOwner, repository.name);
        _requireLocatorAvailableFor(repoId, newLocator, newOwner, repository.name);
        _moveOwnerIndex(repoId, oldOwner, newOwner);
        _locators[oldLocator] = repoId;
        _locators[newLocator] = repoId;
        repository.owner = newOwner;
        delete _reservedLocators[newLocator];
        delete _pendingTransfers[repoId];
        address economic = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.ECONOMIC);
        if (economic == address(0)) revert SuiteNotActive();
        IEconomicOwnershipHook(economic).clearRevenueSplitsOnOwnershipTransfer(repoId);
        emit OwnershipTransferred(repoId, oldOwner, newOwner, repository.name, recovered);
    }

    function _moveOwnerIndex(bytes32 repoId, address oldOwner, address newOwner) private {
        uint256 indexPlusOne = _ownerRepositoryIndexes[repoId];
        if (indexPlusOne == 0) revert InvalidImportRecord();
        bytes32[] storage oldList = _ownerRepositories[oldOwner];
        uint256 index = indexPlusOne - 1;
        bytes32 moved = oldList[oldList.length - 1];
        oldList[index] = moved;
        _ownerRepositoryIndexes[moved] = index + 1;
        oldList.pop();
        _ownerRepositoryIndexes[repoId] = _ownerRepositories[newOwner].length + 1;
        _ownerRepositories[newOwner].push(repoId);
    }

    function _clearPending(bytes32 repoId, string memory name, address newOwner) private {
        delete _reservedLocators[_locator(newOwner, name)];
        delete _pendingTransfers[repoId];
    }

    function _removeCollaborator(bytes32 repoId, address account) private {
        uint256 indexPlusOne = _collaboratorIndexes[repoId][account];
        if (indexPlusOne != 0) {
            address[] storage list = _collaborators[repoId];
            uint256 index = indexPlusOne - 1;
            address moved = list[list.length - 1];
            list[index] = moved;
            _collaboratorIndexes[repoId][moved] = index + 1;
            list.pop();
        }
        delete _collaboratorIndexes[repoId][account];
        delete _roles[repoId][account];
    }

    function _requireRepository(bytes32 repoId) private view returns (Repository storage repository) {
        repository = _repositories[repoId];
        if (!repository.exists) revert RepositoryNotFound(repoId);
    }

    function _moderation() private view returns (address module) {
        module = ISuiteDirectory(suiteDirectory).moduleAddress(SuiteIds.MODERATION);
        if (module == address(0)) revert SuiteNotActive();
    }

    function _canMaintain(bytes32 repoId, address owner, address actor) private view returns (bool) {
        return actor == owner || _roles[repoId][actor] == Role.Maintainer;
    }

    function _locator(address owner, string memory name) private pure returns (bytes32) {
        return keccak256(abi.encode(owner, name));
    }

    function _requireLocatorAvailable(bytes32 locator, address owner, string memory name) private view {
        if (_locators[locator] != bytes32(0) || _reservedLocators[locator] != bytes32(0)) {
            revert LocatorUnavailable(owner, name);
        }
    }

    function _requireLocatorAvailableFor(
        bytes32 repoId,
        bytes32 locator,
        address owner,
        string memory name
    ) private view {
        bytes32 occupied = _locators[locator];
        bytes32 reserved = _reservedLocators[locator];
        if ((occupied != bytes32(0) && occupied != repoId) || (reserved != bytes32(0) && reserved != repoId)) {
            revert LocatorUnavailable(owner, name);
        }
    }

    function _validateRepositoryName(string memory value) private pure {
        bytes memory raw = bytes(value);
        if (raw.length == 0 || raw.length > MAX_NAME_LENGTH) revert InvalidRepositoryName(value);
        for (uint256 i; i < raw.length; ++i) {
            bytes1 c = raw[i];
            if (
                !((c >= "a" && c <= "z") || (c >= "A" && c <= "Z") || (c >= "0" && c <= "9") || c == "-"
                    || c == "_" || c == ".")
            ) {
                revert InvalidRepositoryName(value);
            }
        }
    }

    function _validateRefName(string memory value) private pure {
        bytes memory raw = bytes(value);
        if (raw.length < 5 || raw.length > MAX_REF_NAME_LENGTH) revert InvalidRefName(value);
        uint256 prefix;
        assembly {
            prefix := shr(216, mload(add(raw, 32)))
        }
        if (prefix != 0x726566732f) {
            revert InvalidRefName(value);
        }
        for (uint256 i = 5; i < raw.length; ++i) {
            bytes1 c = raw[i];
            if (c < 0x21 || c > 0x7e || c == "~" || c == "^" || c == ":" || c == "\\") {
                revert InvalidRefName(value);
            }
            if (c == "." && raw[i - 1] == ".") revert InvalidRefName(value);
        }
    }

    function _validateCommitSha(string memory value) private pure {
        bytes memory raw = bytes(value);
        if (raw.length != 40 && raw.length != 64) revert InvalidCommitSha(value);
        for (uint256 i; i < raw.length; ++i) {
            bytes1 c = raw[i];
            if (
                !((c >= "0" && c <= "9") || (c >= "a" && c <= "f") || (c >= "A" && c <= "F"))
            ) revert InvalidCommitSha(value);
        }
    }

    function _validatePackUri(string memory value) private pure {
        bytes memory raw = bytes(value);
        if (raw.length <= 7 || raw.length > MAX_PACK_URI_LENGTH) revert InvalidPackUri(value);
        uint256 prefix;
        assembly {
            prefix := shr(200, mload(add(raw, 32)))
        }
        if (prefix != 0x697066733a2f2f) {
            revert InvalidPackUri(value);
        }
    }

    function _contains(string[] storage values, string calldata candidate) private view returns (bool) {
        bytes32 digest = keccak256(bytes(candidate));
        for (uint256 i; i < values.length; ++i) {
            if (keccak256(bytes(values[i])) == digest) return true;
        }
        return false;
    }

    function _requireLength(string memory value, uint256 maximum) private pure {
        if (bytes(value).length > maximum) revert InputTooLong(bytes(value).length, maximum);
    }

    function _requirePage(uint256 cursor, uint256 limit, uint256 total) private pure {
        if (limit == 0 || limit > MAX_QUERY_PAGE_SIZE) revert InvalidPageSize(limit, MAX_QUERY_PAGE_SIZE);
        if (cursor > total) revert InvalidCursor(cursor, total);
    }

    function _pageEnd(uint256 cursor, uint256 limit, uint256 total) private pure returns (uint256) {
        uint256 end = cursor + limit;
        return end > total ? total : end;
    }

    function _now64() private view returns (uint64) {
        if (block.timestamp > type(uint64).max) revert TimestampOverflow(block.timestamp);
        return uint64(block.timestamp);
    }

    function _addTime(uint64 timestamp, uint64 delay) private pure returns (uint64) {
        uint256 result = uint256(timestamp) + delay;
        if (result > type(uint64).max) revert TimestampOverflow(result);
        return uint64(result);
    }
}
