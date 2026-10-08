/**
 * Client-side repository index for the global search box.
 *
 * The suite contracts expose no global repository enumeration (only per-owner
 * `listRepositoriesPage`), and the public RPC serves `eth_getLogs` only for a
 * recent window, so the index combines two on-chain-only sources:
 *
 *  1. Event scan: RepositoryCreated / OwnershipTransferred /
 *     RepositoryStatusSet logs of the *verified* suite modules, newest first.
 *  2. Absorbed listings: repositories enumerated by contract reads when an
 *     owner page is opened or a wallet connects (works for any chain age).
 *
 * The index is navigation-only: every repository opened from it is still
 * resolved through verifySuite + contract reads, never through the index.
 */

import { decodeEventLog, encodeEventTopics, type Address, type Hex } from "viem";
import { coreAbi, moderationAbi } from "./abis";
import { toInjectiveAddress } from "./address";
import { EVM_LOG_RANGE_LIMIT, logsInSpan, quantity, type RpcLog } from "./activity";
import { networkProfile, parseSuiteDirectories, type AppConfig } from "./profile";
import { repoInfoById, resolveRepo } from "./registry";
import { rpcRequest, verifySuite, type SuiteBinding } from "./transport";

/** Head window size (blocks) scanned first for immediately usable results. */
export const REPO_INDEX_RECENT_WINDOW = 100_000;
/** Blocks withheld from the head cursor so reorged logs are rescanned. */
export const REPO_INDEX_CONFIRMATIONS = 64;
/** Overlap rescanned on incremental refreshes (larger than confirmations). */
export const REPO_INDEX_REORG_OVERLAP = 96;
/** How deep into history the backward walk is allowed to go. */
export const REPO_INDEX_MAX_HISTORY = 8_000_000;
/** Chunk size (blocks) per eth_getLogs call; matches the RPC range limit. */
const SCAN_CHUNK = EVM_LOG_RANGE_LIMIT;
/** Chunks processed per idle tick. */
const TICK_CHUNKS = 16;
/** Chunks queried concurrently inside one scan pass. */
const SCAN_CONCURRENCY = 8;
const TICK_MS = 150;
const CHUNK_FAILURE_LIMIT = 3;
/** Consecutive all-empty walk rounds before history scanning pauses. */
const EMPTY_ROUND_LIMIT = 4;
/** Hard caps so the persisted index stays bounded. */
const MAX_ENTRIES = 50_000;
const MAX_STORED_BYTES = 4_000_000;
const STORE_VERSION = 1;

export interface RepoIndexStatus {
  building: boolean;
  phase: "initial" | "history" | "idle" | "error";
  entries: number;
  error?: string;
}

export interface RepoIndexEntry {
  /** Content-addressed repository id; null when only enumerated by contract. */
  repoId: Hex | null;
  /** Lowercase SuiteDirectory that emitted the indexed events ("" when absorbed). */
  suiteDirectory: string;
  /** Suite protocol version (3 = IPFS, 4 = BYOS). */
  suiteVersion: number;
  /** Owner at the newest applied event, as an inj1 address. */
  owner: string;
  name: string;
  /** Moderation status (0 active, 1 frozen, 2 delisted). */
  status: number;
  /** Position of the newest applied event; older events are ignored. */
  blockNumber: number;
  logIndex: number;
  /** Block of the creation (or adoption) event; newer rank first. */
  createdBlock: number;
}

export type RepoEvent =
  | { kind: "created"; repoId: Hex; owner: string; name: string; blockNumber: number; logIndex: number }
  | { kind: "owner"; repoId: Hex; owner: string; name: string; blockNumber: number; logIndex: number }
  | { kind: "status"; repoId: Hex; status: number; blockNumber: number; logIndex: number };

/** Mutable shard shape consumed by the pure event fold (unit-testable). */
export interface RepoShardState {
  suiteDirectory: string;
  suiteVersion: number;
  entries: Map<string, RepoIndexEntry>;
}

interface RepoShard extends RepoShardState {
  directory: string;
  /** Highest confirmed block whose head range was scanned. */
  cursor: number;
  /** Next descending chunk end for the history walk; below floor means done. */
  walkNext: number;
  /** Lower bound of the history walk (suite deployment era or chain start). */
  walkFloor: number;
}

export interface StoredShard {
  directory: string;
  suiteVersion: number;
  cursor: number;
  walkNext: number;
  walkFloor: number;
  entries: RepoIndexEntry[];
}

