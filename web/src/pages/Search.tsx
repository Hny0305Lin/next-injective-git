import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { GitFork, LoaderCircle, User } from "lucide-react";
import {
  repoIndexShared,
  resolveEntryTarget,
  searchRepoEntries,
  type ExplorerSourceStatus,
  type RepoIndexEntry,
  type RepoIndexStatus,
} from "../lib/repo-index";
import { ContractTypeBadge } from "../components/ContractTypeBadge";
import { resolveRepo } from "../lib/registry";
import { addressUsername, listRepos, loadConfig, resolveOwner } from "../lib/chain";
import { truncateAddress } from "../lib/utils";

interface OwnerMatch {
  address: string;
  alias: string | null;
  repos: number;
}

function statusName(status: number): "active" | "frozen" | "delisted" {
  return status === 1 ? "frozen" : status === 2 ? "delisted" : "active";
}

/**
 * GitHub-style repository search page: /search?q=<term>&type=repositories.
 * Results come from the client-side index (chain head events, history walk,
 * absorbed owner listings, and the public explorer fallback); opening a
 * result always re-resolves it on-chain.
 */
export default function SearchPage() {
  const [params] = useSearchParams();
  const nav = useNavigate();
  const query = (params.get("q") ?? "").trim();
  const cfg = useMemo(() => loadConfig(), []);
  const [status, setStatus] = useState<RepoIndexStatus | null>(null);
  const [explorer, setExplorer] = useState<ExplorerSourceStatus | null>(null);
  const [ownerMatch, setOwnerMatch] = useState<OwnerMatch | null>(null);
  const [exact, setExact] = useState<{ owner: string; name: string } | null>(null);
  const [opening, setOpening] = useState<string | null>(null);

  useEffect(() => {
    const indexer = repoIndexShared(cfg);
    setStatus(indexer.getStatus());
    const unsubscribe = indexer.subscribe(setStatus);
    indexer.ensureStarted();
    void indexer
      .ensureExplorerIndexed()
      .then(setExplorer)
      .catch(() => setExplorer({ state: "unavailable" }));
    return unsubscribe;
  }, [cfg]);

  useEffect(() => {
    setOwnerMatch(null);
    setExact(null);
    if (!query) return;
    let cancelled = false;
    const slash = query.indexOf("/");
    void (async () => {
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
      try {
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
        if (!cancelled) setOwnerMatch({ address, alias, repos: repos.length });
      } catch {
        // Not an address or registered username.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [query, cfg]);

  const results = useMemo(() => {
    if (!query) return [];
    return searchRepoEntries(repoIndexShared(cfg).entries, query, 50);
  }, [query, cfg, status, explorer, ownerMatch]);

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
      </div>

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
            {ownerMatch.repos} {ownerMatch.repos === 1 ? "repository" : "repositories"}
          </span>
        </Link>
      )}

      <div className="search-results" role="list">
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
              {opening === key && <LoaderCircle className="h-3 w-3 animate-spin" size={12} />}
            </button>
          );
        })}
      </div>

      {query && results.length === 0 && !status?.building && (
        <div className="search-empty">
          <p>No repositories found for "{query}".</p>
          <p className="muted small">
            The index grows as you browse owners, connect a wallet, or when the explorer source is reachable.
          </p>
        </div>
      )}

      <div className="search-meta muted small">
        {status?.building
          ? "Indexing chain events..."
          : `Index: ${status?.entries ?? 0} repositories`}
        {" | "}
        Explorer source: {explorer?.state ?? "idle"}
        {explorer?.state === "unavailable" ? " (chain events only)" : explorer?.state === "ok" ? ` (${explorer.events ?? 0} events)` : ""}
      </div>
    </div>
  );
}