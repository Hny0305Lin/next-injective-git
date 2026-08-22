import { keccak256, parseAbi, toHex, type Abi, type Hex } from "viem";

export type SuiteModuleKey =
  | "core"
  | "recovery"
  | "moderation"
  | "economic"
  | "username"
  | "badge"
  | "release";

export const SUITE_VERSION = 3n;

export const MODULE_IDS: Record<SuiteModuleKey, Hex> = {
  core: keccak256(toHex("igit.module.repository-core")),
  recovery: keccak256(toHex("igit.module.recovery")),
  moderation: keccak256(toHex("igit.module.moderation")),
  economic: keccak256(toHex("igit.module.economic")),
  username: keccak256(toHex("igit.module.username")),
  badge: keccak256(toHex("igit.module.badge")),
  release: keccak256(toHex("igit.module.release")),
};

export const MODULE_KEYS = Object.keys(MODULE_IDS) as SuiteModuleKey[];

export const directoryAbi = parseAbi([
  "function state() view returns (uint8)",
  "function suiteVersion() view returns (uint64)",
  "function configuredChainId() view returns (uint256)",
  "function snapshotRoot() view returns (bytes32)",
  "function bootstrapCoordinator() view returns (address)",
  "function bootstrapCoordinatorCodeHash() view returns (bytes32)",
  "function registeredModuleCount() view returns (uint256)",
  "function moduleAddress(bytes32 id) view returns (address)",
  "function moduleCodeHash(bytes32 id) view returns (bytes32)",
  "function verifyModule(bytes32 id) view returns (bool)",
]);

export const boundModuleAbi = parseAbi([
  "function suiteDirectory() view returns (address)",
  "function bootstrapCoordinator() view returns (address)",
  "function moduleId() view returns (bytes32)",
]);

export const coreAbi = parseAbi([
  "function resolveRepository(address owner, string name) view returns ((bytes32 id,address owner,string name,string description,string defaultBranch,bytes32 forkedFrom,uint64 createdAt,uint64 updatedAt,bool exists) repository,bool canonical)",
  "function getRepository(bytes32 repoId) view returns ((bytes32 id,address owner,string name,string description,string defaultBranch,bytes32 forkedFrom,uint64 createdAt,uint64 updatedAt,bool exists))",
  "function listRepositoriesPage(address owner,uint256 cursor,uint256 limit) view returns ((bytes32 id,address owner,string name,string description,string defaultBranch,bytes32 forkedFrom,uint64 createdAt,uint64 updatedAt,bool exists)[] page,uint256 nextCursor)",
  "function getRef(bytes32 repoId,string refName) view returns ((string commitSha,string[] packUris,uint64 updatedAt,address updatedBy,bool exists))",
  "function listRefsPage(bytes32 repoId,uint256 cursor,uint256 limit) view returns (string[] names,(string commitSha,string[] packUris,uint64 updatedAt,address updatedBy,bool exists)[] refs,uint256 nextCursor)",
  "function listCollaboratorsPage(bytes32 repoId,uint256 cursor,uint256 limit) view returns (address[] accounts,uint8[] roles,uint256 nextCursor)",
  "function pendingOwnershipTransfer(bytes32 repoId) view returns ((address newOwner,uint64 executeAfter,uint64 expiresAt))",
  "function updateMetadata(bytes32 repoId,bool updateDescription,string description,bool updateDefaultBranch,string defaultBranch)",
  "function beginOwnershipTransfer(bytes32 repoId,address newOwner)",
  "function cancelOwnershipTransfer(bytes32 repoId)",
  "function expireOwnershipTransfer(bytes32 repoId)",
  "function acceptOwnershipTransfer(bytes32 repoId)",
  "event RepositoryCreated(bytes32 indexed repoId,address indexed owner,string name,bytes32 indexed forkedFrom)",
  "event RepositoryMetadataUpdated(bytes32 indexed repoId,address indexed owner,uint8 fieldMask,string description,string defaultBranch)",
  "event RefUpdated(bytes32 indexed repoId,string indexed refName,string commitSha,string[] packUris,address indexed updatedBy)",
  "event RefDeleted(bytes32 indexed repoId,string indexed refName,address indexed deletedBy)",
  "event CollaboratorUpdated(bytes32 indexed repoId,address indexed account,uint8 role,address indexed updatedBy)",
  "event OwnershipTransferStarted(bytes32 indexed repoId,address indexed oldOwner,address indexed newOwner,uint64 executeAfter,uint64 expiresAt)",
  "event OwnershipTransferCancelled(bytes32 indexed repoId,address indexed oldOwner,address indexed newOwner)",
  "event OwnershipTransferred(bytes32 indexed repoId,address indexed oldOwner,address indexed newOwner,string name,bool recovered)",
]);