export interface SerializableIndex {
  shards: {
    directory: string;
    suiteVersion: number;
    cursor: number;
    walkNext: number;
    walkFloor: number;
    entries: Map<string, RepoIndexEntry>;
  }[];
  absorbed: Map<string, RepoIndexEntry>;
}

let cachedTopics: readonly (readonly Hex[])[] | null = null;

/** topic0 filters for the three indexed events, computed from checked ABIs. */
function repoEventTopics(): readonly (readonly Hex[])[] {
  if (!cachedTopics) {
    cachedTopics = [[
      encodeEventTopics({ abi: coreAbi, eventName: "RepositoryCreated" })[0],
      encodeEventTopics({ abi: coreAbi, eventName: "OwnershipTransferred" })[0],
      encodeEventTopics({ abi: moderationAbi, eventName: "RepositoryStatusSet" })[0],
    ]];
  }
  return cachedTopics;
}

/** Decode one suite log into an index event, or null for unrelated events. */
export function decodeRepoEvent(log: RpcLog): RepoEvent | null {
  if (log.topics.length === 0) return null;
  for (const abi of [coreAbi, moderationAbi]) {
    try {
      const decoded = decodeEventLog({ abi, topics: log.topics as [Hex, ...Hex[]], data: log.data, strict: false });
      const eventName = String(decoded.eventName ?? "");
      const args = (decoded.args ?? {}) as Record<string, unknown>;
      const blockNumber = Number(quantity(log.blockNumber));
      const logIndex = Number(quantity(log.logIndex));
      const repoId = String(args.repoId ?? "").toLowerCase() as Hex;
      if (!/^0x[0-9a-f]{64}$/.test(repoId)) return null;
      if (eventName === "RepositoryCreated") {
        return { kind: "created", repoId, owner: toInjectiveAddress(String(args.owner)), name: String(args.name ?? ""), blockNumber, logIndex };
      }
      if (eventName === "OwnershipTransferred") {
        return { kind: "owner", repoId, owner: toInjectiveAddress(String(args.newOwner)), name: String(args.name ?? ""), blockNumber, logIndex };
      }
      if (eventName === "RepositoryStatusSet") {
        return { kind: "status", repoId, status: Number(args.status ?? 0), blockNumber, logIndex };
      }
      return null;
    } catch {
      // The log belongs to the other module ABI.
    }
  }
  return null;
}


/** Absorbed entries are keyed per suite generation so same-name v3/v4 repositories coexist. */
function absorbedKey(owner: string, name: string, suiteVersion: number): string {
  return `${owner.toLowerCase()}/${name.toLowerCase()}/v${suiteVersion}`;
}

function isAhead(entry: RepoIndexEntry, blockNumber: number, logIndex: number): boolean {
  return blockNumber > entry.blockNumber || (blockNumber === entry.blockNumber && logIndex > entry.logIndex);
}

/**
 * Fold one event into a shard. Absorbed (repoId-less) entries for the same
 * owner/name are upgraded in place; stale events and orphan status events are
 * ignored.
 */
export function applyRepoEvent(shard: RepoShardState, event: RepoEvent): void {
  if (event.kind === "status") {
    const existing = shard.entries.get(event.repoId);
    if (!existing || !isAhead(existing, event.blockNumber, event.logIndex)) return;
    if (event.status >= 0 && event.status <= 2) existing.status = event.status;
    existing.blockNumber = event.blockNumber;
    existing.logIndex = event.logIndex;
    return;
  }
  let targetKey: string = event.repoId;
  let existing = shard.entries.get(targetKey);
  if (!existing && event.name) {
    for (const [key, entry] of shard.entries) {
      if (!entry.repoId && entry.owner === event.owner && entry.name.toLowerCase() === event.name.toLowerCase()) {
        shard.entries.delete(key);
        targetKey = event.repoId;
        existing = entry;
        break;
      }
    }
  }
  if (existing && !isAhead(existing, event.blockNumber, event.logIndex)) return;
  const name = event.name || existing?.name || "";
  if (!name) return;
  shard.entries.set(targetKey, {
    repoId: event.repoId,
    suiteDirectory: shard.suiteDirectory,
    suiteVersion: shard.suiteVersion,
    owner: event.owner,
    name,
    status: existing?.status ?? 0,
    blockNumber: event.blockNumber,
    logIndex: event.logIndex,
    createdBlock: existing?.createdBlock ?? event.blockNumber,
  });
}

