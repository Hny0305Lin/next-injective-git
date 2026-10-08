import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Activity, Archive as ArchiveIcon, ChevronDown, Gauge, GitFork, HardDrive, LayoutDashboard, LoaderCircle, Map as MapIcon, Settings as SettingsIcon, User } from "lucide-react";
import {
  repoIndexShared,
  resolveEntryTarget,
  searchRepoEntries,
  type RepoIndexEntry,
  type RepoIndexStatus,
} from "../lib/repo-index";
import { ContractTypeBadge } from "../components/ContractTypeBadge";
import { resolveRepo } from "../lib/registry";
import { matchPageSuggestion, type PageSuggestion } from "../lib/search";
import {
  formatCosmWasmV1Error,
  listCosmWasmV1Repos,
  prepareCosmWasmV1Snapshot,
  resolveCosmWasmV1Owner,
} from "../lib/cosmwasm-v1";
import { addressUsername, listRepos, loadConfig, resolveOwner, timeAgo, type RepoInfo } from "../lib/chain";
import { truncateAddress } from "../lib/utils";

/** One repository row contributed by a resolved owner query. */
interface OwnerRepoRow {
  owner: string;
  name: string;
  status: number;
  suiteVersion: number;
}

interface OwnerMatch {
  address: string;
  alias: string | null;
  repos: OwnerRepoRow[];
}

type ArchiveState =
  | { phase: "idle" }
  | { phase: "loading" }
  | { phase: "done"; repos: RepoInfo[]; owner: string | null }
  | { phase: "error"; message: string };

/** Rendered state cached per query so returning from a repository page is instant. */
interface SearchSnapshot {
  ownerMatch: OwnerMatch | null;
  exact: { owner: string; name: string } | null;
  archive: ArchiveState;
  archiveOpen: boolean;
  savedAt: number;
}

const SEARCH_SNAPSHOT_TRUST_MS = 10 * 60_000;
const SEARCH_SNAPSHOT_LIMIT = 20;
const searchSnapshots = new Map<string, SearchSnapshot>();

function statusName(status: number): "active" | "frozen" | "delisted" {
  return status === 1 ? "frozen" : status === 2 ? "delisted" : "active";
}

function moderationToStatus(moderation: string): number {
  return moderation === "frozen" ? 1 : moderation === "delisted" ? 2 : 0;
}

const SUGGESTION_ICONS: Record<string, typeof LayoutDashboard> = {
  Dashboard: LayoutDashboard,
  Monitor: Gauge,
  MapMonitor: MapIcon,
  Activity: Activity,
  IPFS: HardDrive,
  "V1 Archive": ArchiveIcon,
  Settings: SettingsIcon,
};

/**
 * GitHub-style repository search page: /search?q=<term>&type=repositories.
 * Results come from the client-side index (chain head events, history walk,
 * absorbed owner listings, and the public explorer fallback) plus the
 * repositories of a resolved owner/username; opening a result always
 * re-resolves it on-chain. V1 archive matches expand inline below.
 */
