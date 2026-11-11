import { encodeAbiParameters, keccak256, zeroAddress, zeroHash, type Address, type Hex } from "viem";
import { sameAddress, toEvmAddress, toInjectiveAddress } from "./address";
import { coreAbi, successorCoreAbi, SUITE_SUCCESSOR_VERSION, SUITE_VERSION } from "./abis";
import {
  readModule,
  rawModuleRead,
  verifySuite,
  writeModule,
  writeVerifiedModule,
  type Eip1193,
  type SuiteBinding,
} from "./transport";
import { refShapeForVersion } from "./suite-compat";
import { fetchVerifiedManifest } from "./successorReader";
import { parseSuiteDirectories, type AppConfig } from "./profile";

export type ModerationStatus = "active" | "frozen" | "delisted";

export interface RepoInfo {
  owner: string;
  name: string;
  description: string;
  default_branch: string;
  created_at: number;
  updated_at: number;
  moderation_status: ModerationStatus;
  forked_from: string | null;
  /** Suite protocol version (3n IPFS, 4n BYOS). */
  suite_version?: bigint;
}

export interface RefInfo {
  ref_name: string;
  commit_sha: string;
  pack_uris: string[];
  updated_at: number;
  updated_by: string;
  /** Present when the ref carries a successor on-chain manifest commitment. */
  commitment?: SuccessorCommitment;
}

export interface CollaboratorInfo {
  address: string;
  role: "maintainer" | "reader";
}

export interface ResolvedRepo {
  repoId: Hex;
  requested: { owner: string; name: string };
  canonical: { owner: string; name: string };
  isCanonical: boolean;
  info: RepoInfo;
  /** The single SuiteDirectory that actually resolved this repository. */
  suiteDirectory?: string;
}

export interface RepoInfoPatch {
  description?: string;
  defaultBranch?: string;
}

export interface RepositoryWriteResult {
  txHash: Hex;
  /** A second transaction is used only when a fork's editable metadata changes. */
  metadataTxHash?: Hex;
}

export class EVMForkMetadataError extends Error {
  constructor(public readonly forkTxHash: Hex, cause: unknown) {
    super(`fork was created, but updating its metadata failed: ${cause instanceof Error ? cause.message : String(cause)}`, { cause });
    this.name = "EVMForkMetadataError";
  }
}

export interface PendingOwnershipTransfer {
  newOwner: string;
  executeAfter: number;
  expiresAt: number;
}

export interface OwnershipTransferCapabilities {
  canCancel: boolean;
  canAccept: boolean;
  canReject: boolean;
  canExpire: boolean;
}

interface RawRepository {
  id: Hex;
  owner: Address;
  name: string;
  description: string;
  defaultBranch: string;
  forkedFrom: Hex;
  createdAt: bigint;
  updatedAt: bigint;
  exists: boolean;
}

interface RawRef {
  commitSha: string;
  packUris: readonly string[];
  updatedAt: bigint;
  updatedBy: Address;
  exists: boolean;
}

const PAGE_SIZE = 64n;
const queryCache = new Map<string, { value: unknown; timestamp: number }>();
const CACHE_TTL_MS = 30_000;
const MAX_REPOSITORY_NAME_BYTES = 64;
const MAX_DESCRIPTION_BYTES = 1024;
const MAX_DEFAULT_BRANCH_BYTES = 64;
const REPOSITORY_NAME_PATTERN = /^[A-Za-z0-9._-]+$/;

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length;
}

/** Mirror RepositoryCore's name checks before asking the wallet to sign. */
export function validateRepositoryName(value: string): string {
  const name = value;
  if (!name) throw new Error("repository name is required");
  if (utf8Length(name) > MAX_REPOSITORY_NAME_BYTES) {
    throw new Error(`repository name must be at most ${MAX_REPOSITORY_NAME_BYTES} UTF-8 bytes`);
  }
  if (!REPOSITORY_NAME_PATTERN.test(name)) {
    throw new Error("repository name may contain only letters, numbers, '-', '_' and '.'");
  }
  return name;
}