/** A contract-enumerated repository listing (owner page or connected wallet). */
export interface AbsorbedRepo {
  owner: string;
  name: string;
  moderation: string;
  suiteVersion?: number;
}

/** Merge contract-enumerated repositories into the absorbed layer. */
export function absorbRepos(absorbed: Map<string, RepoIndexEntry>, repos: readonly AbsorbedRepo[], suiteVersion: number): number {
  let added = 0;
  for (const repo of repos) {
    if (!repo.owner || !repo.name) continue;
    const key = absorbedKey(repo.owner, repo.name, repo.suiteVersion ?? suiteVersion);
    const status = repo.moderation === "frozen" ? 1 : repo.moderation === "delisted" ? 2 : 0;
    const existing = absorbed.get(key);
    if (existing) {
      existing.status = status;
      continue;
    }
    absorbed.set(key, {
      repoId: null,
      suiteDirectory: "",
      suiteVersion: repo.suiteVersion ?? suiteVersion,
      owner: repo.owner,
      name: repo.name,
      status,
      blockNumber: -1,
      logIndex: -1,
      createdBlock: 0,
    });
    added += 1;
  }
  return added;
}

/** Rank and filter index entries for the search box; exact > prefix > substring. */
export function searchRepoEntries(entries: readonly RepoIndexEntry[], rawQuery: string, limit = 20): RepoIndexEntry[] {
  const query = rawQuery.trim().toLowerCase();
  if (query.length < 2) return [];
  const slash = query.indexOf("/");
  const ownerHint = slash >= 0 ? query.slice(0, slash) : "";
  const nameQuery = slash >= 0 ? query.slice(slash + 1) : query;
  if (!nameQuery) return [];
  // A bare address query also matches repositories by exact owner.
  let ownerExact: string | null = null;
  if (slash < 0) {
    try {
      ownerExact = toInjectiveAddress(query);
    } catch {
      ownerExact = null;
    }
  }
  const scored: { entry: RepoIndexEntry; score: number }[] = [];
  for (const entry of entries) {
    if (entry.status === 2) continue; // delisted repositories stay out of search
    if (ownerHint && !entry.owner.toLowerCase().includes(ownerHint)) continue;
    const name = entry.name.toLowerCase();
    let score = -1;
    if (name === nameQuery) score = 0;
    else if (name.startsWith(nameQuery)) score = 1;
    else if (name.includes(nameQuery)) score = 2;
    if (score < 0) {
      if (ownerExact && entry.owner.toLowerCase() === ownerExact.toLowerCase()) {
        score = 3;
      } else {
        continue;
      }
    }
    scored.push({ entry, score });
  }
  scored.sort((left, right) =>
    left.score - right.score ||
    right.entry.createdBlock - left.entry.createdBlock ||
    left.entry.name.localeCompare(right.entry.name) ||
    left.entry.suiteDirectory.localeCompare(right.entry.suiteDirectory)
  );
  return scored.slice(0, limit).map((item) => item.entry);
}

export function repoIndexStorageKey(chainId: number): string {
  return `igit.repo-index.v${STORE_VERSION}.${chainId}`;
}

export function serializeIndex(index: SerializableIndex): string {
  return JSON.stringify({
    version: STORE_VERSION,
    shards: index.shards.map((shard) => ({
      directory: shard.directory,
      suiteVersion: shard.suiteVersion,
      cursor: shard.cursor,
      walkNext: shard.walkNext,
      walkFloor: shard.walkFloor,
      entries: [...shard.entries.values()],
    })),
    absorbed: [...index.absorbed.values()],
  });
}

function isStoredEntry(value: unknown): value is RepoIndexEntry {
  const entry = value as Partial<RepoIndexEntry>;
  return (
    (entry.repoId === null || (typeof entry.repoId === "string" && /^0x[0-9a-f]{64}$/.test(entry.repoId))) &&
    typeof entry.owner === "string" && entry.owner.length > 0 &&
    typeof entry.name === "string" && entry.name.length > 0 &&
    typeof entry.status === "number" && Number.isFinite(entry.status) &&
    typeof entry.blockNumber === "number" && typeof entry.logIndex === "number" &&
    typeof entry.createdBlock === "number" &&
    typeof entry.suiteDirectory === "string" && typeof entry.suiteVersion === "number"
  );
}

