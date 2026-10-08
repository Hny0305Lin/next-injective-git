import type { Hex } from "viem";
import { contractActivity } from "./activity";
import { toInjectiveAddress } from "./address";
import { listCosmWasmV1Refs, listCosmWasmV1Repos } from "./cosmwasm-v1";
import type { PackLocation } from "./packmanifest";
import {
  isSuiteDirectoryConfigured,
  networkProfile,
  parseSuiteDirectories,
  type AppConfig,
} from "./profile";
import { listRepos, resolveRef, resolveSuccessorRefCommit, type RepoInfo } from "./registry";
import { rpcRequest, verifySuite } from "./transport";

// ---------------------------------------------------------------------------
// Storage statistics for the MapMonitor board.
//
// Everything here is a read-only, keyless observation:
//   1. Owners are discovered from the decoded suite activity window (the same
//      bounded observation the Monitor page uses — owners with no recent
//      activity are not counted).
//   2. Repositories are enumerated per owner via listRepositoriesPage.
//   3. Only the DEFAULT REF of each repository is sampled:
//        suite v3 -> ref.pack_uris count into the IPFS layer
//        suite v4 -> on-chain manifest commitment, verified manifest, packs[]
//                    aggregated per PackLocation.provider and per URL host
//
// Counts are therefore packs (git packfiles), not individual files; the
// manifest does not describe object-level contents.
// ---------------------------------------------------------------------------

export type StorageProvider = PackLocation["provider"];

// Per-owner slice of the storage observation: the bounded name samples and
// counters the MapMonitor popup renders. Owner keys are lowercase inj1
// bech32 addresses so lookups from the connected wallet always match.
export interface OwnerRepoSamples {
  reposByProvider: Record<string, number>;
  reposByHost: Record<string, number>;
  repoNamesByProvider: Record<string, string[]>;
  repoNamesByHost: Record<string, string[]>;
}

export interface StorageStats {
  observedOwners: number;
  repos: number;
  reposDiscovered: number;
  totalPacks: number;
  packsByProvider: Record<StorageProvider, number>;
  reposByProvider: Record<StorageProvider, number>;
  packsByHost: Record<string, number>;
  reposByHost: Record<string, number>;
  /**
   * Owner-scoped repository samples backing the MapMonitor popup lists:
   * per-owner repo counters plus at most MAX_REPO_NAMES_PER_KEY bounded
   * names per provider/host key. Names are keyed BY OWNER so the popup can
   * only ever render repositories owned by the wallet that is currently
   * connected; switching wallets (even with stale localStorage) must never
   * surface the previous wallet's repository names. The aggregate counters
   * above stay anonymous and never feed the wallet-gated name lists.
   */
  ownerSamples: Record<string, OwnerRepoSamples>;
  /** Read-only CosmWasm V1 archive observations (separate trust root). */
  v1: { owners: number; repos: number; packs: number };
  sampledAt: number;
  truncated: boolean;
}

// Bounds keep the board a bounded observation, not an unbounded chain walk.
const ACTIVITY_SAMPLE = 120;
const MAX_OWNERS = 24;
const MAX_REPOS_TOTAL = 96;
const V1_MAX_REPOS = 48;
const CONCURRENCY = 4;
// Popup repo lists stay short: at most this many names are kept per key; the
// aggregate counts already say how many others exist.
const MAX_REPO_NAMES_PER_KEY = 10;

// Board results are cached for one week (keyed by the configured suite
// directories); the card's refresh button re-queries on demand.
// v2: cached stats carry owner-scoped repo-name samples. The pre-v2 format
// merged repository names across every observed owner, so a legacy cache
// saved while another wallet was connected could leak those names after a
// wallet switch. Legacy entries are therefore deleted on sight, never read.
const STATS_CACHE_PREFIX = "igit-mapmonitor-stats:v2:";
const STATS_CACHE_LEGACY_PREFIX = "igit-mapmonitor-stats:";
const STATS_CACHE_TTL_MS = 7 * 24 * 60 * 60 * 1000;

export function loadCachedStorageStats(cfg: AppConfig): StorageStats | null {
  try {
    localStorage.removeItem(STATS_CACHE_LEGACY_PREFIX + cfg.suiteDirectory);
    const raw = localStorage.getItem(STATS_CACHE_PREFIX + cfg.suiteDirectory);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as { sampledAt?: number; stats?: StorageStats };
    if (typeof parsed.sampledAt !== "number") return null;
    if (Date.now() - parsed.sampledAt > STATS_CACHE_TTL_MS) return null;
    // Shape guard: a payload without owner-scoped samples cannot serve the
    // wallet-gated popup safely and must not be used.
    if (parsed.stats?.ownerSamples == null || typeof parsed.stats.ownerSamples !== "object") return null;
    return parsed.stats ?? null;
  } catch {
    return null;
  }
}