export function validateRepositoryMetadata(description: string, defaultBranch: string): { description: string; defaultBranch: string } {
  const normalizedDescription = description.trim();
  const normalizedBranch = defaultBranch.trim() || "main";
  if (utf8Length(normalizedDescription) > MAX_DESCRIPTION_BYTES) {
    throw new Error(`description must be at most ${MAX_DESCRIPTION_BYTES} UTF-8 bytes`);
  }
  if (utf8Length(normalizedBranch) > MAX_DEFAULT_BRANCH_BYTES) {
    throw new Error(`default branch must be at most ${MAX_DEFAULT_BRANCH_BYTES} UTF-8 bytes`);
  }
  return { description: normalizedDescription, defaultBranch: normalizedBranch };
}

export async function repositoryNameAvailable(
  cfg: AppConfig,
  owner: string,
  name: string,
): Promise<void> {
  const normalizedName = validateRepositoryName(name);
  try {
    // Writes target the first verified Directory, so check that same suite.
    const primaryDirectory = cfg.suiteDirectory.split(",")[0].trim();
    const existing = await resolveRepoSingleDir({ ...cfg, suiteDirectory: primaryDirectory }, owner, normalizedName);
    throw new Error(`repository name is already used by ${existing.canonical.owner}/${existing.canonical.name}`);
  } catch (error) {
    if (error instanceof EVMLocatorNotFoundError) return;
    throw error;
  }
}

function coreWriteAbi(version: bigint) {
  return version === SUITE_SUCCESSOR_VERSION ? successorCoreAbi : coreAbi;
}

export async function createRepositoryWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  name: string,
  description: string,
  defaultBranch: string,
  expectedAccount?: string,
): Promise<Hex> {
  const normalizedName = validateRepositoryName(name);
  const metadata = validateRepositoryMetadata(description, defaultBranch);
  const binding = await verifySuite(cfg, true);
  if (binding.version !== SUITE_VERSION && binding.version !== SUITE_SUCCESSOR_VERSION) {
    throw new Error(`Web repository creation is disabled for unrecognised Suite v${binding.version}`);
  }
  if (!expectedAccount) throw new Error("Connect a wallet before creating a repository");
  const writeConfig = { ...cfg, suiteDirectory: binding.directory };
  await repositoryNameAvailable(writeConfig, expectedAccount, normalizedName);
  const hash = await writeVerifiedModule(
    provider,
    writeConfig,
    "core",
    coreWriteAbi(binding.version),
    "createRepository",
    [normalizedName, metadata.description, metadata.defaultBranch],
    undefined,
    expectedAccount,
    binding.version,
  );
  clearQueryCache();
  return hash;
}

/**
 * Fork a Suite v3 public-IPFS repository, then apply the form's editable
 * metadata. Suite v4 BYOS is deliberately rejected: its storage is owned by
 * the repository user, so Web cannot promise a durable independent fork.
 */