export default function SearchPage() {
  const [params] = useSearchParams();
  const nav = useNavigate();
  const query = (params.get("q") ?? "").trim();
  const cfg = useMemo(() => loadConfig(), []);
  const [status, setStatus] = useState<RepoIndexStatus | null>(null);
const boot = query ? searchSnapshots.get(query) : undefined;
  const bootFresh = boot !== undefined && Date.now() - boot.savedAt < SEARCH_SNAPSHOT_TRUST_MS;
  const [ownerMatch, setOwnerMatch] = useState<OwnerMatch | null>(bootFresh ? boot.ownerMatch : null);
  const [exact, setExact] = useState<{ owner: string; name: string } | null>(bootFresh ? boot.exact : null);
  const [probing, setProbing] = useState(false);
  const [opening, setOpening] = useState<string | null>(null);
  const [archiveOpen, setArchiveOpen] = useState(bootFresh ? boot.archiveOpen : false);
  const [archive, setArchive] = useState<ArchiveState>(bootFresh ? boot.archive : { phase: "idle" });

  const firstRun = useRef(true);

  useEffect(() => {
    const indexer = repoIndexShared(cfg);
    setStatus(indexer.getStatus());
    const unsubscribe = indexer.subscribe(setStatus);
    indexer.ensureStarted();
    void indexer.ensureExplorerIndexed().catch(() => undefined);
    return unsubscribe;
  }, [cfg]);

  useEffect(() => {
    // Returning from a repository page restores the cached snapshot instantly
    // and revalidates in the background; a changed query restores or clears.
    if (firstRun.current) {
      firstRun.current = false;
    } else {
      const cached = searchSnapshots.get(query);
      const fresh = cached !== undefined && Date.now() - cached.savedAt < SEARCH_SNAPSHOT_TRUST_MS;
      setOwnerMatch(fresh ? cached.ownerMatch : null);
      setExact(fresh ? cached.exact : null);
      setArchiveOpen(fresh ? cached.archiveOpen : false);
      setArchive(fresh ? cached.archive : { phase: "idle" });
    }
    if (!query) return;
    let cancelled = false;
    const slash = query.indexOf("/");
    setProbing(true);
    void (async () => {
      try {
        if (slash > 0 && slash < query.length - 1) {
          const ownerPart = query.slice(0, slash);
          const namePart = query.slice(slash + 1);
          try {
            const resolved = await resolveRepo(cfg, ownerPart, namePart);
            if (!cancelled) setExact({ owner: resolved.info.owner, name: resolved.info.name });
          } catch {
            // Not an exact repository; fall through to keyword results.
          }
          return;
        }
        if (slash >= 0) return;
        const address = await resolveOwner(cfg, query);
        const alias = query.toLowerCase().startsWith("inj1")
          ? await addressUsername(cfg, address).catch(() => null)
          : query;
        const repos = await listRepos(cfg, address);
        if (cancelled) return;
        // A resolved owner enriches the index with their repositories.
        repoIndexShared(cfg).absorbOwnerRepos(repos.map((repo) => ({
          owner: repo.owner,
          name: repo.name,
          moderation: repo.moderation_status,
          suiteVersion: repo.suite_version != null ? Number(repo.suite_version) : undefined,
        })));
        if (!cancelled) {
          setOwnerMatch({
            address,
            alias,
            repos: repos.map((repo) => ({
              owner: repo.owner,
              name: repo.name,
              status: moderationToStatus(repo.moderation_status),
              suiteVersion: repo.suite_version != null ? Number(repo.suite_version) : 4,
            })),
          });
        }
      } catch {
        // Not an address or registered username.
      } finally {
        if (!cancelled) setProbing(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [query, cfg]);

  // The V1 archive section expands in place: results load below the EVM
  // results without ever navigating to the archive pages.
  useEffect(() => {
    if (!archiveOpen || !query) return;
    let cancelled = false;
    setArchive((previous) => (previous.phase === "idle" ? { phase: "loading" } : previous));
    void (async () => {
      try {
        await prepareCosmWasmV1Snapshot();
        const slash = query.indexOf("/");
        const ownerPart = slash > 0 ? query.slice(0, slash) : query;
        const namePart = slash > 0 ? query.slice(slash + 1) : "";
        let owner: string;
        try {
          owner = await resolveCosmWasmV1Owner(ownerPart);
        } catch {
          if (!cancelled) setArchive({ phase: "done", repos: [], owner: null });
          return;
        }
        const repos = await listCosmWasmV1Repos(owner);
        if (cancelled) return;
        const needle = namePart.trim().toLowerCase();
        const filtered = needle
          ? repos.filter((repo) => repo.name.toLowerCase().includes(needle))
          : repos;
        if (!cancelled) setArchive({ phase: "done", repos: filtered, owner });
      } catch (cause) {
        if (!cancelled) setArchive({ phase: "error", message: formatCosmWasmV1Error(cause, "owner") });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [archiveOpen, query]);

  const pageSuggestion = useMemo<PageSuggestion | null>(() => matchPageSuggestion(query), [query]);

  const results = useMemo(() => {
    if (!query) return [];
    const hits = searchRepoEntries(repoIndexShared(cfg).entries, query, 50);
    const seen = new Set(hits.map((entry) => `${entry.owner}/${entry.name}/v${entry.suiteVersion}`));
    const ownerRows: RepoIndexEntry[] = [];
    for (const repo of ownerMatch?.repos ?? []) {
      const key = `${repo.owner}/${repo.name}/v${repo.suiteVersion}`;
      if (seen.has(key)) continue;
      seen.add(key);
      ownerRows.push({
        repoId: null,
        suiteDirectory: "",
        suiteVersion: repo.suiteVersion,
        owner: repo.owner,
        name: repo.name,
        status: repo.status,
        blockNumber: -1,
        logIndex: -1,
        createdBlock: 0,
      });
    }
    return [...hits, ...ownerRows];
  }, [query, cfg, status, ownerMatch]);

  // Loading is shown as a rotating border on the results area instead of a
  // loose spinner widget.
  const resultsLoading = probing || (status?.building ?? false) || opening !== null;

  useEffect(() => {
    if (!query || probing) return;
    if (archiveOpen && archive.phase === "loading") return;
    if (searchSnapshots.size >= SEARCH_SNAPSHOT_LIMIT) {
      const oldest = searchSnapshots.keys().next().value;
      if (oldest !== undefined) searchSnapshots.delete(oldest);
    }
    searchSnapshots.set(query, { ownerMatch, exact, archive, archiveOpen, savedAt: Date.now() });
  }, [query, ownerMatch, exact, archive, archiveOpen, probing]);

  const open = async (entry: RepoIndexEntry) => {
    setOpening(`${entry.suiteDirectory}:${entry.repoId ?? entry.name}`);
    const { owner, name } = await resolveEntryTarget(cfg, entry);
    setOpening(null);
    nav(`/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`);
  };

  return (
    <div className="search-page">
      <div className="search-page-head">
        <h1>Search</h1>
        <span className="muted">
          repositories matching <code>{query || "(empty)"}</code>
        </span>
        {resultsLoading && (
          <span className="search-page-loading" role="status" aria-live="polite">
            <LoaderCircle size={12} className="animate-spin" aria-hidden="true" />
            Searching...
          </span>
        )}
      </div>

      {pageSuggestion && (() => {
        const SuggestIcon = SUGGESTION_ICONS[pageSuggestion.label] ?? LayoutDashboard;
        return (
          <button
            type="button"
            className="card search-exact-card"
            onClick={() => nav(pageSuggestion.to)}
          >
            <SuggestIcon size={15} aria-hidden="true" />
            <span>
              Did you mean <b>{pageSuggestion.label}</b>?
            </span>
          </button>
        );
      })()}

      {exact && (
        <button
          type="button"
          className="card search-exact-card"
          onClick={() => nav(`/${encodeURIComponent(exact.owner)}/${encodeURIComponent(exact.name)}`)}
        >
          <GitFork size={15} aria-hidden="true" />
          <span>
            Jump to <b>{truncateAddress(exact.owner, 12)}/{exact.name}</b>
          </span>
        </button>
      )}

      {ownerMatch && (
        <Link className="card search-owner-card" to={`/${encodeURIComponent(ownerMatch.address)}`}>
          <User size={15} aria-hidden="true" />
          <span>
            <b>{ownerMatch.alias ?? truncateAddress(ownerMatch.address, 14)}</b>
            {" - "}
            {ownerMatch.repos.length} {ownerMatch.repos.length === 1 ? "repository" : "repositories"}
          </span>
        </Link>
      )}

      <div
        className="search-results"
        role="list"
      >
        {results.map((entry) => {
          const key = `${entry.suiteDirectory}:${entry.repoId ?? `${entry.owner}/${entry.name}/v${entry.suiteVersion}`}`;
          return (
            <button
              key={key}
              type="button"
              className="search-result card"
              role="listitem"
              onClick={() => void open(entry)}
              disabled={opening === key}
            >
              <span className="search-result-title">
                <b>{entry.name}</b>
                <ContractTypeBadge kind="evm-v2" suiteVersion={BigInt(entry.suiteVersion)} />
                <span className={`badge ${statusName(entry.status)}`}>{statusName(entry.status)}</span>
              </span>
              <span className="search-result-meta muted small">
                {truncateAddress(entry.owner, 16)}
                {entry.repoId ? " - chain event" : " - owner listing"}
              </span>
            </button>
          );
        })}
      </div>

      {results.length > 0 && (
        <div className="search-archive">
          <button
            type="button"
            className="search-archive-toggle"
            onClick={() => setArchiveOpen((open) => !open)}
            aria-expanded={archiveOpen}
          >
            <span className="search-archive-toggle-label">View other V1 Archive results</span>
            <ChevronDown
              size={14}
              className={`search-archive-chev${archiveOpen ? " open" : ""}`}
              aria-hidden="true"
            />
          </button>
          {archiveOpen && (
            <div
              className="search-archive-body" role="list"
            >
              {archive.phase === "loading" && (
                <div className="search-archive-note muted small" role="status">
                  <LoaderCircle size={12} className="animate-spin" aria-hidden="true" />
                  Loading V1 archive results...
                </div>
              )}
              {archive.phase === "error" && (
                <div className="search-archive-note muted small">{archive.message}</div>
              )}
              {archive.phase === "done" && archive.owner === null && (
                <div className="search-archive-note muted small">
                  This query does not resolve to a V1 archive owner.
                </div>
              )}
              {archive.phase === "done" && archive.owner !== null && archive.repos.length === 0 && (
                <div className="search-archive-note muted small">
                  No V1 archive repositories match this query.
                </div>
              )}
              {archive.phase === "done" && archive.repos.map((repo) => (
                <button
                  key={`${repo.owner}/${repo.name}`}
                  type="button"
                  className="search-result card"
                  onClick={() => nav(
                    `/archive/cosmwasm-v1/${encodeURIComponent(repo.owner)}/${encodeURIComponent(repo.name)}`,
                  )}
                >
                  <span className="search-result-title">
                    <b>{repo.name}</b>
                    <ContractTypeBadge kind="cosmwasm-v1" />
                    <span className={`badge ${repo.moderation_status}`}>{repo.moderation_status}</span>
                  </span>
                  <span className="search-result-meta muted small">
                    {truncateAddress(repo.owner, 16)} - updated {timeAgo(repo.updated_at)}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {query && results.length === 0 && !status?.building && !probing && (
        <div className="search-empty">
          <p>No repositories found for "{query}".</p>
          <p className="muted small">
            The index grows as you browse owners, connect a wallet, or when the explorer source is reachable.
          </p>
        </div>
      )}
    </div>
  );
}