export interface ParsedIndex {
  shards: StoredShard[];
  absorbed: RepoIndexEntry[];
}

/** Parse and validate persisted index JSON; null when unusable. */
export function parseIndex(raw: string | null): ParsedIndex | null {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as { version?: unknown; shards?: unknown; absorbed?: unknown };
    if (parsed.version !== STORE_VERSION || !Array.isArray(parsed.shards)) return null;
    const shards: StoredShard[] = [];
    for (const item of parsed.shards) {
      const shard = item as Partial<StoredShard>;
      if (
        typeof shard.directory !== "string" || !/^0x[0-9a-f]{40}$/.test(shard.directory) ||
        typeof shard.suiteVersion !== "number" ||
        typeof shard.cursor !== "number" || !Number.isFinite(shard.cursor) || shard.cursor < 0 ||
        typeof shard.walkNext !== "number" || !Number.isFinite(shard.walkNext) || shard.walkNext < -1 ||
        typeof shard.walkFloor !== "number" || !Number.isFinite(shard.walkFloor) || shard.walkFloor < 0 ||
        !Array.isArray(shard.entries)
      ) return null;
      const directory = shard.directory;
      const suiteVersion = shard.suiteVersion;
      shards.push({
        directory,
        suiteVersion,
        cursor: shard.cursor,
        walkNext: shard.walkNext,
        walkFloor: shard.walkFloor,
        entries: shard.entries.filter(isStoredEntry).map((entry) => ({ ...entry, suiteDirectory: directory, suiteVersion })),
      });
    }
    const absorbed = Array.isArray(parsed.absorbed)
      ? parsed.absorbed.filter(isStoredEntry).map((entry) => ({ ...entry, suiteDirectory: "" }))
      : [];
    return { shards, absorbed };
  } catch {
    return null;
  }
}

function trimEntries(all: RepoIndexEntry[], shardMaps: Map<string, RepoIndexEntry>[]): RepoIndexEntry[] {
  all.sort((left, right) => right.createdBlock - left.createdBlock);
  const keep = all.slice(0, MAX_ENTRIES);
  const keepKeys = new Set(keep.map((entry) => `${entry.suiteDirectory}:${entry.repoId ?? absorbedKey(entry.owner, entry.name, entry.suiteVersion)}`));
  for (const map of shardMaps) {
    for (const key of [...map.keys()]) {
      const entry = map.get(key);
      if (!entry || !keepKeys.has(`${entry.suiteDirectory}:${entry.repoId ?? absorbedKey(entry.owner, entry.name, entry.suiteVersion)}`)) {
        map.delete(key);
      }
    }
  }
  return keep;
}

export interface ExplorerSourceStatus {
  state: "idle" | "loading" | "ok" | "unavailable";
  events?: number;
  error?: string;
}

function explorerQuantity(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    if (/^0x[0-9a-fA-F]+$/.test(value)) return Number(BigInt(value));
    if (/^\d+$/.test(value)) return Number(value);
  }
  throw new Error(`explorer log has an invalid quantity: ${String(value)}`);
}

export function toExplorerRpcLogForTest(item: Record<string, unknown>): RpcLog | null {
  return toExplorerRpcLog(item);
}

function toExplorerRpcLog(item: Record<string, unknown>): RpcLog | null {
  const address = String(item.address ?? "");
  const topics = item.topics;
  const data = String(item.data ?? "0x");
  const transactionHash = String(item.transactionHash ?? "");
  if (!/^0x[0-9a-fA-F]{40}$/.test(address) || !Array.isArray(topics) || topics.length === 0) return null;
  try {
    return {
      address: address as Address,
      topics: topics as Hex[],
      data: data as Hex,
      blockNumber: `0x${explorerQuantity(item.blockNumber).toString(16)}`,
      transactionHash: transactionHash as Hex,
      logIndex: `0x${explorerQuantity(item.logIndex).toString(16)}`,
    };
  } catch {
    // Malformed explorer quantities are skipped, not fatal.
    return null;
  }
}

/** Etherscan-style explorer URL for one topic0 on one module address. */
export function explorerLogsUrl(explorerBase: string, address: string, topic0: Hex): string {
  const base = explorerBase.replace(/\/+$/, "");
  return `${base}/api?module=logs&action=getLogs&address=${address}&fromBlock=0&toBlock=latest&topic0=${topic0}`;
}