export async function forkRepositoryWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  sourceRepoId: Hex,
  newName: string,
  metadata: RepoInfoPatch,
  expectedAccount?: string,
): Promise<RepositoryWriteResult> {
  const normalizedName = validateRepositoryName(newName);
  if (!expectedAccount) throw new Error("Connect a wallet before forking a repository");
  const normalizedPatch: RepoInfoPatch = {
    ...(metadata.description === undefined ? {} : { description: metadata.description.trim() }),
    ...(metadata.defaultBranch === undefined ? {} : { defaultBranch: metadata.defaultBranch.trim() || "main" }),
  };
  if (normalizedPatch.description !== undefined && utf8Length(normalizedPatch.description) > MAX_DESCRIPTION_BYTES) {
    throw new Error(`description must be at most ${MAX_DESCRIPTION_BYTES} UTF-8 bytes`);
  }
  if (normalizedPatch.defaultBranch !== undefined && utf8Length(normalizedPatch.defaultBranch) > MAX_DEFAULT_BRANCH_BYTES) {
    throw new Error(`default branch must be at most ${MAX_DEFAULT_BRANCH_BYTES} UTF-8 bytes`);
  }
  const binding = await verifySuite(cfg, true);
  if (binding.version !== SUITE_VERSION) {
    throw new Error("Web Fork is available only for the verified Suite v3 public-IPFS path; Suite v4 BYOS forks are disabled because their storage is user-controlled.");
  }
  const writeConfig = { ...cfg, suiteDirectory: binding.directory };
  const source = await repoInfoById(writeConfig, sourceRepoId, binding);
  if (source.moderation_status !== "active") {
    throw new Error("This repository cannot be forked because it is not active");
  }
  await repositoryNameAvailable(writeConfig, expectedAccount, normalizedName);
  const forkTxHash = await writeVerifiedModule(
    provider,
    writeConfig,
    "core",
    coreAbi,
    "forkRepository",
    [sourceRepoId, normalizedName],
    undefined,
    expectedAccount,
    binding.version,
  );
  clearQueryCache();
  if (normalizedPatch.description === undefined && normalizedPatch.defaultBranch === undefined) {
    return { txHash: forkTxHash };
  }
  try {
    const changedPatch: RepoInfoPatch = {
      ...(normalizedPatch.description === undefined || normalizedPatch.description === source.description
        ? {} : { description: normalizedPatch.description }),
      ...(normalizedPatch.defaultBranch === undefined || normalizedPatch.defaultBranch === source.default_branch
        ? {} : { defaultBranch: normalizedPatch.defaultBranch }),
    };
    if (changedPatch.description === undefined && changedPatch.defaultBranch === undefined) {
      return { txHash: forkTxHash };
    }
    const metadataTxHash = await updateRepoInfoWithEvm(
      provider,
      writeConfig,
      repositoryIdForName(cfg.evmChainId, binding.directory, expectedAccount, normalizedName),
      changedPatch,
      expectedAccount,
    );
    return { txHash: forkTxHash, metadataTxHash };
  } catch (cause) {
    throw new EVMForkMetadataError(forkTxHash, cause);
  }
}

function repositoryIdForName(chainId: number, directory: Address, owner: string, name: string): Hex {
  return keccak256(encodeAbiParameters(
    [{ type: "string" }, { type: "uint256" }, { type: "address" }, { type: "address" }, { type: "string" }],
    ["igit:suite:v3:repo", BigInt(chainId), directory, toEvmAddress(owner), name],
  ));
}

export class EVMRepoNotFoundError extends Error {
  constructor(public readonly repoId: string) {
    super(`repository not found (repo ID ${repoId})`);
    this.name = "EVMRepoNotFoundError";
  }
}

export class EVMLocatorNotFoundError extends Error {
  constructor(public readonly owner: string, public readonly repo: string) {
    super(`repository locator not found: igit://${owner}/${repo}`);
    this.name = "EVMLocatorNotFoundError";
  }
}

function safeNumber(value: bigint, label: string): number {
  const number = Number(value);
  if (!Number.isSafeInteger(number)) throw new Error(`${label} exceeds the JavaScript safe integer range`);
  return number;
}

function statusName(value: unknown): ModerationStatus {
  const status = Number(value);
  const name = (["active", "frozen", "delisted"] as const)[status];
  if (!name) throw new Error(`invalid moderation status ${String(value)}`);
  return name;
}

async function mapRepository(cfg: AppConfig, raw: RawRepository, binding: SuiteBinding): Promise<RepoInfo> {
  if (!raw.exists || raw.id === zeroHash) throw new EVMRepoNotFoundError(raw.id);
  const moderation = await readModule(cfg, "moderation", "effectiveStatus", [raw.id], binding);
  return {
    owner: toInjectiveAddress(raw.owner),
    name: raw.name,
    description: raw.description,
    default_branch: raw.defaultBranch,
    created_at: safeNumber(raw.createdAt, "repository created timestamp"),
    updated_at: safeNumber(raw.updatedAt, "repository updated timestamp"),
    moderation_status: statusName(moderation),
    forked_from: raw.forkedFrom === zeroHash ? null : raw.forkedFrom,
    suite_version: binding.version,
  };
}

async function cached<T>(key: string, load: () => Promise<T>): Promise<T> {
  const hit = queryCache.get(key);
  if (hit && Date.now() - hit.timestamp < CACHE_TTL_MS) return hit.value as T;
  const value = await load();
  queryCache.set(key, { value, timestamp: Date.now() });
  return value;
}

