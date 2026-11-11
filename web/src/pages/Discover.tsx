import { useCallback, useEffect, useMemo, useState } from "react";
import { Check, Copy, Database, ExternalLink, GitBranch, LoaderCircle, Search, SlidersHorizontal } from "lucide-react";
import { Link } from "react-router-dom";
import { ContractTypeBadge } from "../components/ContractTypeBadge";
import { PublicDirectoryHero } from "../components/PublicDirectoryHero";
import { showToast } from "../components/Toast";
import { formatError, isSuiteDirectoryConfigured, loadConfig, timeAgo } from "../lib/chain";
import { filterDiscoverRepositories, pageDiscoverRepositories, type DiscoverRepository, type DiscoverStatus, type DiscoverSuiteVersion } from "../lib/discover";
import { repoIndexShared, type RepoIndexStatus } from "../lib/repo-index";
import { repoInfoById } from "../lib/registry";
import { verifySuite } from "../lib/transport";

const PAGE_SIZE = 12;
const HYDRATION_LIMIT = 200;
type LoadState = "loading" | "ready" | "unconfigured" | "error";

async function hydratePublicRepositories(cfg: ReturnType<typeof loadConfig>, indexStatus: RepoIndexStatus | null): Promise<{ rows: DiscoverRepository[]; failures: number; considered: number; indexError: string | null }> {
  const candidates = repoIndexShared(cfg).publicEntries.sort((a, b) => b.createdBlock - a.createdBlock).slice(0, HYDRATION_LIMIT);
  const rows: DiscoverRepository[] = [];
  let failures = 0;
  let cursor = 0;
  const worker = async () => {
    while (cursor < candidates.length) {
      const entry = candidates[cursor++];
      if (!entry.repoId) continue;
      try {
        const info = await repoInfoById({ ...cfg, suiteDirectory: entry.suiteDirectory }, entry.repoId);
        rows.push({ ...info, source: "evm-suite-events", repo_id: entry.repoId, suite_directory: entry.suiteDirectory });
      } catch {
        failures += 1;
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(6, candidates.length) }, () => worker()));
  return { rows, failures, considered: candidates.length, indexError: indexStatus?.phase === "error" ? indexStatus.error ?? "The repository index could not be read from the RPC." : null };
}

function statusLabel(status: DiscoverStatus): string {
  return status === "all" ? "All statuses" : status[0].toUpperCase() + status.slice(1);
}

