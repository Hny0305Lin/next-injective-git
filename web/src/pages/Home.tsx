import {
  CircleDot,
  GitBranch,
  Search,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  formatError,
  isSuiteDirectoryConfigured,
  listRepos,
  loadConfig,
  resolveOwner,
  timeAgo,
  type RepoInfo,
} from "../lib/chain";
import { useWallet } from "../lib/WalletContext";
import { ContractTypeBadge } from "../components/ContractTypeBadge";

export default function Home() {
  const cfg = useMemo(() => loadConfig(), []);
  const { address } = useWallet();
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [repoFilter, setRepoFilter] = useState("");
  const [err, setErr] = useState("");
  const suiteConfigured = isSuiteDirectoryConfigured(cfg.suiteDirectory);

  useEffect(() => {
    setErr("");
    setRepos(null);
    (async () => {
      try {
        if (address) {
          if (!suiteConfigured) {
            setRepos([]);
            return;
          }
          const owner = await resolveOwner(cfg, address);
          setRepos(await listRepos(cfg, owner));
        } else {
          setRepos([]);
        }
      } catch (error) {
        console.error("[repositories] load failed:", error);
        setErr(formatError(error));
      }
    })();
  }, [address, cfg, suiteConfigured]);

  const normalizedFilter = repoFilter.trim().toLowerCase();
  const filtered = normalizedFilter
    ? (repos ?? []).filter((repo) =>
        repo.name.toLowerCase().includes(normalizedFilter) ||
        (repo.description ?? "").toLowerCase().includes(normalizedFilter),
      )
    : repos ?? [];

  return (
    <div className="dashboard-page">
      <div className="page-heading">
        <div>
          <h1>{address ? "Your repositories" : "Dashboard"}</h1>
          <p>Browse repositories and resolve IPFS objects.</p>
        </div>
      </div>

      <div className="overview-strip" aria-label="Workspace overview">
        <div className="overview-item">
          <span><GitBranch size={15} /> Repositories</span>
          <strong>{repos ? repos.length : "-"}</strong>
        </div>
        <div className="overview-item">
          <span><CircleDot size={15} /> Network</span>
          <strong className={`status-value${suiteConfigured ? "" : " warning"}`}>
            <i /> {suiteConfigured ? "Injective" : "Setup needed"}
          </strong>
        </div>
      </div>

      {err && <div className="error" role="alert">{err}</div>}

      <section className="dashboard-main" aria-labelledby="repositories-title">
        <div className="section-heading">
          <div>
            <h2 id="repositories-title">{address ? "Repositories" : "Workspace"}</h2>
            <p>{address ? `${filtered.length} visible repositories` : "Connect a wallet to load your workspace."}</p>
          </div>
        </div>

        {address && (
          <div className="dash-search">
            <Search size={15} aria-hidden="true" />
            <input
              className="field"
              value={repoFilter}
              onChange={(event) => setRepoFilter(event.target.value)}
              placeholder="Find a repository..."
              aria-label="Find a repository"
            />
          </div>
        )}

        {!repos && !err && (
          <div className="surface-panel empty-state">
            <div className="spinner" aria-live="polite">Loading repositories...</div>
          </div>
        )}

        {repos && repos.length === 0 && !address && (
          <div className="surface-panel empty-state">
            <GitBranch size={22} />
            <h3>Connect your workspace</h3>
            <p>Connect a wallet to see your repositories, or inspect public activity on the chain.</p>
          </div>
        )}

        {repos && repos.length === 0 && address && !suiteConfigured && (
          <div className="surface-panel empty-state">
            <GitBranch size={22} />
            <h3>Repository data is not configured</h3>
            <p>Connect remains available, but repository reads require a verified EVM Suite.</p>
            <Link to="/settings" className="btn">Configure Suite</Link>
          </div>
        )}

        {repos && repos.length === 0 && address && suiteConfigured && (
          <div className="surface-panel empty-state">
            <GitBranch size={22} />
            <h3>No repositories yet</h3>
            <p>Create the first repository from your terminal.</p>
            <code className="command-line">igit init my-repo "hello chain"</code>
          </div>
        )}

        {filtered.length > 0 && (
          <div className="surface-panel repo-list">
            {filtered.map((repo) => (
              <div className="repo-list-item" key={repo.name}>
                <div className="repo-list-icon"><GitBranch size={15} /></div>
                <div className="repo-list-content">
                  <h3>
                    <Link to={`/${address ?? "demo"}/${repo.name}`}>{repo.name}</Link>
                    <ContractTypeBadge kind="evm-v2" suiteVersion={repo.suite_version} />
                    <span className={`badge ${repo.moderation_status}`}>{repo.moderation_status}</span>
                    {repo.forked_from && <span className="badge">fork</span>}
                  </h3>
                  <div className="meta muted">
                    {repo.description || <i>No description</i>}
                    <span>Default <code>{repo.default_branch}</code></span>
                    <span>Updated {timeAgo(repo.updated_at)}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