export const recoveryAbi = parseAbi([
  "function guardianConfig(bytes32 repoId) view returns ((address configuredBy,uint8 threshold,address[] guardians))",
  "function recoveryProposal(bytes32 repoId) view returns ((address proposedBy,address newOwner,uint64 executeAfter,uint64 expiresAt,uint64 nonce,uint8 approvals))",
  "function setGuardians(bytes32 repoId,address[] guardians,uint8 threshold)",
  "function proposeRecovery(bytes32 repoId,address newOwner)",
  "function approveRecovery(bytes32 repoId)",
  "function cancelRecovery(bytes32 repoId)",
  "function executeRecovery(bytes32 repoId)",
]);

export const moderationAbi = parseAbi([
  "function effectiveStatus(bytes32 repoId) view returns (uint8)",
  "function getReport(uint256 reportId) view returns ((uint256 id,bytes32 repoId,address reporter,uint8 status,uint8 resolution,string reasonHash,uint64 createdAt,uint64 updatedAt,bool exists))",
  "function submitReport(bytes32 repoId,string reasonHash) returns (uint256)",
  "function setRepositoryStatus(bytes32 repoId,uint8 status,string reasonHash)",
  "function resolveReport(uint256 reportId,uint8 status,string reasonHash)",
  "function appealReport(uint256 reportId,string reasonHash)",
  "function resolveAppeal(uint256 reportId,uint8 status,string reasonHash)",
  "function committee() view returns (address)",
  "event RepositoryStatusSet(bytes32 indexed repoId,uint8 status,address indexed updatedBy,string reasonHash)",
  "event ReportSubmitted(uint256 indexed reportId,bytes32 indexed repoId,address indexed reporter,string reasonHash)",
]);

export const economicAbi = parseAbi([
  "function admin() view returns (address)",
  "function treasury() view returns (address)",
  "function platformFeeBps() view returns (uint16)",
  "function revenueSplits(bytes32 repoId) view returns ((address recipient,uint16 bps)[])",
  "function sponsorDenoms(bytes32 repoId) view returns (string[])",
  "function sponsorTotal(bytes32 repoId,string denom) view returns (uint256)",
  "function sponsor(bytes32 repoId,string message) payable",
  "function setRevenueSplits(bytes32 repoId,address[] recipients,uint16[] bps)",
  "event SponsorSettled(bytes32 indexed repoId,address indexed sponsor,uint256 amount,uint256 platformFee,string message)",
  "event RevenueSplitsUpdated(bytes32 indexed repoId,address indexed owner,uint256 totalBps)",
]);

export const usernameAbi = parseAbi([
  "function resolveUsername(string name) view returns ((address owner,uint64 registeredAt))",
  "function usernameOf(address owner) view returns (string)",
  "function originalOwnerOf(string name) view returns (address)",
  "function isReserved(string name) view returns (bool)",
  "function registerUsername(string name)",
  "function claimOriginalUsername(string name)",
  "function releaseUsername()",
]);

export const badgeAbi = parseAbi([
  "function getBadge(uint256 badgeId) view returns ((uint256 id,bytes32 repoId,address recipient,address awardedBy,string reason,uint64 awardedAt,bool exists))",
  "function listBadgesByRecipientPage(address recipient,uint256 cursor,uint256 limit) view returns ((uint256 id,bytes32 repoId,address recipient,address awardedBy,string reason,uint64 awardedAt,bool exists)[] page,uint256 nextCursor)",
  "function listBadgesByRepositoryPage(bytes32 repoId,uint256 cursor,uint256 limit) view returns ((uint256 id,bytes32 repoId,address recipient,address awardedBy,string reason,uint64 awardedAt,bool exists)[] page,uint256 nextCursor)",
  "function awardBadge(bytes32 repoId,address recipient,string reason) returns (uint256)",
  "event BadgeAwarded(uint256 indexed badgeId,bytes32 indexed repoId,address indexed recipient,address awardedBy,string reason)",
]);

export const releaseAbi = parseAbi([
  "function getArtifact(string version,string platform) view returns ((string version,string platform,bytes32 sha256,address registeredBy,uint64 registeredAt,bool exists))",
  "function listArtifactsPage(string version,uint256 cursor,uint256 limit) view returns ((string version,string platform,bytes32 sha256,address registeredBy,uint64 registeredAt,bool exists)[] page,uint256 nextCursor)",
  "function registerArtifact(string version,string platform,bytes32 digest)",
]);

export const MODULE_ABIS: Record<SuiteModuleKey, Abi> = {
  core: coreAbi,
  recovery: recoveryAbi,
  moderation: moderationAbi,
  economic: economicAbi,
  username: usernameAbi,
  badge: badgeAbi,
  release: releaseAbi,
};

export const activityAbis = [coreAbi, moderationAbi, economicAbi, badgeAbi] as const;
