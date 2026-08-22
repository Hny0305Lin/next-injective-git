// Canonical moved to ./chain/registry.ts — this file remains for compatibility
import { zeroAddress, zeroHash, type Address, type Hex } from "viem";
import { sameAddress, toEvmAddress, toInjectiveAddress } from "./address";
import { readModule, verifySuite, writeModule, type Eip1193, type SuiteBinding } from "./transport";
import type { AppConfig } from "./profile";

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
}

export interface RefInfo {
  ref_name: string;
  commit_sha: string;
  pack_uris: string[];
  updated_at: number;
  updated_by: string;
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
}

export interface RepoInfoPatch {
  description?: string;
  defaultBranch?: string;
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

export async function resolveRepo(cfg: AppConfig, owner: string, repo: string): Promise<ResolvedRepo> {
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

export async function listRepos(cfg: AppConfig, owner: string, includeInactive = false): Promise<RepoInfo[]> {
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

export async function listRefs(cfg: AppConfig, owner: string, repo: string): Promise<RefInfo[]> {
  const resolved = await resolveRepo(cfg, owner, repo);
  const binding = await verifySuite(cfg);
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
  provider: Eip1193, cfg: AppConfig, repoId: Hex, patch: RepoInfoPatch,
): Promise<Hex> {
  if (patch.description === undefined && patch.defaultBranch === undefined) {
    throw new Error("repository metadata patch is empty");
  }
  const hash = await writeModule(provider, cfg, "core", "updateMetadata", [
    repoId,
    patch.description !== undefined,
    patch.description ?? "",
    patch.defaultBranch !== undefined,
    patch.defaultBranch ?? "",
  ]);
  clearQueryCache();
  return hash;
}