export function saveCachedStorageStats(cfg: AppConfig, stats: StorageStats): void {
  try {
    localStorage.setItem(
      STATS_CACHE_PREFIX + cfg.suiteDirectory,
      JSON.stringify({ sampledAt: Date.now(), stats }),
    );
  } catch {
    // Storage may be unavailable; caching is best-effort.
  }
}

// Latest EVM block height via a plain JSON-RPC read. The Monitor page keeps an
// identical local helper; this shared copy lets the MapMonitor marquee report
// the same observed block without duplicating the validation logic.
export async function latestEvmBlock(cfg: AppConfig): Promise<bigint> {
  const raw = await rpcRequest<Hex>(cfg, "eth_blockNumber");
  if (!/^0x(?:0|[1-9a-fA-F][0-9a-fA-F]*)$/.test(raw)) {
    throw new Error("EVM RPC returned an invalid latest block");
  }
  return BigInt(raw);
}

function emptyStats(): StorageStats {
  return {
    observedOwners: 0,
    repos: 0,
    reposDiscovered: 0,
    totalPacks: 0,
    packsByProvider: { ipfs: 0, "aws-s3": 0, "cloudflare-r2": 0 },
    reposByProvider: { ipfs: 0, "aws-s3": 0, "cloudflare-r2": 0 },
    packsByHost: {},
    reposByHost: {},
    ownerSamples: {},
    v1: { owners: 0, repos: 0, packs: 0 },
    sampledAt: Date.now(),
    truncated: false,
  };
}

async function mapLimit<T, R>(items: readonly T[], limit: number, run: (item: T) => Promise<R>): Promise<R[]> {
  const results = new Array<R>(items.length);
  let cursor = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (cursor < items.length) {
      const index = cursor++;
      results[index] = await run(items[index]);
    }
  });
  await Promise.all(workers);
  return results;
}

function locationHost(url: string): string | null {
  try {
    return new URL(url).host.toLowerCase();
  } catch {
    return null;
  }
}

// Owner identities accumulate across sessions: a flaky activity window (RPC
// rate limits, empty spans) must never collapse the board to zero once an
// owner has been observed.
const OWNERS_STORAGE_KEY = "igit-mapmonitor-owners";

function cachedOwners(): string[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(OWNERS_STORAGE_KEY) ?? "[]") as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((value): value is string => typeof value === "string" && value.startsWith("inj1"));
  } catch {
    return [];
  }
}

function rememberOwners(owners: readonly string[]): void {
  try {
    localStorage.setItem(OWNERS_STORAGE_KEY, JSON.stringify([...new Set(owners)].slice(0, 64)));
  } catch {
    // Storage may be unavailable (private mode); the cache is best-effort.
  }
}

function countPack(stats: StorageStats, providers: Set<StorageProvider>, hosts: Set<string>): void {
  stats.totalPacks += 1;
  for (const provider of providers) stats.packsByProvider[provider] += 1;
  for (const host of hosts) stats.packsByHost[host] = (stats.packsByHost[host] ?? 0) + 1;
}

function countRepo(stats: StorageStats, providers: Set<StorageProvider>, hosts: Set<string>): void {
  for (const provider of providers) stats.reposByProvider[provider] += 1;
  for (const host of hosts) stats.reposByHost[host] = (stats.reposByHost[host] ?? 0) + 1;
}

// Owner-scoped repo counters for the popup lists (same keying as the
// aggregate counts, but attributed to exactly one owner).
function ownerBucket(stats: StorageStats, owner: string): OwnerRepoSamples {
  const key = owner.toLowerCase();
  let bucket = stats.ownerSamples[key];
  if (bucket == null) {
    bucket = { reposByProvider: {}, reposByHost: {}, repoNamesByProvider: {}, repoNamesByHost: {} };
    stats.ownerSamples[key] = bucket;
  }
  return bucket;
}

function countOwnerRepo(bucket: OwnerRepoSamples, providers: Set<StorageProvider>, hosts: Set<string>): void {
  for (const provider of providers) {
    bucket.reposByProvider[provider] = (bucket.reposByProvider[provider] ?? 0) + 1;
  }
  for (const host of hosts) {
    bucket.reposByHost[host] = (bucket.reposByHost[host] ?? 0) + 1;
  }
}