export default function Discover() {
  const cfg = useMemo(() => loadConfig(), []);
  const [indexStatus, setIndexStatus] = useState<RepoIndexStatus | null>(null);
  const [rows, setRows] = useState<DiscoverRepository[]>([]);
  const [state, setState] = useState<LoadState>("loading");
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [owner, setOwner] = useState("");
  const [status, setStatus] = useState<DiscoverStatus>("all");
  const [suiteVersion, setSuiteVersion] = useState<DiscoverSuiteVersion>("all");
  const [page, setPage] = useState(1);
  const [copied, setCopied] = useState<string | null>(null);
  const indexBuilding = indexStatus?.building ?? false;
  const indexPhase = indexStatus?.phase;
  const indexError = indexStatus?.error;

  useEffect(() => {
    const indexer = repoIndexShared(cfg);
    setIndexStatus(indexer.getStatus());
    const unsubscribe = indexer.subscribe(setIndexStatus);
    indexer.ensureStarted();
    void indexer.ensureExplorerIndexed().catch(() => undefined);
    return unsubscribe;
  }, [cfg]);

  const refresh = useCallback(async () => {
    if (!isSuiteDirectoryConfigured(cfg.suiteDirectory)) { setState("unconfigured"); setRows([]); return; }
    setState("loading"); setError("");
    try {
      await verifySuite(cfg);
      const result = await hydratePublicRepositories(cfg, indexStatus);
      setRows(result.rows);
      if (result.indexError) { setState("error"); setError(result.indexError); }
      else {
        if (result.failures > 0 && result.rows.length === 0) {
          setState("error");
          setError("The verified Suite index was found, but repository metadata could not be read from the RPC.");
        } else {
          setState("ready");
          if (result.failures > 0) setError(`${result.failures} discovered repositories could not be refreshed from the RPC.`);
        }
      }
    } catch (cause) { setState("error"); setRows([]); setError(formatError(cause)); }
  }, [cfg, indexPhase, indexError]);

  useEffect(() => {
    // A history walk can emit many entry-count updates. Hydrate once for the
    // initial index and again when the walk finishes, rather than issuing a
    // full metadata read for every intermediate block chunk.
    const timer = window.setTimeout(() => void refresh(), indexBuilding ? 250 : 0);
    return () => window.clearTimeout(timer);
  }, [refresh, indexBuilding]);
  useEffect(() => setPage(1), [query, owner, status, suiteVersion]);

  const filtered = useMemo(() => filterDiscoverRepositories(rows, { query, owner, status, suiteVersion }), [rows, query, owner, status, suiteVersion]);
  const visible = useMemo(() => pageDiscoverRepositories(filtered, page, PAGE_SIZE), [filtered, page]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const indexedCount = repoIndexShared(cfg).publicEntries.length;
  const copyClone = async (row: DiscoverRepository) => {
    const command = `igit clone igit://${row.owner}/${row.name}`;
    try { await navigator.clipboard.writeText(command); setCopied(row.repo_id); showToast("Command copied"); window.setTimeout(() => setCopied((value) => value === row.repo_id ? null : value), 1500); }
    catch { showToast("Could not copy clone command"); }
  };

  return (
    <div className="discover-page">
      <PublicDirectoryHero />
      <section className="discover-toolbar card" aria-label="Repository filters">
        <label className="discover-search"><Search size={15} aria-hidden="true" /><input className="field" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search name, owner, or description" aria-label="Search repositories" /></label>
        <label><span>Owner</span><input className="field" value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="inj1..." aria-label="Filter by owner" /></label>
        <label><span>Status</span><select className="field" value={status} onChange={(e) => setStatus(e.target.value as DiscoverStatus)} aria-label="Filter by moderation status"><option value="all">All statuses</option><option value="active">Active</option><option value="frozen">Frozen</option><option value="delisted">Delisted</option></select></label>
        <label><span>Suite</span><select className="field" value={suiteVersion} onChange={(e) => setSuiteVersion(e.target.value === "all" ? "all" : Number(e.target.value) as 3 | 4)} aria-label="Filter by Suite version"><option value="all">all</option><option value="3">Suite v3</option><option value="4">Suite v4</option></select></label>
        <span className="discover-sort"><SlidersHorizontal size={14} aria-hidden="true" /> Recently updated</span>
      </section>
      <p className="discover-scope muted"><GitBranch size={13} aria-hidden="true" /> Scope: newest {Math.min(indexedCount, HYDRATION_LIMIT)} repositories discovered from verified Suite events in this browser. This is a bounded client index, not a claim of complete network enumeration. {indexStatus?.building ? "Index is still refreshing." : ""}</p>
      {error && <div className="error discover-error" role="alert">{error}</div>}
      {state === "loading" && rows.length === 0 && <div className="surface-panel empty-state"><LoaderCircle className="animate-spin" size={22} /><h3>Reading the public Suite index</h3><p>Verifying the configured SuiteDirectory and loading repository metadata from chain.</p></div>}
      {state === "unconfigured" && <div className="surface-panel empty-state"><Database size={22} /><h3>Public discovery is not configured</h3><p>This client has no configured SuiteDirectory. Add a verified Directory in Settings before reading repository data.</p><Link className="btn" to="/settings">Open Settings</Link></div>}
      {state === "error" && rows.length === 0 && <div className="surface-panel empty-state"><Database size={22} /><h3>Repositories could not be loaded</h3><p>Check the RPC connection and the verified SuiteDirectory, then try again.</p><button type="button" onClick={() => void refresh()}>Try again</button></div>}
      {state === "ready" && filtered.length === 0 && <div className="surface-panel empty-state"><Search size={22} /><h3>No repositories match these filters</h3><p>Try a different keyword, owner, status, or Suite version.</p></div>}
      {visible.length > 0 && <div className="discover-results" aria-live="polite">{visible.map((row) => <article className="discover-card card" key={`${row.suite_directory}:${row.repo_id}`}><div className="discover-card-main"><div className="discover-card-title"><GitBranch size={16} aria-hidden="true" /><Link to={`/${encodeURIComponent(row.owner)}/${encodeURIComponent(row.name)}?suite=${row.suite_version}`}>{row.owner}/{row.name}</Link><ContractTypeBadge kind="evm-v2" suiteVersion={row.suite_version} /><span className={`badge ${row.moderation_status}`}>{statusLabel(row.moderation_status)}</span></div><p className="discover-description">{row.description || <i>No description</i>}</p><div className="discover-meta muted"><span>Default <code>{row.default_branch}</code></span><span>Updated {timeAgo(row.updated_at)}</span><span>{row.suite_version === 4n ? "EVM V4 (BYOS)" : "EVM V2 + V3"}</span>{row.forked_from && <span>Forked from <code>{row.forked_from}</code></span>}</div></div><div className="discover-card-actions"><Link className="btn" to={`/${encodeURIComponent(row.owner)}/${encodeURIComponent(row.name)}?suite=${row.suite_version}`}><ExternalLink size={13} /> Open</Link><button type="button" className="btn" onClick={() => void copyClone(row)} title={`igit clone igit://${row.owner}/${row.name}`} aria-label={`Copy clone command for ${row.name}`}>{copied === row.repo_id ? <Check size={13} /> : <Copy size={13} />} {copied === row.repo_id ? "Copied" : "Copy"}</button></div></article>)}</div>}
      {filtered.length > PAGE_SIZE && <div className="discover-pagination" aria-label="Repository pages"><button type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>Previous</button><span className="muted">Page {page} of {pageCount}</span><button type="button" disabled={page >= pageCount} onClick={() => setPage((value) => value + 1)}>Next</button></div>}
    </div>
  );
}