export function clearQueryCache(): void {
  queryCache.clear();
}

async function resolveRepoSingleDir(cfg: AppConfig, owner: string, repo: string): Promise<ResolvedRepo> {
  const requestedOwner = toEvmAddress(owner);
  return cached(`repo:${cfg.profile}:${cfg.suiteDirectory}:${requestedOwner}:${repo}`, async () => {
    const binding = await verifySuite(cfg);
    let result: unknown;
    try {
      result = await readModule(cfg, "core", "resolveRepository", [requestedOwner, repo], binding);
    } catch (error) {
      if (error instanceof Error && error.message.startsWith("LocatorNotFound")) {
        throw new EVMLocatorNotFoundError(toInjectiveAddress(requestedOwner), repo);
      }
      throw error;
    }
    const [raw, canonical] = result as [RawRepository, boolean];
    const info = await mapRepository(cfg, raw, binding);
    return {
      repoId: raw.id,
      requested: { owner: toInjectiveAddress(requestedOwner), name: repo },
      canonical: { owner: info.owner, name: raw.name },
      isCanonical: canonical,
      info,
      suiteDirectory: cfg.suiteDirectory,
    };
  });
}

export async function repoInfo(cfg: AppConfig, owner: string, repo: string): Promise<RepoInfo> {
  return (await resolveRepo(cfg, owner, repo)).info;
}

export async function repoInfoById(
  cfg: AppConfig, repoId: Hex, binding?: SuiteBinding,
): Promise<RepoInfo> {
  const suite = binding ?? await verifySuite(cfg);
  const raw = await readModule(cfg, "core", "getRepository", [repoId], suite) as RawRepository;
  return mapRepository(cfg, raw, suite);
}

async function listReposSingleDir(cfg: AppConfig, owner: string, includeInactive = false): Promise<RepoInfo[]> {
  const address = toEvmAddress(owner);
  const binding = await verifySuite(cfg);
  const repositories: RawRepository[] = [];
  let cursor = 0n;
  while (true) {
    const [page, next] = await readModule(
      cfg, "core", "listRepositoriesPage", [address, cursor, PAGE_SIZE], binding,
    ) as [RawRepository[], bigint];
    repositories.push(...page);
    if (next === cursor && page.length !== 0) throw new Error("repository page cursor did not advance");
    if (page.length < Number(PAGE_SIZE)) break;
    cursor = next;
  }
  const mapped = await Promise.all(repositories.map((repository) => mapRepository(cfg, repository, binding)));
  return includeInactive ? mapped : mapped.filter((repository) => repository.moderation_status === "active");
}

// 鈹€鈹€ Multi-directory helpers (comma-separated SuiteDirectory support) 鈹€鈹€

export async function resolveRepo(
  cfg: AppConfig,
  owner: string,
  repo: string,
  options?: { suiteVersion?: bigint },
): Promise<ResolvedRepo> {
  const dirs = parseSuiteDirectories(cfg.suiteDirectory);
  if (dirs.length <= 1) return resolveRepoSingleDir(cfg, owner, repo);
  let firstHit: ResolvedRepo | null = null;
  let lastErr: unknown;
  for (const dir of dirs) {
    try {
      const resolved = await resolveRepoSingleDir({ ...cfg, suiteDirectory: dir }, owner, repo);
      if (firstHit == null) firstHit = resolved;
      // With a requested suite version, keep walking until that generation's
      // copy resolves; without one, the first directory hit wins as before.
      if (options?.suiteVersion != null && resolved.info.suite_version !== options.suiteVersion) {
        continue;
      }
      return resolved;
    } catch (e) {
      lastErr = e;
      if (!(e instanceof EVMLocatorNotFoundError)) throw e;
    }
  }
  if (firstHit != null) return firstHit;
  throw lastErr;
}

