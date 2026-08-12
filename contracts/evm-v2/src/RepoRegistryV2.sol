// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.24;

/// @title Minimal EVM V2 repository registry
/// @notice Stores repository metadata and git refs, with owner-only writes.
contract RepoRegistryV2 {
    string private constant REPO_ID_DOMAIN = "igit:v2:repo";
    string private constant IMPORT_DOMAIN = "igit:v2:import";
    uint8 private constant STATUS_ACTIVE = 0;
    uint8 private constant STATUS_FROZEN = 1;
    uint8 private constant STATUS_DELISTED = 2;

    uint256 public constant MAX_NAME_LENGTH = 64;
    uint256 public constant MAX_DESCRIPTION_LENGTH = 1024;
    uint256 public constant MAX_REF_NAME_LENGTH = 256;
    uint256 public constant MAX_COMMIT_SHA_LENGTH = 64;
    uint256 public constant MAX_PACK_URIS = 128;
    uint256 public constant MAX_PACK_URI_LENGTH = 512;
    uint256 public constant MAX_REFS_PER_REPO = 1024;
    uint256 public constant MAX_COLLABORATORS_PER_REPO = 256;
    uint256 public constant MAX_QUERY_PAGE_SIZE = 64;
    uint64 public constant OWNERSHIP_TRANSFER_DELAY = 7 days;
    uint64 public constant OWNERSHIP_TRANSFER_ACCEPTANCE_WINDOW = 30 days;

    address public immutable admin;
    uint256 public immutable identityChainId;

    enum Role {
        None,
        Maintainer,
        Reader
    }

    struct Repo {
        address owner;
        string name;
        string description;
        string defaultBranch;
        uint64 createdAt;
        uint64 updatedAt;
        uint8 moderationStatus;
        bool exists;
    }

    struct Ref {
        string commitSha;
        string[] packUris;
        uint64 updatedAt;
        address updatedBy;
        bool exists;
    }

    struct PendingOwnershipTransfer {
        address newOwner;
        uint64 proposedAt;
        uint64 executeAfter;
        uint64 expiresAt;
    }

    /// @notice A migration session is bound to one verified V1 snapshot and
    /// one source contract. Only one session can be active, which makes a
    /// partially applied snapshot resumable without exposing it as live V2
    /// state or allowing concurrent user writes.
    struct ImportSession {
        bytes32 snapshotHash;
        string sourceChainId;
        address sourceContract;
        uint64 sourceHeight;
        uint256 expectedRepositories;
        uint256 remainingRefs;
        uint256 remainingCollaborators;
        uint256 expectedBatches;
        uint256 importedRepositories;
        uint256 importedRefs;
        uint256 importedCollaborators;
        uint256 nextSequence;
        uint256 incompleteRepositories;
        bool active;
        bool finalized;
    }

    struct ImportRepoState {
        bytes32 sessionId;
        uint256 expectedRefs;
        uint256 expectedCollaborators;
        uint256 importedRefs;
        uint256 importedCollaborators;
        bool complete;
    }

    struct ImportRef {
        string refName;
        string commitSha;
        string[] packUris;
        uint64 updatedAt;
        address updatedBy;
    }

    struct ImportCollaborator {
        address account;
        Role role;
    }

    enum ReservationKind {
        None,
        OwnershipTransfer,
        Fork,
        Import
    }

    struct LocatorReservation {
        bytes32 subjectId;
        ReservationKind kind;
    }

    mapping(bytes32 repoId => Repo repo) private _repos;
    mapping(address owner => bytes32[] repoIds) private _ownerRepoIds;
    mapping(bytes32 repoId => uint256 indexPlusOne) private _ownerRepoIndexes;
    mapping(bytes32 locatorKey => bytes32 repoId) private _activeLocators;
    mapping(bytes32 locatorKey => bytes32 repoId) private _aliases;
    mapping(bytes32 locatorKey => LocatorReservation reservation) private _locatorReservations;
    mapping(bytes32 repoId => PendingOwnershipTransfer transfer) private _pendingTransfers;
    mapping(bytes32 repoId => mapping(bytes32 refId => Ref ref)) private _refs;
    mapping(bytes32 repoId => string[] names) private _refNames;
    mapping(bytes32 repoId => mapping(address collaborator => Role role)) private _roles;
    mapping(bytes32 repoId => address[] collaborators) private _collaborators;
    mapping(bytes32 repoId => mapping(address collaborator => uint256 indexPlusOne)) private _collaboratorIndexes;
    mapping(bytes32 sessionId => ImportSession session) private _importSessions;
    mapping(bytes32 repoId => ImportRepoState state) private _importRepoStates;
    bytes32 public activeImportSession;
    bool private _importWindowClosed;

    constructor(address initialAdmin) {
        if (initialAdmin == address(0)) revert InvalidAdmin();
        admin = initialAdmin;
        identityChainId = block.chainid;
    }

    error InvalidAdmin();
    error EmptyInput();
    error EmptyPackUris();
    error InvalidRepoName(string name);
    error InvalidRefName(string refName);
    error InputTooLong(uint256 length, uint256 maximum);
    error TooManyPackUris(uint256 count, uint256 maximum);
    error TooManyRefs(uint256 count, uint256 maximum);
    error RepoAlreadyExists(bytes32 repoId);
    error RepoNotFound(bytes32 repoId);
    error LocatorNotFound(address owner, string name);
    error LocatorUnavailable(address owner, string name);
    error RepoMoved(bytes32 repoId, address currentOwner, string name);
    error RefNotFound(bytes32 repoId, bytes32 refId);
    error Unauthorized(address caller);
    error ShaMismatch(string expected, string actual);
    error InvalidCommitSha(string commitSha);
    error InvalidPackUri(string uri);
    error InvalidCollaborator(address collaborator);
    error TooManyCollaborators(uint256 count, uint256 maximum);
    error InvalidPageSize(uint256 requested, uint256 maximum);
    error InvalidCursor(uint256 cursor, uint256 total);
    error FrozenRepo(bytes32 repoId);
    error InvalidRole(uint8 role);
    error InvalidTransferTarget(address newOwner);
    error TransferAlreadyPending(bytes32 repoId);
    error TransferNotPending(bytes32 repoId);
    error TransferTooEarly(bytes32 repoId, uint64 executeAfter);
    error TransferExpired(bytes32 repoId, uint64 expiresAt);
    error TransferNotExpired(bytes32 repoId, uint64 expiresAt);
    error TransferUnauthorized(bytes32 repoId, address caller);
    error LocatorReservationMismatch(bytes32 locatorKey, bytes32 expectedSubject);
    error RepoIndexCorrupted(bytes32 repoId, address owner);
    error TimestampOverflow(uint256 timestamp);
    error ImportAlreadyActive(bytes32 sessionId);
    error ImportStateLocked(bytes32 sessionId);
    error ImportSessionAlreadyExists(bytes32 sessionId);
    error ImportSessionNotActive(bytes32 sessionId);
    error ImportSequenceMismatch(uint256 expected, uint256 actual);
    error ImportRepoIdMismatch(bytes32 expected, bytes32 actual);
    error ImportRepoSessionMismatch(bytes32 repoId, bytes32 expectedSession, bytes32 actualSession);
    error ImportCountExceeded(bytes32 repoId, uint256 expected, uint256 actual);
    error ImportCountMismatch(bytes32 field, uint256 expected, uint256 actual);
    error ImportRepositoriesIncomplete(uint256 count);
    error EmptyImportBatch();
    error InvalidImportSource();
    error InvalidImportOwner(address owner);
    error InvalidImportCounts();
    error InvalidImportPayloadHash();
    error ImportPayloadMismatch(bytes32 expected, bytes32 actual);
    error InvalidImportStatus(uint8 status);
    error ImportWindowClosed();

    event RepoCreated(
        bytes32 indexed repoId,
        address indexed owner,
        string name,
        string description,
        string defaultBranch
    );
    event RepoInfoUpdated(
        bytes32 indexed repoId,
        address indexed owner,
        string repo,
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
    event CollaboratorUpdated(bytes32 indexed repoId, address indexed collaborator, Role role, address indexed updatedBy);
    event RepoFrozen(bytes32 indexed repoId, bool frozen, address indexed updatedBy);
    event OwnershipTransferStarted(
        bytes32 indexed repoId,
        address indexed oldOwner,
        address indexed newOwner,
        string name,
        uint64 proposedAt,
        uint64 executeAfter,
        uint64 expiresAt
    );
    event OwnershipTransferCancelled(bytes32 indexed repoId, address indexed owner, address target);
    event OwnershipTransferRejected(bytes32 indexed repoId, address indexed owner, address indexed target);
    event OwnershipTransferExpired(bytes32 indexed repoId, address indexed owner, address indexed target);
    event OwnershipTransferred(
        bytes32 indexed repoId,
        address indexed oldOwner,
        address indexed newOwner,
        address acceptedBy,
        string name,
        uint64 executeAfter
    );
    event ImportSessionCreated(
        bytes32 indexed sessionId,
        bytes32 indexed snapshotHash,
        string sourceChainId,
        address indexed sourceContract,
        uint64 sourceHeight,
        uint256 expectedRepositories,
        uint256 expectedRefs,
        uint256 expectedCollaborators,
        uint256 expectedBatches
    );
    event ImportRepoApplied(
        bytes32 indexed sessionId,
        uint256 indexed sequence,
        bytes32 indexed repoId,
        bytes32 payloadHash,
        address owner,
        string name,
        uint8 moderationStatus,
        uint256 expectedRefs,
        uint256 expectedCollaborators
    );
    event ImportRefsApplied(
        bytes32 indexed sessionId,
        uint256 indexed sequence,
        bytes32 indexed repoId,
        bytes32 payloadHash,
        uint256 count
    );
    event ImportCollaboratorsApplied(
        bytes32 indexed sessionId,
        uint256 indexed sequence,
        bytes32 indexed repoId,
        bytes32 payloadHash,
        uint256 count
    );
    event ImportFinalized(
        bytes32 indexed sessionId,
        bytes32 indexed snapshotHash,
        uint256 repositories,
        uint256 refs,
        uint256 collaborators,
        uint256 batches
    );

    /// @notice Start a resumable, hash-bound V1 snapshot import. The snapshot
    /// SHA-256 digest is also the session ID, so the same snapshot can never be
    /// replayed as a second session on this deployment.
    function createImportSession(
        bytes32 snapshotHash,
        string calldata sourceChainId,
        address sourceContract,
        uint64 sourceHeight,
        uint256 expectedRepositories,
        uint256 expectedRefs,
        uint256 expectedCollaborators,
        uint256 expectedBatches
    ) external returns (bytes32 sessionId) {
        _requireAdmin();
        if (_importWindowClosed) revert ImportWindowClosed();
        if (activeImportSession != bytes32(0)) {
            revert ImportAlreadyActive(activeImportSession);
        }
        if (
            snapshotHash == bytes32(0) ||
            bytes(sourceChainId).length == 0 ||
            sourceContract == address(0)
        ) revert InvalidImportSource();
        if (
            expectedBatches < expectedRepositories ||
            (expectedRepositories == 0 && (expectedRefs != 0 || expectedCollaborators != 0))
        ) revert InvalidImportCounts();

        sessionId = snapshotHash;
        if (_importSessions[sessionId].snapshotHash != bytes32(0)) {
            revert ImportSessionAlreadyExists(sessionId);
        }

        _importSessions[sessionId] = ImportSession({
            snapshotHash: snapshotHash,
            sourceChainId: sourceChainId,
            sourceContract: sourceContract,
            sourceHeight: sourceHeight,
            expectedRepositories: expectedRepositories,
            remainingRefs: expectedRefs,
            remainingCollaborators: expectedCollaborators,
            expectedBatches: expectedBatches,
            importedRepositories: 0,
            importedRefs: 0,
            importedCollaborators: 0,
            nextSequence: 0,
            incompleteRepositories: 0,
            active: true,
            finalized: false
        });
        activeImportSession = sessionId;

        emit ImportSessionCreated(
            sessionId,
            snapshotHash,
            sourceChainId,
            sourceContract,
            sourceHeight,
            expectedRepositories,
            expectedRefs,
            expectedCollaborators,
            expectedBatches
        );
    }

    /// @notice Return fixed-size progress fields so an interrupted executor
    /// can resume from `nextSequence` without decoding the dynamic source ID.
    function importProgress(
        bytes32 sessionId
    )
        external
        view
        returns (
            bool exists,
            bool active,
            bool finalized,
            uint256 nextSequence,
            uint256 expectedBatches,
            uint256 importedRepositories,
            uint256 expectedRepositories,
            uint256 importedRefs,
            uint256 remainingRefs,
            uint256 importedCollaborators,
            uint256 remainingCollaborators,
            uint256 incompleteRepositories
        )
    {
        ImportSession storage session = _importSessions[sessionId];
        exists = session.snapshotHash != bytes32(0);
        active = exists && activeImportSession == sessionId;
        finalized = exists && _importWindowClosed && !active;
        nextSequence = session.nextSequence;
        expectedBatches = session.expectedBatches;
        importedRepositories = session.importedRepositories;
        expectedRepositories = session.expectedRepositories;
        importedRefs = session.importedRefs;
        remainingRefs = session.remainingRefs;
        importedCollaborators = session.importedCollaborators;
        remainingCollaborators = session.remainingCollaborators;
        incompleteRepositories = session.incompleteRepositories;
    }

    /// @notice Whether this deployment can still accept the one-shot V1
    /// snapshot import. A controller uses this to reject registries that have
    /// already published a native repository or finalized an import session.
    function importWindowClosed() external view returns (bool) {
        return _importWindowClosed;
    }

    /// @notice Import one repository header and reserve its canonical locator.
    /// Historical description and default-branch bytes are preserved without
    /// applying the smaller limits used for new interactive V2 writes.
    function importRepo(
        bytes32 sessionId,
        uint256 sequence,
        bytes32 repoId,
        address owner,
        string calldata name,
        string calldata description,
        string calldata defaultBranch,
        uint8 moderationStatus,
        uint64 createdAt,
        uint64 updatedAt,
        uint256 expectedRefs,
        uint256 expectedCollaborators,
        bytes32 payloadHash
    ) external {
        _requireAdmin();
        ImportSession storage session = _requireActiveImport(sessionId);
        _validateImportBatch(session, sequence, payloadHash);
        if (owner == address(0)) revert InvalidImportOwner(owner);
        _validateRepoName(name);
        if (expectedRefs > MAX_REFS_PER_REPO) {
            revert TooManyRefs(expectedRefs, MAX_REFS_PER_REPO);
        }
        if (expectedCollaborators > MAX_COLLABORATORS_PER_REPO) {
            revert TooManyCollaborators(expectedCollaborators, MAX_COLLABORATORS_PER_REPO);
        }
        if (expectedRefs > session.remainingRefs) {
            revert ImportCountMismatch(bytes32("refs"), session.remainingRefs, expectedRefs);
        }
        if (expectedCollaborators > session.remainingCollaborators) {
            revert ImportCountMismatch(
                bytes32("collaborators"),
                session.remainingCollaborators,
                expectedCollaborators
            );
        }
        _validateImportStatus(moderationStatus);

        bytes32 derived = _importedRepoId(
            session.sourceChainId,
            session.sourceContract,
            owner,
            name
        );
        if (derived != repoId) revert ImportRepoIdMismatch(derived, repoId);
        bytes32 expectedPayloadHash = sha256(
            abi.encode(
                repoId,
                owner,
                name,
                description,
                defaultBranch,
                moderationStatus,
                createdAt,
                updatedAt,
                expectedRefs,
                expectedCollaborators
            )
        );
        if (expectedPayloadHash != payloadHash) {
            revert ImportPayloadMismatch(expectedPayloadHash, payloadHash);
        }
        if (_repos[repoId].exists) revert RepoAlreadyExists(repoId);
        if (session.importedRepositories >= session.expectedRepositories) {
            revert ImportCountMismatch(
                bytes32("repositories"),
                session.expectedRepositories,
                session.importedRepositories + 1
            );
        }

        bytes32 locatorKey = _locatorKey(owner, name);
        _requireLocatorAvailable(locatorKey, owner, name);
        _repos[repoId] = Repo({
            owner: owner,
            name: name,
            description: description,
            defaultBranch: defaultBranch,
            createdAt: createdAt,
            updatedAt: updatedAt,
            moderationStatus: moderationStatus,
            exists: true
        });
        _activeLocators[locatorKey] = repoId;
        _addOwnerRepo(owner, repoId);

        _importRepoStates[repoId] = ImportRepoState({
            sessionId: sessionId,
            expectedRefs: expectedRefs,
            expectedCollaborators: expectedCollaborators,
            importedRefs: 0,
            importedCollaborators: 0,
            complete: expectedRefs == 0 && expectedCollaborators == 0
        });
        session.importedRepositories += 1;
        // The session fields are remaining declaration budgets after creation.
        // Every imported repo consumes its exact per-repo counts before any
        // item batches can be accepted.
        session.remainingRefs -= expectedRefs;
        session.remainingCollaborators -= expectedCollaborators;
        if (expectedRefs != 0 || expectedCollaborators != 0) {
            session.incompleteRepositories += 1;
        }
        _completeImportBatch(session);

        emit ImportRepoApplied(
            sessionId,
            sequence,
            repoId,
            payloadHash,
            owner,
            name,
            moderationStatus,
            expectedRefs,
            expectedCollaborators
        );
    }

    /// @notice Append a non-empty, ordered ref batch for an imported repo.
    /// A reverted transaction does not consume its sequence number and can be
    /// retried safely with the same manifest entry.
    function importRefs(
        bytes32 sessionId,
        uint256 sequence,
        bytes32 repoId,
        ImportRef[] calldata refs,
        bytes32 payloadHash
    ) external {
        _requireAdmin();
        ImportSession storage session = _requireActiveImport(sessionId);
        _validateImportBatch(session, sequence, payloadHash);
        if (refs.length == 0) revert EmptyImportBatch();
        ImportRepoState storage state = _requireImportRepo(sessionId, repoId);
        bytes32 expectedPayloadHash = sha256(abi.encode(refs));
        if (expectedPayloadHash != payloadHash) {
            revert ImportPayloadMismatch(expectedPayloadHash, payloadHash);
        }
        uint256 nextCount = state.importedRefs + refs.length;
        if (nextCount > state.expectedRefs) {
            revert ImportCountExceeded(repoId, state.expectedRefs, nextCount);
        }

        for (uint256 i; i < refs.length; ++i) {
            ImportRef calldata imported = refs[i];
            _validateRefName(imported.refName);
            _validateCommitSha(imported.commitSha);
            if (imported.packUris.length == 0) revert EmptyPackUris();
            if (imported.packUris.length > MAX_PACK_URIS) {
                revert TooManyPackUris(imported.packUris.length, MAX_PACK_URIS);
            }
            if (imported.updatedBy == address(0)) {
                revert InvalidImportOwner(imported.updatedBy);
            }

            bytes32 refId = keccak256(bytes(imported.refName));
            Ref storage storedRef = _refs[repoId][refId];
            if (storedRef.exists) revert RepoAlreadyExists(refId);
            storedRef.commitSha = imported.commitSha;
            storedRef.updatedAt = imported.updatedAt;
            storedRef.updatedBy = imported.updatedBy;
            storedRef.exists = true;
            for (uint256 j; j < imported.packUris.length; ++j) {
                _validatePackUri(imported.packUris[j]);
                storedRef.packUris.push(imported.packUris[j]);
            }
            _refNames[repoId].push(imported.refName);
        }

        state.importedRefs = nextCount;
        session.importedRefs += refs.length;
        _markImportRepoComplete(session, state);
        _completeImportBatch(session);
        emit ImportRefsApplied(sessionId, sequence, repoId, payloadHash, refs.length);
    }

    /// @notice Append a non-empty, ordered collaborator batch for an imported
    /// repo. Imported owner addresses may not also be collaborators.
    function importCollaborators(
        bytes32 sessionId,
        uint256 sequence,
        bytes32 repoId,
        ImportCollaborator[] calldata collaborators,
        bytes32 payloadHash
    ) external {
        _requireAdmin();
        ImportSession storage session = _requireActiveImport(sessionId);
        _validateImportBatch(session, sequence, payloadHash);
        if (collaborators.length == 0) revert EmptyImportBatch();
        ImportRepoState storage state = _requireImportRepo(sessionId, repoId);
        bytes32 expectedPayloadHash = sha256(abi.encode(collaborators));
        if (expectedPayloadHash != payloadHash) {
            revert ImportPayloadMismatch(expectedPayloadHash, payloadHash);
        }
        uint256 nextCount = state.importedCollaborators + collaborators.length;
        if (nextCount > state.expectedCollaborators) {
            revert ImportCountExceeded(repoId, state.expectedCollaborators, nextCount);
        }

        address owner = _repos[repoId].owner;
        for (uint256 i; i < collaborators.length; ++i) {
            ImportCollaborator calldata imported = collaborators[i];
            if (imported.account == address(0) || imported.account == owner) {
                revert InvalidCollaborator(imported.account);
            }
            if (imported.role == Role.None || imported.role > Role.Reader) {
                revert InvalidRole(uint8(imported.role));
            }
            if (_roles[repoId][imported.account] != Role.None) {
                revert InvalidCollaborator(imported.account);
            }
            _collaborators[repoId].push(imported.account);
            _collaboratorIndexes[repoId][imported.account] = _collaborators[repoId].length;
            _roles[repoId][imported.account] = imported.role;
        }

        state.importedCollaborators = nextCount;
        session.importedCollaborators += collaborators.length;
        _markImportRepoComplete(session, state);
        _completeImportBatch(session);
        emit ImportCollaboratorsApplied(
            sessionId,
            sequence,
            repoId,
            payloadHash,
            collaborators.length
        );
    }

    /// @notice Make the imported snapshot visible only after every declared
    /// batch and count has been applied. There is intentionally no cancel path
    /// that could silently expose or strand a partially imported repository.
    function finalizeImport(bytes32 sessionId) external {
        _requireAdmin();
        ImportSession storage session = _requireActiveImport(sessionId);
        if (session.nextSequence != session.expectedBatches) {
            revert ImportCountMismatch(
                bytes32("batches"),
                session.expectedBatches,
                session.nextSequence
            );
        }
        if (session.importedRepositories != session.expectedRepositories) {
            revert ImportCountMismatch(
                bytes32("repositories"),
                session.expectedRepositories,
                session.importedRepositories
            );
        }
        if (session.remainingRefs != 0) {
            revert ImportCountMismatch(bytes32("refs"), 0, session.remainingRefs);
        }
        if (session.remainingCollaborators != 0) {
            revert ImportCountMismatch(
                bytes32("collaborators"),
                0,
                session.remainingCollaborators
            );
        }
        if (session.incompleteRepositories != 0) {
            revert ImportRepositoriesIncomplete(session.incompleteRepositories);
        }

        session.active = false;
        session.finalized = true;
        activeImportSession = bytes32(0);
        _importWindowClosed = true;
        emit ImportFinalized(
            sessionId,
            session.snapshotHash,
            session.importedRepositories,
            session.importedRefs,
            session.importedCollaborators,
            session.nextSequence
        );
    }

    /// @notice Register a repository in the caller's namespace.
    function createRepo(
        string calldata name,
        string calldata description,
        string calldata defaultBranch
    ) external returns (bytes32 repoId) {
        _requireImportIdle();
        _validateRepoName(name);
        // V1 treats description as optional and defaults an empty branch to
        // "main". Keep that behavior so `igit init repo` remains valid.
        if (bytes(description).length > MAX_DESCRIPTION_LENGTH) {
            revert InputTooLong(bytes(description).length, MAX_DESCRIPTION_LENGTH);
        }
        string memory normalizedBranch = defaultBranch;
        if (bytes(normalizedBranch).length == 0) normalizedBranch = "main";
        _requireLength(normalizedBranch, MAX_NAME_LENGTH);

        bytes32 locatorKey = _locatorKey(msg.sender, name);
        _requireLocatorAvailable(locatorKey, msg.sender, name);

        repoId = _nativeRepoId(msg.sender, name);
        if (_repos[repoId].exists) revert RepoAlreadyExists(repoId);

        uint64 nowTs = uint64(block.timestamp);
        _repos[repoId] = Repo({
            owner: msg.sender,
            name: name,
            description: description,
            defaultBranch: normalizedBranch,
            createdAt: nowTs,
            updatedAt: nowTs,
            moderationStatus: STATUS_ACTIVE,
            exists: true
        });
        _activeLocators[locatorKey] = repoId;
        _addOwnerRepo(msg.sender, repoId);
        _importWindowClosed = true;
        emit RepoCreated(repoId, msg.sender, name, description, normalizedBranch);
    }

    /// @notice Patch metadata in the caller's repository namespace. The
    /// explicit flags preserve V1's distinction between an omitted field and
    /// deliberately setting that field to an empty string.
    function updateRepoInfo(
        string calldata repo,
        bool updateDescription,
        string calldata description,
        bool updateDefaultBranch,
        string calldata defaultBranch
    ) external {
        bytes32 repoId = _resolveWritableLocator(msg.sender, repo);
        Repo storage repository = _repos[repoId];
        if (repository.owner != msg.sender) revert Unauthorized(msg.sender);

        if (updateDescription && bytes(description).length > MAX_DESCRIPTION_LENGTH) {
            revert InputTooLong(bytes(description).length, MAX_DESCRIPTION_LENGTH);
        }
        if (updateDefaultBranch && bytes(defaultBranch).length > MAX_NAME_LENGTH) {
            revert InputTooLong(bytes(defaultBranch).length, MAX_NAME_LENGTH);
        }

        uint8 fieldMask;
        if (updateDescription) {
            repository.description = description;
            fieldMask |= 1;
        }
        if (updateDefaultBranch) {
            repository.defaultBranch = defaultBranch;
            fieldMask |= 2;
        }
        repository.updatedAt = uint64(block.timestamp);

        emit RepoInfoUpdated(
            repoId,
            msg.sender,
            repo,
            fieldMask,
            repository.description,
            repository.defaultBranch
        );
    }

    /// @notice Update or create a ref. `expectedSha` is ignored when `force`
    /// is true. Existing refs require an exact expected SHA for non-force
    /// updates, matching the V1 optimistic-concurrency behavior.
    function updateRef(
        address owner,
        string calldata repo,
        string calldata refName,
        string calldata commitSha,
        string[] calldata packUris,
        string calldata expectedSha,
        bool force
    ) external {
        bytes32 repoId = _resolveWritableLocator(owner, repo);
        Repo storage repository = _repos[repoId];
        if (!_canWrite(repoId, repository, msg.sender)) revert Unauthorized(msg.sender);
        if (repository.moderationStatus == STATUS_FROZEN) revert FrozenRepo(repoId);

        _validateRefName(refName);
        _validateCommitSha(commitSha);
        if (packUris.length == 0) revert EmptyPackUris();
        if (packUris.length > MAX_PACK_URIS) revert TooManyPackUris(packUris.length, MAX_PACK_URIS);
        for (uint256 i; i < packUris.length; ++i) {
            _validatePackUri(packUris[i]);
        }

        bytes32 refId = keccak256(bytes(refName));
        Ref storage storedRef = _refs[repoId][refId];
        bool existed = storedRef.exists;
        if (existed && !force) {
            // Do not treat an omitted/empty expected SHA as permission to
            // overwrite a live ref. Callers must explicitly force-push when
            // they intend to bypass the concurrency check.
            if (bytes(expectedSha).length != 0) {
                _validateCommitSha(expectedSha);
            }
            if (keccak256(bytes(storedRef.commitSha)) != keccak256(bytes(expectedSha))) {
                revert ShaMismatch(expectedSha, storedRef.commitSha);
            }
        }

        if (!existed) {
            if (_refNames[repoId].length >= MAX_REFS_PER_REPO) {
                revert TooManyRefs(_refNames[repoId].length + 1, MAX_REFS_PER_REPO);
            }
            _refNames[repoId].push(refName);
            storedRef.exists = true;
        }
        storedRef.commitSha = commitSha;
        if (force || !existed) {
            delete storedRef.packUris;
        }
        // A normal update extends the ordered pack set, matching V1's
        // incremental-history semantics. Force updates replace it with the
        // self-contained pack set prepared by the remote helper.
        for (uint256 i; i < packUris.length; ++i) {
            if (force) {
                storedRef.packUris.push(packUris[i]);
            } else if (!_containsUri(storedRef.packUris, packUris[i])) {
                if (storedRef.packUris.length >= MAX_PACK_URIS) {
                    revert TooManyPackUris(storedRef.packUris.length + 1, MAX_PACK_URIS);
                }
                storedRef.packUris.push(packUris[i]);
            }
        }
        storedRef.updatedAt = uint64(block.timestamp);
        storedRef.updatedBy = msg.sender;
        repository.updatedAt = storedRef.updatedAt;

        emit RefUpdated(repoId, refName, commitSha, packUris, msg.sender);
    }

    /// @notice Delete a ref. Repository ownership is checked against `owner`.
    function deleteRef(address owner, string calldata repo, string calldata refName) external {
        bytes32 repoId = _resolveWritableLocator(owner, repo);
        Repo storage repository = _repos[repoId];
        if (!_canWrite(repoId, repository, msg.sender)) revert Unauthorized(msg.sender);
        if (repository.moderationStatus == STATUS_FROZEN) revert FrozenRepo(repoId);

        bytes32 refId = keccak256(bytes(refName));
        if (!_refs[repoId][refId].exists) revert RefNotFound(repoId, refId);
        delete _refs[repoId][refId];
        _removeRefName(repoId, refName);
        repository.updatedAt = uint64(block.timestamp);
        emit RefDeleted(repoId, refName, msg.sender);
    }

    function getRepo(address owner, string calldata repo) external view returns (Repo memory) {
        (bytes32 repoId,) = _resolveLocator(owner, repo);
        return _repos[repoId];
    }

    /// @notice Return a bounded page of repositories currently owned by an
    /// address. Repository IDs remain stable across ownership transfers;
    /// owner enumeration order is an internal index order and is not sorted.
    function listReposPage(
        address owner,
        uint256 cursor,
        uint256 limit
    )
        external
        view
        returns (uint256 nextCursor, bool hasMore, bytes32[] memory repoIds, Repo[] memory repositories)
    {
        _requireImportIdle();
        bytes32[] storage storedIds = _ownerRepoIds[owner];
        uint256 total = storedIds.length;
        _requirePageBounds(cursor, limit, total);
        uint256 end = cursor + limit;
        if (end > total) end = total;

        repoIds = new bytes32[](end - cursor);
        repositories = new Repo[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) {
            uint256 pageIndex = i - cursor;
            bytes32 repoId = storedIds[i];
            repoIds[pageIndex] = repoId;
            repositories[pageIndex] = _repos[repoId];
        }
        nextCursor = end;
        hasMore = end < total;
    }

    /// @notice Return a bounded page of refs addressed by the stable repo ID.
    /// `nextCursor` is the next array index and `hasMore` is explicit so a
    /// final page at index zero cannot be confused with an uninitialized
    /// cursor. The legacy locator wrapper above remains available for old
    /// readers, while new clients should resolve once and page by ID.
    function listRefsPageById(
        bytes32 repoId,
        uint256 cursor,
        uint256 limit
    )
        external
        view
        returns (uint256 nextCursor, bool hasMore, string[] memory names, Ref[] memory values)
    {
        string[] storage storedNames = _refNames[repoId];
        uint256 total = storedNames.length;
        _requirePage(repoId, cursor, limit, total);
        uint256 end = cursor + limit;
        if (end > total) end = total;

        names = new string[](end - cursor);
        values = new Ref[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) {
            uint256 pageIndex = i - cursor;
            names[pageIndex] = storedNames[i];
            values[pageIndex] = _refs[repoId][keccak256(bytes(storedNames[i]))];
        }
        nextCursor = end;
        hasMore = end < total;
    }

    function resolveRef(
        address owner,
        string calldata repo,
        string calldata refName
    ) external view returns (Ref memory) {
        (bytes32 repoId,) = _resolveLocator(owner, repo);
        bytes32 refId = keccak256(bytes(refName));
        if (!_refs[repoId][refId].exists) revert RefNotFound(repoId, refId);
        return _refs[repoId][refId];
    }

    /// @notice Set or remove a collaborator. Only the repository owner may
    /// manage roles; `Role.None` removes an existing collaborator.
    function setCollaborator(
        address owner,
        string calldata repo,
        address collaborator,
        Role role
    ) external {
        bytes32 repoId = _resolveWritableLocator(owner, repo);
        Repo storage repository = _repos[repoId];
        if (msg.sender != repository.owner) revert Unauthorized(msg.sender);
        if (collaborator == address(0) || collaborator == repository.owner) {
            revert InvalidCollaborator(collaborator);
        }
        if (role > Role.Reader) revert InvalidRole(uint8(role));

        Role previous = _roles[repoId][collaborator];
        if (role == Role.None) {
            if (previous != Role.None) {
                delete _roles[repoId][collaborator];
                _removeCollaborator(repoId, collaborator);
            }
        } else {
            if (previous == Role.None) {
                if (_collaborators[repoId].length >= MAX_COLLABORATORS_PER_REPO) {
                    revert TooManyCollaborators(
                        _collaborators[repoId].length + 1,
                        MAX_COLLABORATORS_PER_REPO
                    );
                }
                _collaborators[repoId].push(collaborator);
                _collaboratorIndexes[repoId][collaborator] = _collaborators[repoId].length;
            }
            _roles[repoId][collaborator] = role;
        }
        emit CollaboratorUpdated(repoId, collaborator, role, msg.sender);
    }

    function getCollaborator(
        address owner,
        string calldata repo,
        address collaborator
    ) external view returns (Role) {
        (bytes32 repoId,) = _resolveLocator(owner, repo);
        return _roles[repoId][collaborator];
    }

    /// @notice Return a bounded page of collaborators addressed by stable ID.
    /// The order is the contract's current collaborator index order; callers
    /// must treat it as an enumeration rather than a user-visible sort.
    function listCollaboratorsPageById(
        bytes32 repoId,
        uint256 cursor,
        uint256 limit
    )
        external
        view
        returns (uint256 nextCursor, bool hasMore, address[] memory collaborators, Role[] memory roles)
    {
        address[] storage stored = _collaborators[repoId];
        uint256 total = stored.length;
        _requirePage(repoId, cursor, limit, total);
        uint256 end = cursor + limit;
        if (end > total) end = total;

        collaborators = new address[](end - cursor);
        roles = new Role[](end - cursor);
        for (uint256 i = cursor; i < end; ++i) {
            uint256 pageIndex = i - cursor;
            collaborators[pageIndex] = stored[i];
            roles[pageIndex] = _roles[repoId][stored[i]];
        }
        nextCursor = end;
        hasMore = end < total;
    }

    /// @notice Freeze or unfreeze ref mutations. The repository owner or the
    /// deployment admin may change this flag.
    function setFrozen(address owner, string calldata repo, bool frozen) external {
        bytes32 repoId = _resolveWritableLocator(owner, repo);
        Repo storage repository = _repos[repoId];
        if (msg.sender != repository.owner && msg.sender != admin) revert Unauthorized(msg.sender);
        repository.moderationStatus = frozen ? STATUS_FROZEN : STATUS_ACTIVE;
        repository.updatedAt = uint64(block.timestamp);
        emit RepoFrozen(repoId, frozen, msg.sender);
    }

    /// @notice Resolve either a canonical locator or a historical alias.
    /// Reads through aliases return the current repository metadata so callers
    /// can construct the canonical URL without guessing from the input.
    function resolveRepo(
        address owner,
        string calldata repo
    ) external view returns (bytes32 repoId, bool isCanonical, Repo memory repository) {
        (repoId, isCanonical) = _resolveLocator(owner, repo);
        repository = _repos[repoId];
    }

    function getRepoById(bytes32 repoId) external view returns (Repo memory) {
        return _requireRepo(repoId);
    }

    function pendingOwnershipTransfer(
        bytes32 repoId
    ) external view returns (PendingOwnershipTransfer memory) {
        _requireRepo(repoId);
        return _pendingTransfers[repoId];
    }

    /// @notice Start the V1-compatible seven-day ownership transfer delay and
    /// reserve the target locator immediately to avoid a naming race.
    function beginOwnershipTransfer(bytes32 repoId, address newOwner) external {
        Repo storage repository = _requireRepo(repoId);
        if (msg.sender != repository.owner) revert TransferUnauthorized(repoId, msg.sender);
        if (newOwner == address(0) || newOwner == repository.owner) {
            revert InvalidTransferTarget(newOwner);
        }
        if (_pendingTransfers[repoId].newOwner != address(0)) {
            revert TransferAlreadyPending(repoId);
        }

        bytes32 targetKey = _locatorKey(newOwner, repository.name);
        _requireTransferTargetAvailable(targetKey, repoId, newOwner, repository.name);

        uint64 proposedAt = _toTimestamp(block.timestamp);
        uint64 executeAfter = _toTimestamp(block.timestamp + OWNERSHIP_TRANSFER_DELAY);
        uint64 expiresAt = _toTimestamp(
            block.timestamp + OWNERSHIP_TRANSFER_DELAY + OWNERSHIP_TRANSFER_ACCEPTANCE_WINDOW
        );
        _pendingTransfers[repoId] = PendingOwnershipTransfer({
            newOwner: newOwner,
            proposedAt: proposedAt,
            executeAfter: executeAfter,
            expiresAt: expiresAt
        });
        _locatorReservations[targetKey] = LocatorReservation({
            subjectId: repoId,
            kind: ReservationKind.OwnershipTransfer
        });

        emit OwnershipTransferStarted(
            repoId,
            repository.owner,
            newOwner,
            repository.name,
            proposedAt,
            executeAfter,
            expiresAt
        );
    }

    function cancelOwnershipTransfer(bytes32 repoId) external {
        Repo storage repository = _requireRepo(repoId);
        if (msg.sender != repository.owner) revert TransferUnauthorized(repoId, msg.sender);

        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner == address(0)) revert TransferNotPending(repoId);

        _clearOwnershipTransfer(repoId, repository.name, pending);

        emit OwnershipTransferCancelled(repoId, repository.owner, pending.newOwner);
    }

    /// @notice Let the proposed owner immediately release their namespace.
    function rejectOwnershipTransfer(bytes32 repoId) external {
        Repo storage repository = _requireRepo(repoId);
        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner == address(0)) revert TransferNotPending(repoId);
        if (msg.sender != pending.newOwner) revert TransferUnauthorized(repoId, msg.sender);

        _clearOwnershipTransfer(repoId, repository.name, pending);
        emit OwnershipTransferRejected(repoId, repository.owner, pending.newOwner);
    }

    /// @notice Permissionless cleanup after the target's acceptance window.
    function expireOwnershipTransfer(bytes32 repoId) external {
        Repo storage repository = _requireRepo(repoId);
        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner == address(0)) revert TransferNotPending(repoId);
        if (block.timestamp <= pending.expiresAt) {
            revert TransferNotExpired(repoId, pending.expiresAt);
        }

        _clearOwnershipTransfer(repoId, repository.name, pending);
        emit OwnershipTransferExpired(repoId, repository.owner, pending.newOwner);
    }

    /// @notice Accept a matured transfer. Refs, collaborators, metadata, and
    /// timestamps remain keyed by the immutable repoId and are not copied.
    function acceptOwnership(bytes32 repoId) external {
        Repo storage repository = _requireRepo(repoId);
        PendingOwnershipTransfer memory pending = _pendingTransfers[repoId];
        if (pending.newOwner == address(0)) revert TransferNotPending(repoId);
        if (msg.sender != pending.newOwner) revert TransferUnauthorized(repoId, msg.sender);
        if (block.timestamp < pending.executeAfter) {
            revert TransferTooEarly(repoId, pending.executeAfter);
        }
        if (block.timestamp > pending.expiresAt) {
            revert TransferExpired(repoId, pending.expiresAt);
        }

        bytes32 oldKey = _locatorKey(repository.owner, repository.name);
        bytes32 targetKey = _locatorKey(pending.newOwner, repository.name);
        _requireOwnershipReservation(targetKey, repoId);

        address oldOwner = repository.owner;
        delete _activeLocators[oldKey];
        _aliases[oldKey] = repoId;
        delete _locatorReservations[targetKey];
        if (_aliases[targetKey] == repoId) delete _aliases[targetKey];
        _activeLocators[targetKey] = repoId;
        delete _pendingTransfers[repoId];
        _moveOwnerRepo(repoId, oldOwner, pending.newOwner);
        repository.owner = pending.newOwner;

        if (_roles[repoId][pending.newOwner] != Role.None) {
            delete _roles[repoId][pending.newOwner];
            _removeCollaborator(repoId, pending.newOwner);
            emit CollaboratorUpdated(repoId, pending.newOwner, Role.None, msg.sender);
        }

        emit OwnershipTransferred(
            repoId,
            oldOwner,
            pending.newOwner,
            msg.sender,
            repository.name,
            pending.executeAfter
        );
    }

    function computeRepoId(address initialOwner, string calldata repo) external view returns (bytes32) {
        return _nativeRepoId(initialOwner, repo);
    }

    function _nativeRepoId(address initialOwner, string memory repo) internal view returns (bytes32) {
        return keccak256(abi.encode(REPO_ID_DOMAIN, identityChainId, address(this), initialOwner, repo));
    }

    function _importedRepoId(
        string memory sourceChainId,
        address sourceContract,
        address owner,
        string memory repo
    ) internal view returns (bytes32) {
        return keccak256(
            abi.encode(
                IMPORT_DOMAIN,
                identityChainId,
                address(this),
                sourceChainId,
                sourceContract,
                owner,
                repo
            )
        );
    }

    function _requireAdmin() internal view {
        if (msg.sender != admin) revert Unauthorized(msg.sender);
    }

    function _requireImportIdle() internal view {
        if (activeImportSession != bytes32(0)) {
            revert ImportStateLocked(activeImportSession);
        }
    }

    function _requireActiveImport(
        bytes32 sessionId
    ) internal view returns (ImportSession storage session) {
        session = _importSessions[sessionId];
        if (
            sessionId == bytes32(0) ||
            activeImportSession != sessionId ||
            !session.active ||
            session.finalized
        ) revert ImportSessionNotActive(sessionId);
    }

    function _validateImportStatus(uint8 status) internal pure {
        if (status > STATUS_DELISTED) revert InvalidImportStatus(status);
    }

    function _validateImportBatch(
        ImportSession storage session,
        uint256 sequence,
        bytes32 payloadHash
    ) internal view {
        if (payloadHash == bytes32(0)) revert InvalidImportPayloadHash();
        if (sequence != session.nextSequence) {
            revert ImportSequenceMismatch(session.nextSequence, sequence);
        }
        if (session.nextSequence >= session.expectedBatches) {
            revert ImportCountMismatch(
                bytes32("batches"),
                session.expectedBatches,
                session.nextSequence
            );
        }
    }

    function _completeImportBatch(ImportSession storage session) internal {
        unchecked {
            session.nextSequence += 1;
        }
    }

    function _requireImportRepo(
        bytes32 sessionId,
        bytes32 repoId
    ) internal view returns (ImportRepoState storage state) {
        state = _importRepoStates[repoId];
        if (state.sessionId != sessionId) {
            revert ImportRepoSessionMismatch(repoId, sessionId, state.sessionId);
        }
        if (!_repos[repoId].exists) revert RepoNotFound(repoId);
    }

    function _markImportRepoComplete(
        ImportSession storage session,
        ImportRepoState storage state
    ) internal {
        if (
            !state.complete &&
            state.importedRefs == state.expectedRefs &&
            state.importedCollaborators == state.expectedCollaborators
        ) {
            state.complete = true;
            session.incompleteRepositories -= 1;
        }
    }

    function _locatorKey(address owner, string memory repo) internal pure returns (bytes32) {
        return keccak256(abi.encode(owner, repo));
    }

    function _resolveLocator(
        address owner,
        string memory repo
    ) internal view returns (bytes32 repoId, bool isCanonical) {
        _requireImportIdle();
        bytes32 locatorKey = _locatorKey(owner, repo);
        repoId = _activeLocators[locatorKey];
        if (repoId != bytes32(0)) return (repoId, true);

        repoId = _aliases[locatorKey];
        if (repoId != bytes32(0)) return (repoId, false);

        revert LocatorNotFound(owner, repo);
    }

    function _resolveWritableLocator(address owner, string memory repo) internal view returns (bytes32 repoId) {
        bool isCanonical;
        (repoId, isCanonical) = _resolveLocator(owner, repo);
        if (!isCanonical) {
            Repo storage repository = _repos[repoId];
            revert RepoMoved(repoId, repository.owner, repository.name);
        }
    }

    function _requireRepo(bytes32 repoId) internal view returns (Repo storage repository) {
        _requireImportIdle();
        repository = _repos[repoId];
        if (!repository.exists) revert RepoNotFound(repoId);
    }

    function _requirePage(bytes32 repoId, uint256 cursor, uint256 limit, uint256 total) internal view {
        _requireRepo(repoId);
        _requirePageBounds(cursor, limit, total);
    }

    function _requirePageBounds(uint256 cursor, uint256 limit, uint256 total) internal pure {
        if (limit == 0 || limit > MAX_QUERY_PAGE_SIZE) {
            revert InvalidPageSize(limit, MAX_QUERY_PAGE_SIZE);
        }
        // A cursor equal to the end is a valid empty page.
        if (cursor > total) {
            revert InvalidCursor(cursor, total);
        }
    }

    function _addOwnerRepo(address owner, bytes32 repoId) private {
        if (_ownerRepoIndexes[repoId] != 0) revert RepoIndexCorrupted(repoId, owner);
        _ownerRepoIds[owner].push(repoId);
        _ownerRepoIndexes[repoId] = _ownerRepoIds[owner].length;
    }

    function _moveOwnerRepo(bytes32 repoId, address oldOwner, address newOwner) private {
        bytes32[] storage oldIds = _ownerRepoIds[oldOwner];
        uint256 indexPlusOne = _ownerRepoIndexes[repoId];
        if (
            indexPlusOne == 0 ||
            indexPlusOne > oldIds.length ||
            oldIds[indexPlusOne - 1] != repoId
        ) revert RepoIndexCorrupted(repoId, oldOwner);

        uint256 index = indexPlusOne - 1;
        uint256 last = oldIds.length - 1;
        if (index != last) {
            bytes32 moved = oldIds[last];
            oldIds[index] = moved;
            _ownerRepoIndexes[moved] = indexPlusOne;
        }
        oldIds.pop();
        delete _ownerRepoIndexes[repoId];
        _addOwnerRepo(newOwner, repoId);
    }

    function _requireLocatorAvailable(bytes32 locatorKey, address owner, string memory repo) internal view {
        if (
            _activeLocators[locatorKey] != bytes32(0) ||
            _aliases[locatorKey] != bytes32(0) ||
            _locatorReservations[locatorKey].kind != ReservationKind.None
        ) revert LocatorUnavailable(owner, repo);
    }

    function _requireTransferTargetAvailable(
        bytes32 locatorKey,
        bytes32 repoId,
        address owner,
        string memory repo
    ) internal view {
        if (
            _activeLocators[locatorKey] != bytes32(0) ||
            (_aliases[locatorKey] != bytes32(0) && _aliases[locatorKey] != repoId) ||
            _locatorReservations[locatorKey].kind != ReservationKind.None
        ) revert LocatorUnavailable(owner, repo);
    }

    function _requireOwnershipReservation(bytes32 locatorKey, bytes32 repoId) internal view {
        LocatorReservation storage reservation = _locatorReservations[locatorKey];
        if (
            reservation.subjectId != repoId ||
            reservation.kind != ReservationKind.OwnershipTransfer
        ) revert LocatorReservationMismatch(locatorKey, repoId);
    }

    function _clearOwnershipTransfer(
        bytes32 repoId,
        string memory repo,
        PendingOwnershipTransfer memory pending
    ) internal {
        bytes32 targetKey = _locatorKey(pending.newOwner, repo);
        _requireOwnershipReservation(targetKey, repoId);
        delete _locatorReservations[targetKey];
        delete _pendingTransfers[repoId];
    }

    function _toTimestamp(uint256 timestamp) internal pure returns (uint64) {
        if (timestamp > type(uint64).max) revert TimestampOverflow(timestamp);
        return uint64(timestamp);
    }

    function _requireLength(string memory value, uint256 maximum) internal pure {
        uint256 length = bytes(value).length;
        if (length == 0) revert EmptyInput();
        if (length > maximum) revert InputTooLong(length, maximum);
    }

    function _validateRepoName(string memory value) internal pure {
        uint256 length = bytes(value).length;
        if (length == 0) revert EmptyInput();
        if (length > MAX_NAME_LENGTH) revert InputTooLong(length, MAX_NAME_LENGTH);
        bytes memory chars = bytes(value);
        for (uint256 i; i < length; ++i) {
            uint8 c = uint8(chars[i]);
            bool alphaNum = (c >= 0x30 && c <= 0x39) || (c >= 0x41 && c <= 0x5a) || (c >= 0x61 && c <= 0x7a);
            if (!alphaNum && c != 0x2e && c != 0x5f && c != 0x2d) revert InvalidRepoName(value);
        }
    }

    function _validateRefName(string memory value) internal pure {
        uint256 length = bytes(value).length;
        if (length < 5 || length > MAX_REF_NAME_LENGTH) revert InvalidRefName(value);
        bytes memory chars = bytes(value);
        if (!(chars[0] == bytes1("r") && chars[1] == bytes1("e") && chars[2] == bytes1("f") && chars[3] == bytes1("s") && chars[4] == bytes1("/"))) {
            revert InvalidRefName(value);
        }
        for (uint256 i; i < length; ++i) {
            uint8 c = uint8(chars[i]);
            if (c <= 0x20 || c == 0x7f || c == 0x7e || c == 0x5e || c == 0x3a || c == 0x5c) revert InvalidRefName(value);
            if (i + 1 < length && chars[i] == bytes1(".") && chars[i + 1] == bytes1(".")) revert InvalidRefName(value);
        }
    }

    function _validateCommitSha(string memory commitSha) internal pure {
        uint256 length = bytes(commitSha).length;
        if (length != 40 && length != 64) revert InvalidCommitSha(commitSha);
        bytes memory value = bytes(commitSha);
        for (uint256 i; i < length; ++i) {
            uint8 c = uint8(value[i]);
            bool digit = c >= 0x30 && c <= 0x39;
            bool lower = c >= 0x61 && c <= 0x66;
            bool upper = c >= 0x41 && c <= 0x46;
            if (!digit && !lower && !upper) revert InvalidCommitSha(commitSha);
        }
    }

    function _validatePackUri(string memory uri) internal pure {
        bytes memory value = bytes(uri);
        if (value.length <= 7 || value.length > MAX_PACK_URI_LENGTH) revert InvalidPackUri(uri);
        if (
            value[0] != bytes1("i") ||
            value[1] != bytes1("p") ||
            value[2] != bytes1("f") ||
            value[3] != bytes1("s") ||
            value[4] != bytes1(":") ||
            value[5] != bytes1("/") ||
            value[6] != bytes1("/")
        ) revert InvalidPackUri(uri);
        uint8 first = uint8(value[7]);
        bool locatorStartsAlphaNum = (first >= 0x30 && first <= 0x39) ||
            (first >= 0x41 && first <= 0x5a) ||
            (first >= 0x61 && first <= 0x7a);
        if (!locatorStartsAlphaNum) revert InvalidPackUri(uri);
        for (uint256 i = 7; i < value.length; ++i) {
            uint8 c = uint8(value[i]);
            if (c <= 0x20 || c == 0x7f) revert InvalidPackUri(uri);
        }
    }

    function _canWrite(bytes32 repoId, Repo storage repository, address caller) internal view returns (bool) {
        return caller == repository.owner || _roles[repoId][caller] == Role.Maintainer;
    }

    function _removeCollaborator(bytes32 repoId, address collaborator) private {
        address[] storage collaborators = _collaborators[repoId];
        uint256 indexPlusOne = _collaboratorIndexes[repoId][collaborator];
        if (indexPlusOne == 0) return;

        uint256 index = indexPlusOne - 1;
        uint256 last = collaborators.length - 1;
        if (index != last) {
            address moved = collaborators[last];
            collaborators[index] = moved;
            _collaboratorIndexes[repoId][moved] = indexPlusOne;
        }
        collaborators.pop();
        delete _collaboratorIndexes[repoId][collaborator];
    }

    function _removeRefName(bytes32 repoId, string calldata refName) private {
        string[] storage names = _refNames[repoId];
        bytes32 target = keccak256(bytes(refName));
        uint256 length = names.length;
        for (uint256 i; i < length; ++i) {
            if (keccak256(bytes(names[i])) == target) {
                if (i != length - 1) names[i] = names[length - 1];
                names.pop();
                return;
            }
        }
    }

    function _containsUri(string[] storage values, string calldata candidate) private view returns (bool) {
        bytes32 wanted = keccak256(bytes(candidate));
        for (uint256 i; i < values.length; ++i) {
            if (keccak256(bytes(values[i])) == wanted) return true;
        }
        return false;
    }
}
