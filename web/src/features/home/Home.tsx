import {
  Activity,
  ArrowUpRight,
  Box,
  CircleDot,
  GitBranch,
  Search,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import {
  contractActivity,
  formatError,
  isSuiteDirectoryConfigured,
  listRepos,
  loadConfig,
  resolveOwner,
  timeAgo,
  type ContractTx,
  type RepoInfo,
} from "../../lib/chain";
import { useWallet } from "../../lib/WalletContext";
import { ContractTypeBadge } from "../../components/ContractTypeBadge";
import { truncateAddress } from "../../lib/utils";

const EVM_EXPLORER = "https://testnet-injective.cloud.blockscout.com";
const ACTIVITY_LIMIT = 100;

export default function Home() {
  const cfg = useMemo(() => loadConfig(), []);
  const { address } = useWallet();
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [activity, setActivity] = useState<ContractTx[]>([]);
  const [activityLoaded, setActivityLoaded] = useState(false);
  const [activityError, setActivityError] = useState("");
  const [repoFilter, setRepoFilter] = useState("");
  const [err, setErr] = useState("");
  const sideRef = useRef<HTMLDivElement>(null);
  const suiteConfigured = isSuiteDirectoryConfigured(cfg.suiteDirectory);

  useEffect(() => {
    if (!suiteConfigured) {
      setActivityLoaded(true);
      setActivityError("SuiteDirectory is not configured");
      return;
    }
    const element = sideRef.current;
    if (!element) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && !activityLoaded) {
          setActivityLoaded(true);
          setActivityError("");
          contractActivity(cfg, ACTIVITY_LIMIT).then(setActivity).catch((error) => {
            console.error("[activity] load failed:", error);
            setActivity([]);
            setActivityError(formatError(error));
          });
        }
      },
      { rootMargin: "200px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [cfg, activityLoaded]);

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
          <p>Browse repositories, inspect on-chain activity, and resolve IPFS objects.</p>
        </div>
        <Link to="/explorer" className="page-heading-action">
          Explore activity <ArrowUpRight size={15} />
        </Link>
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
        <div className="overview-item">
          <span><Box size={15} /> Objects</span>
          <strong>IPFS</strong>
        </div>
      </div>

      {err && <div className="error" role="alert">{err}</div>}

      <div className="dashboard">
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
              <Link to="/explorer" className="btn primary">Explore repositories</Link>
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
                      <ContractTypeBadge kind="evm-v2" />
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

        <aside className="dashboard-side" ref={sideRef} aria-labelledby="activity-title">
          <div className="section-heading compact">
            <div>
              <h2 id="activity-title">Recent activity</h2>
              <p>Latest confirmed contract actions.</p>
            </div>
            <Activity size={16} />
          </div>

          <div className="surface-panel activity-panel">
            {!activityLoaded && (
              <div className="empty-state compact">
                <div className="spinner">Loading activity...</div>
              </div>
            )}

            {activityLoaded && activityError && (
              <div className="empty-state compact activity-error">
                <Activity size={20} />
                <p>Activity is unavailable until the EVM Suite passes verification.</p>
                <Link to="/settings">Review configuration</Link>
              </div>
            )}

            {activityLoaded && !activityError && activity.length === 0 && (
              <div className="empty-state compact">
                <Activity size={20} />
                <p>No recent activity.</p>
              </div>
            )}

            {activityLoaded && !activityError && activity.length > 0 && (
              <div className="activity-list">
                {activity.slice(0, 10).map((transaction) => (
                  <div className="activity-row" key={transaction.txhash}>
                    <span className={`action-badge a-${transaction.action || "unknown"}`}>
                      {transaction.action || "unknown"}
                    </span>
                    <span className="activity-meta">
                      <code>{truncateAddress(transaction.sender, 6)}</code>
                      <span>{timeAgo(Date.parse(transaction.timestamp) / 1000)}</span>
                    </span>
                    {transaction.code !== 0 && <span className="fail-tag">failed</span>}
                  </div>
                ))}
              </div>
            )}

            {activityLoaded && !activityError && (
              <Link to="/explorer" className="panel-link">
                View all activity <ArrowUpRight size={14} />
              </Link>
            )}
          </div>
        </aside>
      </div>

      <div className="page-note">
        <span>Injective testnet</span>
        <span>SuiteDirectory <code>{suiteConfigured ? truncateAddress(cfg.suiteDirectory, 10) : "Not configured"}</code></span>
        {suiteConfigured ? (
          <a href={`${EVM_EXPLORER}/address/${cfg.suiteDirectory}`} target="_blank" rel="noreferrer">
            Open in Blockscout <ArrowUpRight size={13} />
          </a>
        ) : (
          <Link to="/settings">Configure EVM Suite <ArrowUpRight size={13} /></Link>
        )}
      </div>
    </div>
  );
}