// A same-named repository exists independently in each suite generation;
// the dedupe key must include the suite version or the v4 copy (first in the
// configured directory order) would silently hide the v3 original.
export async function listRepos(cfg: AppConfig, owner: string, includeInactive = false): Promise<RepoInfo[]> {
  const dirs = parseSuiteDirectories(cfg.suiteDirectory);
  if (dirs.length <= 1) return listReposSingleDir(cfg, owner, includeInactive);
  const results = await Promise.allSettled(dirs.map((dir) => listReposSingleDir({ ...cfg, suiteDirectory: dir }, owner, includeInactive)));
  const seen = new Set<string>();
  const merged: RepoInfo[] = [];
  for (const r of results) {
    if (r.status !== "fulfilled") continue;
    for (const repo of r.value) {
      const key = `${repo.owner}/${repo.name}/v${repo.suite_version ?? "?"}`;
      if (!seen.has(key)) { seen.add(key); merged.push(repo); }
    }
  }
  merged.sort((a, b) => b.updated_at - a.updated_at);
  return merged;
}
export async function listRefs(cfg: AppConfig, owner: string, repo: string): Promise<RefInfo[]> {
  const binding = await verifySuite(cfg);
  if (refShapeForVersion(binding.version) === "manifest-commitment") return listSuccessorRefs(cfg, owner, repo);
  const resolved = await resolveRepo(cfg, owner, repo);
  const output: RefInfo[] = [];
  let cursor = 0n;
  while (true) {
    const [names, refs, next] = await readModule(
      cfg, "core", "listRefsPage", [resolved.repoId, cursor, PAGE_SIZE], binding,
    ) as [string[], RawRef[], bigint];
    if (names.length !== refs.length) throw new Error("repository ref page has mismatched names and records");
    output.push(...refs.map((ref, index) => ({
      ref_name: names[index],
      commit_sha: ref.commitSha,
      pack_uris: [...ref.packUris],
      updated_at: safeNumber(ref.updatedAt, "ref updated timestamp"),
      updated_by: toInjectiveAddress(ref.updatedBy),
    })));
    if (next === cursor && refs.length !== 0) throw new Error("ref page cursor did not advance");
    if (refs.length < Number(PAGE_SIZE)) break;
    cursor = next;
  }
  return output;
}

export async function resolveRef(
  cfg: AppConfig, owner: string, repo: string, refName: string,
): Promise<{ ref_name: string; commit_sha: string; pack_uris: string[] }> {
  const binding = await verifySuite(cfg);
  if (refShapeForVersion(binding.version) === "manifest-commitment") {
    const { ref } = await resolveSuccessorRefCommit(cfg, owner, repo, refName);
    return { ref_name: ref.ref_name, commit_sha: ref.commit_sha, pack_uris: [] };
  }
  const resolved = await resolveRepo(cfg, owner, repo);
  const raw = await readModule(cfg, "core", "getRef", [resolved.repoId, refName]) as RawRef;
  return { ref_name: refName, commit_sha: raw.commitSha, pack_uris: [...raw.packUris] };
}

export async function listCollaborators(
  cfg: AppConfig, owner: string, repo: string,
): Promise<CollaboratorInfo[]> {
  const resolved = await resolveRepo(cfg, owner, repo);
  const binding = await verifySuite(cfg);
  const output: CollaboratorInfo[] = [];
  let cursor = 0n;
  while (true) {
    const [accounts, roles, next] = await readModule(
      cfg, "core", "listCollaboratorsPage", [resolved.repoId, cursor, PAGE_SIZE], binding,
    ) as [Address[], number[], bigint];
    if (accounts.length !== roles.length) throw new Error("collaborator page has mismatched accounts and roles");
    output.push(...accounts.map((address, index) => {
      const role: CollaboratorInfo["role"] | undefined = Number(roles[index]) === 1
        ? "maintainer"
        : Number(roles[index]) === 2 ? "reader" : undefined;
      if (!role) throw new Error(`invalid collaborator role ${String(roles[index])}`);
      return { address: toInjectiveAddress(address), role };
    }));
    if (next === cursor && accounts.length !== 0) throw new Error("collaborator page cursor did not advance");
    if (accounts.length < Number(PAGE_SIZE)) break;
    cursor = next;
  }
  return output;
}