/**
 * Best-effort pull of repository events from the configured public block
 * explorer. The public RPC silently prunes old eth_getLogs ranges, so the
 * explorer (which indexes full history) fills that gap for search hints.
 * Throws on network/HTTP failures; the caller treats that as "unavailable".
 */
export async function fetchExplorerRepoEvents(
  explorerBase: string,
  coreAddress: string,
  moderationAddress: string,
): Promise<{ events: RepoEvent[]; logs: RpcLog[] }> {
  const topics = repoEventTopics()[0];
  const createdTopic = topics[0];
  const transferTopic = topics[1];
  const statusTopic = topics[2];
  const urls = [
    explorerLogsUrl(explorerBase, coreAddress, createdTopic),
    explorerLogsUrl(explorerBase, coreAddress, transferTopic),
    // Moderation events come from the moderation module; without them the
    // explorer-sourced entries would show stale governance status.
    explorerLogsUrl(explorerBase, moderationAddress, statusTopic),
  ];
  const logs: RpcLog[] = [];
  for (const url of urls) {
    // Bound the external call; a hung explorer must not stall the search page.
    const response = await fetch(url, { headers: { accept: "application/json" }, signal: AbortSignal.timeout(12_000) });
    if (!response.ok) throw new Error(`explorer logs HTTP ${response.status}`);
    const body = (await response.json()) as { status?: unknown; message?: unknown; result?: unknown };
    const status = String(body.status ?? "");
    if (status !== "1") {
      const message = String(body.message ?? "");
      if (/no (records|logs)/i.test(message)) continue;
      throw new Error(`explorer logs rejected: ${message || status}`);
    }
    if (!Array.isArray(body.result)) continue;
    for (const item of body.result) {
      const log = toExplorerRpcLog(item as Record<string, unknown>);
      if (log) logs.push(log);
    }
  }
  const events: RepoEvent[] = [];
  for (const log of logs) {
    const event = decodeRepoEvent(log);
    if (event) events.push(event);
  }
  return { events, logs };
}

/** Order events by on-chain position so status events apply after creation. */
export function sortRepoEvents(events: readonly RepoEvent[]): RepoEvent[] {
  return [...events].sort((left, right) => left.blockNumber - right.blockNumber || left.logIndex - right.logIndex);
}

function browserStorage(): Storage | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage;
  } catch {
    return null;
  }
}

/**
 * Merge event-sourced and absorbed entries for search. Absorbed entries come
 * from a fresh effectiveStatus contract read, so their governance status
 * outranks the event replay, which may have missed pruned status logs.
 */
export function mergeEntriesForSearch(
  shardEntries: readonly RepoIndexEntry[],
  absorbedEntries: readonly RepoIndexEntry[],
): RepoIndexEntry[] {
  const absorbedByKey = new Map(absorbedEntries.map((entry) => [absorbedKey(entry.owner, entry.name, entry.suiteVersion), entry]));
  const merged: RepoIndexEntry[] = [];
  const seen = new Set<string>();
  for (const entry of shardEntries) {
    const key = absorbedKey(entry.owner, entry.name, entry.suiteVersion);
    const absorbed = absorbedByKey.get(key);
    merged.push(absorbed ? { ...entry, status: absorbed.status } : entry);
    seen.add(key);
  }
  for (const entry of absorbedEntries) {
    const key = absorbedKey(entry.owner, entry.name, entry.suiteVersion);
    if (seen.has(key)) continue;
    merged.push(entry);
  }
  return merged;
}

export class RepoIndexer {
  readonly key: string;
  private shards = new Map<string, RepoShard>();
  private absorbed = new Map<string, RepoIndexEntry>();
  private listeners = new Set<(status: RepoIndexStatus) => void>();
  private status: RepoIndexStatus = { building: false, phase: "idle", entries: 0 };
  private started = false;
  private refreshing = false;
  private tickTimer: ReturnType<typeof setTimeout> | null = null;
  private chunkFailures = 0;
  private emptyRounds = 0;
  private historyPaused = false;
  private explorerPromise: Promise<ExplorerSourceStatus> | null = null;

  constructor(private readonly cfg: AppConfig) {
    this.key = `${cfg.evmChainId}|${parseSuiteDirectories(cfg.suiteDirectory).join(",")}`;
    this.load();
  }