// Keeps a bounded, de-duplicated name sample per key for the popup lists.
function rememberRepoNames(target: Record<string, string[]>, key: string, name: string): void {
  const list = target[key];
  if (list == null) {
    target[key] = [name];
    return;
  }
  if (list.length >= MAX_REPO_NAMES_PER_KEY || list.includes(name)) return;
  list.push(name);
}

// Wallet-scoped popup sample: the ONLY supported way to read repository
// names for the MapMonitor popups. Names and the "+ N others" total come
// exclusively from the given owner's bucket, so a wallet switch can never
// render the previous wallet's repositories -- not from fresh walks, and
// not from the week cache saved while another wallet was connected.
export function ownerRepoSample(
  stats: StorageStats | null,
  owner: string | null,
  keys: { provider?: StorageProvider; host?: string },
): { names: readonly string[]; total: number | null } {
  if (stats == null || owner == null || owner === "") return { names: [], total: null };
  const bucket = stats.ownerSamples?.[owner.toLowerCase()];
  if (bucket == null) return { names: [], total: null };
  if (keys.provider != null) {
    return {
      names: bucket.repoNamesByProvider?.[keys.provider] ?? [],
      total: bucket.reposByProvider?.[keys.provider] ?? 0,
    };
  }
  if (keys.host != null) {
    return {
      names: bucket.repoNamesByHost?.[keys.host] ?? [],
      total: bucket.reposByHost?.[keys.host] ?? 0,
    };
  }
  return { names: [], total: null };
}

// Owners via the public block explorer's transaction index: every sender
// that ever called a suite core contract, with no block window. The chain
// RPC cannot serve receipts for older suite transactions (state pruning),
// which makes the activity window alone unreliable for discovery.
async function explorerCoreSenders(cfg: AppConfig, dirs: readonly string[]): Promise<string[]> {
  const base = networkProfile(cfg).evmExplorer.replace(/\/+$/, "");
  const senders = new Set<string>();
  await Promise.all(dirs.map(async (dir) => {
    try {
      const binding = await verifySuite({ ...cfg, suiteDirectory: dir });
      const url = `${base}/api?module=account&action=txlist&address=${binding.modules.core}&sort=desc&page=1&offset=100`;
      const response = await fetch(url);
      if (!response.ok) return;
      const data = await response.json() as { result?: { from?: string; isError?: string }[] };
      for (const tx of data.result ?? []) {
        if (tx.isError === "0" && typeof tx.from === "string") {
          senders.add(toInjectiveAddress(tx.from));
        }
      }
    } catch {
      // Explorer unreachable: activity senders and the owner cache still apply.
    }
  }));
  return [...senders];
}

async function sampleV3Ref(cfg: AppConfig, repo: RepoInfo, stats: StorageStats): Promise<void> {
  // On-chain ref names are fully qualified (refs/heads/<branch>).
  const ref = await resolveRef(cfg, repo.owner, repo.name, `refs/heads/${repo.default_branch}`);
  const packs = ref.pack_uris.length;
  for (let index = 0; index < packs; index++) {
    countPack(stats, new Set<StorageProvider>(["ipfs"]), new Set<string>());
  }
  const providers = new Set<StorageProvider>(["ipfs"]);
  countRepo(stats, providers, new Set<string>());
  const bucket = ownerBucket(stats, repo.owner);
  countOwnerRepo(bucket, providers, new Set<string>());
  rememberRepoNames(bucket.repoNamesByProvider, "ipfs", repo.name);
}

async function sampleV4Ref(cfg: AppConfig, repo: RepoInfo, stats: StorageStats): Promise<void> {
  // On-chain ref names are fully qualified (refs/heads/<branch>).
  const { manifest } = await resolveSuccessorRefCommit(cfg, repo.owner, repo.name, `refs/heads/${repo.default_branch}`);
  const providers = new Set<StorageProvider>();
  const hosts = new Set<string>();
  for (const pack of manifest.packs) {
    const packProviders = new Set<StorageProvider>();
    const packHosts = new Set<string>();
    for (const location of pack.locations) {
      packProviders.add(location.provider);
      const host = locationHost(location.url);
      if (host) packHosts.add(host);
    }
    for (const provider of packProviders) providers.add(provider);
    for (const host of packHosts) hosts.add(host);
    countPack(stats, packProviders, packHosts);
  }
  countRepo(stats, providers, hosts);
  const bucket = ownerBucket(stats, repo.owner);
  countOwnerRepo(bucket, providers, hosts);
  for (const provider of providers) rememberRepoNames(bucket.repoNamesByProvider, provider, repo.name);
  for (const host of hosts) rememberRepoNames(bucket.repoNamesByHost, host, repo.name);
}