export async function pendingOwnershipTransferWithEvm(
  cfg: AppConfig, repoId: Hex,
): Promise<PendingOwnershipTransfer | null> {
  const raw = await readModule(cfg, "core", "pendingOwnershipTransfer", [repoId]) as {
    newOwner: Address; executeAfter: bigint; expiresAt: bigint;
  };
  if (raw.newOwner === zeroAddress) return null;
  return {
    newOwner: toInjectiveAddress(raw.newOwner),
    executeAfter: safeNumber(raw.executeAfter, "ownership transfer execute timestamp"),
    expiresAt: safeNumber(raw.expiresAt, "ownership transfer expiry timestamp"),
  };
}

export function ownershipTransferCapabilities(
  pending: PendingOwnershipTransfer | null,
  account: string,
  currentOwner: string,
): OwnershipTransferCapabilities {
  if (!pending) return { canCancel: false, canAccept: false, canReject: false, canExpire: false };
  return {
    canCancel: sameAddress(account, currentOwner),
    canAccept: sameAddress(account, pending.newOwner),
    canReject: sameAddress(account, pending.newOwner),
    canExpire: account.length > 0,
  };
}

async function transfer(
  provider: Eip1193, cfg: AppConfig, functionName: string, repoId: Hex, args: readonly unknown[] = [],
): Promise<Hex> {
  const hash = await writeModule(provider, cfg, "core", functionName, [repoId, ...args]);
  clearQueryCache();
  return hash;
}

export function beginOwnershipTransferWithEvm(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, newOwner: string,
): Promise<Hex> {
  return transfer(provider, cfg, "beginOwnershipTransfer", repoId, [toEvmAddress(newOwner)]);
}

export function cancelOwnershipTransferWithEvm(provider: Eip1193, cfg: AppConfig, repoId: Hex): Promise<Hex> {
  return transfer(provider, cfg, "cancelOwnershipTransfer", repoId);
}

export function rejectOwnershipTransferWithEvm(provider: Eip1193, cfg: AppConfig, repoId: Hex): Promise<Hex> {
  // The target's rejection is the same on-chain cancellation operation as an
  // owner cancellation. RepositoryCore authorizes both parties.
  return transfer(provider, cfg, "cancelOwnershipTransfer", repoId);
}

export function expireOwnershipTransferWithEvm(provider: Eip1193, cfg: AppConfig, repoId: Hex): Promise<Hex> {
  return transfer(provider, cfg, "expireOwnershipTransfer", repoId);
}

export function acceptOwnershipWithEvm(provider: Eip1193, cfg: AppConfig, repoId: Hex): Promise<Hex> {
  return transfer(provider, cfg, "acceptOwnershipTransfer", repoId);
}

export async function updateRepoInfoWithEvm(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, patch: RepoInfoPatch, expectedAccount?: string,
): Promise<Hex> {
  if (patch.description === undefined && patch.defaultBranch === undefined) {
    throw new Error("repository metadata patch is empty");
  }
  const binding = await verifySuite(cfg, true);
  const abi = coreWriteAbi(binding.version);
  const writeConfig = { ...cfg, suiteDirectory: binding.directory };
  const hash = await writeVerifiedModule(
    provider, writeConfig, "core", abi, "updateMetadata", [
      repoId,
      patch.description !== undefined,
      patch.description ?? "",
      patch.defaultBranch !== undefined,
      patch.defaultBranch ?? "",
    ], undefined, expectedAccount, binding.version,
  );
  clearQueryCache();
  return hash;
}


// ---- Storage-neutral successor (suiteVersion 4) ref reads ----
// Refs carry an on-chain manifest commitment; the commit OID is learned from
// the verified manifest, never from a guessed field.

export interface SuccessorCommitment {
  manifest_digest: string;
  manifest_size: number;
  bootstrap_locator: string;
  revision: number;
  repo_id: string;
  suite_directory: string;
}

interface RawSuccessorRef {
  manifestDigest: string;
  manifestSize: bigint;
  bootstrapLocator: string;
  revision: bigint;
  updatedAt: bigint;
  updatedBy: string;
  exists: boolean;
}