  private load(): void {
    const configured = parseSuiteDirectories(this.cfg.suiteDirectory);
    const parsed = parseIndex(browserStorage()?.getItem(repoIndexStorageKey(this.cfg.evmChainId)) ?? null);
    if (!parsed) return;
    for (const stored of parsed.shards) {
      if (!configured.includes(stored.directory)) continue;
      this.shards.set(stored.directory, {
        directory: stored.directory,
        suiteDirectory: stored.directory,
        suiteVersion: stored.suiteVersion,
        cursor: stored.cursor,
        walkNext: stored.walkNext,
        walkFloor: stored.walkFloor,
        entries: new Map(stored.entries.map((entry) => [entry.repoId as string, entry])),
      });
    }
    this.absorbed = new Map(parsed.absorbed.map((entry) => [absorbedKey(entry.owner, entry.name, entry.suiteVersion), entry]));
  }

  private persist(): void {
    const maps = [...this.shards.values()].map((shard) => shard.entries).concat([this.absorbed]);
    const all = maps.flatMap((map) => [...map.values()]);
    if (all.length > MAX_ENTRIES) trimEntries(all, maps);
    let raw = serializeIndex({ shards: [...this.shards.values()], absorbed: this.absorbed });
    if (raw.length > MAX_STORED_BYTES) {
      // Stop growing history; the head window keeps updating on refreshes.
      for (const shard of this.shards.values()) shard.walkNext = -1;
      raw = serializeIndex({ shards: [...this.shards.values()], absorbed: this.absorbed });
    }
    try {
      browserStorage()?.setItem(repoIndexStorageKey(this.cfg.evmChainId), raw);
    } catch {
      // Quota exceeded: keep the index in memory only.
    }
  }

  getStatus(): RepoIndexStatus {
    return this.status;
  }

  get entries(): RepoIndexEntry[] {
    return this.mergedEntries();
  }

  private mergedEntries(): RepoIndexEntry[] {
    const shardEntries: RepoIndexEntry[] = [];
    for (const shard of this.shards.values()) shardEntries.push(...shard.entries.values());
    return mergeEntriesForSearch(shardEntries, [...this.absorbed.values()]);
  }

  /** Merge contract-enumerated repositories (owner page, connected wallet). */
  absorbOwnerRepos(repos: readonly AbsorbedRepo[]): void {
    const added = absorbRepos(this.absorbed, repos, Number(this.statusEntriesDefaultVersion()));
    if (added > 0) {
      this.persist();
      this.setStatus({ ...this.statusLike(), entries: this.countEntries() });
    }
  }

  private statusEntriesDefaultVersion(): bigint {
    const first = this.shards.values().next().value;
    return first ? BigInt(first.suiteVersion) : 4n;
  }

  private statusLike(): { building: boolean; phase: RepoIndexStatus["phase"]; error?: string } {
    return { building: this.status.building, phase: this.status.phase, error: this.status.error };
  }

  /**
   * Pull full-history repository events from the configured public block
   * explorer (read-only, navigation hints only). Idempotent per indexer.
   */
  ensureExplorerIndexed(): Promise<ExplorerSourceStatus> {
    if (this.explorerPromise) return this.explorerPromise;
    this.explorerPromise = (async (): Promise<ExplorerSourceStatus> => {
      const explorer = networkProfile(this.cfg).evmExplorer;
      if (!explorer) return { state: "unavailable", error: "no explorer configured" };
      let applied = 0;
      for (const directory of parseSuiteDirectories(this.cfg.suiteDirectory)) {
        try {
          const binding = await verifySuite({ ...this.cfg, suiteDirectory: directory });
          // The explorer pull may run before the head scan creates the shard;
          // create it on demand so events are never silently dropped.
          let shard = this.shards.get(directory);
          if (!shard) {
            shard = {
              directory,
              suiteDirectory: directory,
              suiteVersion: Number(binding.version),
              cursor: 0,
              walkNext: -1,
              walkFloor: 0,
              entries: new Map(),
            };
            this.shards.set(directory, shard);
          }
          const { events } = await fetchExplorerRepoEvents(explorer, binding.modules.core, binding.modules.moderation);
          for (const event of sortRepoEvents(events)) {
            const before = shard.entries.size;
            applyRepoEvent(shard, event);
            if (shard.entries.size > before) applied += 1;
          }
        } catch (error) {
          console.warn(`repo index: explorer source failed for ${directory}`, error);
          return { state: "unavailable", error: error instanceof Error ? error.message : String(error) };
        }
      }
      this.persist();
      this.setStatus({ building: this.historyRemaining() > 0, phase: this.status.phase === "error" ? "error" : this.historyRemaining() > 0 ? "history" : "idle", entries: this.countEntries() });
      return { state: "ok", events: applied };
    })();
    return this.explorerPromise;
  }