// V1 archive sampling reuses the EVM activity owners: username migration kept
// the identities, so the same bech32 addresses own the historical V1 repos.
// (Public LCD tx-search indexing is unavailable on both endpoints, so there
// is no direct "all archive owners" discovery; the note reports the bound.)
async function sampleV1Archive(stats: StorageStats, owners: readonly string[]): Promise<void> {
  const boundedOwners = owners.slice(0, MAX_OWNERS);
  stats.v1.owners = boundedOwners.length;
  const repos = (await mapLimit(boundedOwners, CONCURRENCY, async (owner) => {
    try {
      return await listCosmWasmV1Repos(owner);
    } catch {
      return [] as RepoInfo[];
    }
  })).flat();
  if (repos.length > V1_MAX_REPOS) stats.truncated = true;
  const bounded = repos.slice(0, V1_MAX_REPOS);
  stats.v1.repos = bounded.length;
  await mapLimit(bounded, CONCURRENCY, async (repo) => {
    try {
      const refs = await listCosmWasmV1Refs(repo.owner, repo.name);
      stats.v1.packs += refs.reduce((sum, ref) => sum + ref.pack_uris.length, 0);
    } catch {
      // Unreadable refs leave that repo's packs unattributed.
    }
  });
}

export async function collectStorageStats(cfg: AppConfig): Promise<StorageStats> {
  if (!isSuiteDirectoryConfigured(cfg.suiteDirectory)) {
    throw new Error("SuiteDirectory is not configured");
  }

  const stats = emptyStats();

  // 1) Discover owners: block explorer core-contract senders (complete
  //    history) plus the EVM activity window, merged with the accumulated
  //    cross-session cache. The same identities (username migration) also
  //    own the historical V1 archive repos.
  let senders: string[] = [];
  try {
    const activity = await contractActivity(cfg, ACTIVITY_SAMPLE);
    senders = activity.map((tx) => tx.sender);
  } catch {
    senders = [];
  }
  const explorerSenders = await explorerCoreSenders(cfg, parseSuiteDirectories(cfg.suiteDirectory));
  const candidates = [...new Set([...senders, ...explorerSenders, ...cachedOwners()])];
  rememberOwners(candidates);
  stats.observedOwners = candidates.length;
  if (candidates.length > MAX_OWNERS) stats.truncated = true;

  // V1 archive observation (separate trust root, read-only snapshot).
  await sampleV1Archive(stats, candidates);

  // 2) Enumerate repositories per owner, PER DIRECTORY: the shared
  //    listRepos() merge dedupes by owner/name, which silently drops a v3
  //    repository whose v4 successor shares the name. Suite copies are
  //    distinct storage and must both be sampled.
  const dirs = parseSuiteDirectories(cfg.suiteDirectory);
  const listings = await mapLimit(candidates.slice(0, MAX_OWNERS), CONCURRENCY, async (owner) => {
    const perDir = await Promise.allSettled(
      dirs.map((dir) =>
        listRepos({ ...cfg, suiteDirectory: dir }, owner).then((repos) => repos.map((repo) => ({ repo, dir }))),
      ),
    );
    return perDir.flatMap((result) => (result.status === "fulfilled" ? result.value : []));
  });
  const discovered = listings.flat();
  if (discovered.length > MAX_REPOS_TOTAL) stats.truncated = true;
  const sampled = discovered
    .sort((a, b) => b.repo.updated_at - a.repo.updated_at)
    .slice(0, MAX_REPOS_TOTAL);
  stats.reposDiscovered = discovered.length;
  stats.repos = sampled.length;

  // 3) Sample the default ref of each repository through its own directory.
  await mapLimit(sampled, CONCURRENCY, async ({ repo, dir }) => {
    const dirCfg = { ...cfg, suiteDirectory: dir };
    try {
      if (repo.suite_version === 4n) {
        await sampleV4Ref(dirCfg, repo, stats);
      } else {
        await sampleV3Ref(dirCfg, repo, stats);
      }
    } catch {
      // A repo whose default ref is unreadable still counts toward the repo
      // total; its packs simply stay unattributed.
    }
  });

  stats.sampledAt = Date.now();
  return stats;
}