export function successorCommitment(
  raw: RawSuccessorRef,
  repoId: string,
  suiteDirectory: string,
): SuccessorCommitment {
  return {
    // Strip the 0x prefix: PackManifest commitment digests are bare 64-hex.
    manifest_digest: (raw.manifestDigest ?? "").toLowerCase().replace(/^0x/, ""),
    manifest_size: Number(raw.manifestSize),
    bootstrap_locator: raw.bootstrapLocator,
    revision: Number(raw.revision),
    repo_id: repoId.toLowerCase(),
    suite_directory: suiteDirectory.toLowerCase(),
  };
}

/** Resolve a successor ref to a verified manifest and its commit OID. */
export async function resolveSuccessorRefCommit(
  cfg: AppConfig,
  owner: string,
  repo: string,
  refName: string,
): Promise<{ ref: RefInfo; manifest: import("./packmanifest").PackManifest }> {
  const resolved = await resolveRepo(cfg, owner, repo);
  const binding = await verifySuite(cfg);
  if (refShapeForVersion(binding.version) !== "manifest-commitment") throw new Error("suite refs do not use the manifest-commitment shape");
  const raw = await rawModuleRead(cfg, binding.modules.core, successorCoreAbi, "getRef", [resolved.repoId, refName], binding.blockTag) as RawSuccessorRef;
  if (!raw.exists) throw new Error("ref not found");
  const commitment = successorCommitment(raw, resolved.repoId, binding.directory);
  const manifest = await fetchVerifiedManifest(
    { chainId: String(cfg.evmChainId), suiteDirectory: commitment.suite_directory, repoId: commitment.repo_id, refName },
    { sha256: commitment.manifest_digest, size: String(commitment.manifest_size), bootstrapLocator: commitment.bootstrap_locator },
  );
  const ref: RefInfo = {
    ref_name: refName,
    commit_sha: manifest.commit.oid,
    pack_uris: [],
    updated_at: Number(raw.updatedAt),
    updated_by: toInjectiveAddress(raw.updatedBy),
    commitment,
  };
  return { ref, manifest };
}

/** List refs of a successor suite; commit OIDs come from verified manifests. */
export async function listSuccessorRefs(cfg: AppConfig, owner: string, repo: string): Promise<RefInfo[]> {
  const resolved = await resolveRepo(cfg, owner, repo);
  const binding = await verifySuite(cfg);
  if (refShapeForVersion(binding.version) !== "manifest-commitment") throw new Error("suite refs do not use the manifest-commitment shape");
  const output: RefInfo[] = [];
  let cursor = 0n;
  while (true) {
    const [names, refs, next] = await rawModuleRead(
      cfg, binding.modules.core, successorCoreAbi, "listRefsPage", [resolved.repoId, cursor, PAGE_SIZE], binding.blockTag,
    ) as [string[], RawSuccessorRef[], bigint];
    if (names.length !== refs.length) throw new Error("successor ref page has mismatched names and records");
    for (let index = 0; index < names.length; index++) {
      const raw = refs[index];
      if (!raw.exists) continue;
      const commitment = successorCommitment(raw, resolved.repoId, binding.directory);
      let commitSha = "";
      try {
        const manifest = await fetchVerifiedManifest(
          { chainId: String(cfg.evmChainId), suiteDirectory: commitment.suite_directory, repoId: commitment.repo_id, refName: names[index] },
          { sha256: commitment.manifest_digest, size: String(commitment.manifest_size), bootstrapLocator: commitment.bootstrap_locator },
        );
        commitSha = manifest.commit.oid;
      } catch {
        // keep the commitment advertised; the commit column resolves lazily
      }
      output.push({
        ref_name: names[index],
        commit_sha: commitSha,
        pack_uris: [],
        updated_at: Number(raw.updatedAt),
        updated_by: toInjectiveAddress(raw.updatedBy),
        commitment,
      });
    }
    if (next === cursor && refs.length !== 0) throw new Error("ref page cursor did not advance");
    if (refs.length < Number(PAGE_SIZE)) break;
    cursor = next;
  }
  return output;
}