  subscribe(listener: (status: RepoIndexStatus) => void): () => void {
    this.listeners.add(listener);
    listener(this.status);
    if (this.started) this.scheduleNextTick();
    return () => {
      this.listeners.delete(listener);
      if (this.listeners.size === 0 && this.tickTimer) {
        clearTimeout(this.tickTimer);
        this.tickTimer = null;
      }
    };
  }

  private setStatus(next: RepoIndexStatus): void {
    this.status = next;
    for (const listener of [...this.listeners]) listener(next);
  }

  private countEntries(): number {
    return this.mergedEntries().length;
  }

  private historyRemaining(): number {
    if (this.historyPaused) return 0;
    let remaining = 0;
    for (const shard of this.shards.values()) {
      if (shard.walkNext >= shard.walkFloor) remaining += shard.walkNext - shard.walkFloor + 1;
    }
    return remaining;
  }

  ensureStarted(): void {
    if (this.started) return;
    this.started = true;
    void this.refresh();
  }

  private async refresh(): Promise<void> {
    if (this.refreshing) return;
    this.refreshing = true;
    try {
      this.setStatus({ building: true, phase: "initial", entries: this.countEntries() });
      const latest = quantity(await rpcRequest<Hex>(this.cfg, "eth_blockNumber"));
      const safe = latest > BigInt(REPO_INDEX_CONFIRMATIONS) ? latest - BigInt(REPO_INDEX_CONFIRMATIONS) : 0n;
      const recentFrom = latest > BigInt(REPO_INDEX_RECENT_WINDOW) ? latest - BigInt(REPO_INDEX_RECENT_WINDOW) : 0n;
      for (const directory of parseSuiteDirectories(this.cfg.suiteDirectory)) {
        try {
          const binding = await verifySuite({ ...this.cfg, suiteDirectory: directory });
          let shard = this.shards.get(directory);
          if (!shard) {
            shard = {
              directory,
              suiteDirectory: directory,
              suiteVersion: Number(binding.version),
              cursor: 0,
              walkNext: -1,
              walkFloor: 0,
              entries: new Map(),
            };
            this.shards.set(directory, shard);
          }
          if (shard.cursor === 0) {
            await this.scan(shard, binding, recentFrom, safe);
            shard.cursor = Number(safe);
            shard.walkNext = Number(recentFrom) - 1;
            const floor = Number(recentFrom) - REPO_INDEX_MAX_HISTORY;
            shard.walkFloor = floor > 0 ? floor : 0;
          } else {
            const overlap = BigInt(Math.max(0, shard.cursor + 1 - REPO_INDEX_REORG_OVERLAP));
            if (safe >= overlap) {
              await this.scan(shard, binding, overlap, safe);
              shard.cursor = Number(safe);
            }
          }
        } catch (error) {
          // One unavailable directory must not block indexing of the others.
          console.warn(`repo index: refresh failed for ${directory}`, error);
        }
      }
      this.persist();
      const remaining = this.historyRemaining();
      this.setStatus({ building: remaining > 0, phase: remaining > 0 ? "history" : "idle", entries: this.countEntries() });
      this.scheduleNextTick();
    } catch (error) {
      this.setStatus({ building: false, phase: "error", entries: this.countEntries(), error: error instanceof Error ? error.message : String(error) });
    } finally {
      this.refreshing = false;
    }
  }

  /**
   * Scan [from, to] in bounded chunks, a few in parallel. The fold is
   * order-independent (per-entry position guard), so parallel chunks are safe.
   */
  private async scan(shard: RepoShard, binding: SuiteBinding, from: bigint, to: bigint): Promise<void> {
    if (to < from) return;
    const addresses: Address[] = [binding.modules.core, binding.modules.moderation];
    const spans: { from: bigint; to: bigint }[] = [];
    for (let start = from; start <= to; start += BigInt(SCAN_CHUNK)) {
      const end = start + BigInt(SCAN_CHUNK) - 1n > to ? to : start + BigInt(SCAN_CHUNK) - 1n;
      spans.push({ from: start, to: end });
    }
    for (let offset = 0; offset < spans.length; offset += SCAN_CONCURRENCY) {
      const batch = spans.slice(offset, offset + SCAN_CONCURRENCY);
      const batches = await Promise.all(batch.map((span) => logsInSpan(this.cfg, addresses, span.from, span.to, 0, repoEventTopics())));
      const logs = batches.flat().filter((log) => !log.removed);
      logs.sort((left, right) => {
        const block = quantity(left.blockNumber) - quantity(right.blockNumber);
        return block === 0n ? Number(quantity(left.logIndex) - quantity(right.logIndex)) : block > 0n ? 1 : -1;
      });
      for (const log of logs) {
        const event = decodeRepoEvent(log);
        if (event) applyRepoEvent(shard, event);
      }
    }
  }

  private scheduleNextTick(): void {
    if (this.tickTimer || this.listeners.size === 0 || this.historyRemaining() === 0) return;
    this.tickTimer = setTimeout(() => {
      this.tickTimer = null;
      void this.historyTick();
    }, TICK_MS);
  }

  /** Walk history backwards in small chunks, newest first, across shards in parallel. */
  private async historyTick(): Promise<void> {
    const before = this.countEntries();
    await Promise.all([...this.shards.values()].map(async (shard) => {
      const binding = await verifySuite({ ...this.cfg, suiteDirectory: shard.directory }).catch(() => null);
      if (!binding || shard.walkNext < shard.walkFloor) return;
      // One wide scan per round; scan() splits it into RPC-sized chunks and
      // queries several of them concurrently.
      const to = BigInt(shard.walkNext);
      const width = Math.min(TICK_CHUNKS * SCAN_CHUNK, shard.walkNext - shard.walkFloor + 1);
      const from = BigInt(shard.walkNext - width + 1);
      try {
        await this.scan(shard, binding, from, to);
        shard.walkNext = Number(from) - 1;
        this.chunkFailures = 0;
      } catch (error) {
        this.chunkFailures += 1;
        console.warn(`repo index: history chunk failed for ${shard.directory} at ${from}`, error);
        // Pruned or unservable history: stop walking after repeated failures.
        if (this.chunkFailures >= CHUNK_FAILURE_LIMIT) shard.walkNext = -1;
      }
    }));
    this.persist();
    const grew = this.countEntries() > before;
    this.emptyRounds = grew ? 0 : this.emptyRounds + 1;
    // A log-pruned RPC answers old ranges with empty results; stop hammering
    // it after several all-empty rounds and rely on the head window plus the
    // absorbed contract listings. The walk resumes on the next app session.
    if (!grew && this.emptyRounds >= EMPTY_ROUND_LIMIT) this.historyPaused = true;
    const remaining = this.historyRemaining();
    this.setStatus({ building: remaining > 0, phase: remaining > 0 ? "history" : "idle", entries: this.countEntries() });
    if (remaining > 0) this.scheduleNextTick();
  }
}

/**
 * Resolve an index entry to its authoritative on-chain owner/name. The index
 * is navigation-only; this performs the real contract read (or falls back to
 * the indexed values when resolution fails).
 */
export async function resolveEntryTarget(
  cfg: AppConfig,
  entry: RepoIndexEntry,
): Promise<{ owner: string; name: string }> {
  try {
    if (entry.repoId) {
      const info = await repoInfoById({ ...cfg, suiteDirectory: entry.suiteDirectory }, entry.repoId);
      if (info.owner && info.name) return { owner: info.owner, name: info.name };
    } else {
      const resolved = await resolveRepo(cfg, entry.owner, entry.name);
      if (resolved.info.owner && resolved.info.name) {
        return { owner: resolved.info.owner, name: resolved.info.name };
      }
    }
  } catch {
    // Fall back to the indexed owner/name when resolution fails.
  }
  return { owner: entry.owner, name: entry.name };
}

const sharedIndexers = new Map<string, RepoIndexer>();

/** One indexer per (chain, configured directories) pair for the app lifetime. */
export function repoIndexShared(cfg: AppConfig): RepoIndexer {
  const key = `${cfg.evmChainId}|${parseSuiteDirectories(cfg.suiteDirectory).join(",")}`;
  let indexer = sharedIndexers.get(key);
  if (!indexer) {
    indexer = new RepoIndexer(cfg);
    sharedIndexers.set(key, indexer);
  }
  return indexer;
}