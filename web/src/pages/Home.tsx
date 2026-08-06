import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useWallet } from "../lib/WalletContext";
import {
  contractActivity,
  loadConfig,
  resolveOwner,
  listRepos,
  timeAgo,
  type RepoInfo,
  type ContractTx,
} from "../lib/chain";

const INJ_EXPLORER = "https://testnet.explorer.injective.network";

function shortAddr(s: string, n = 8) {
  return s.length > n * 2 ? `${s.slice(0, n)}…${s.slice(-4)}` : s;
}

const ACTIVITY_LIMIT = 100;

export default function Home() {
  const cfg = useMemo(() => loadConfig(), []);
  const { address } = useWallet();
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [activity, setActivity] = useState<ContractTx[]>([]);
  const [activityLoaded, setActivityLoaded] = useState(false);
  const [repoFilter, setRepoFilter] = useState("");
  const [err, setErr] = useState("");
  const sideRef = useRef<HTMLDivElement>(null);

  // Lazy-load activity only when the sidebar scrolls into view
  useEffect(() => {
    const el = sideRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && !activityLoaded) {
          setActivityLoaded(true);
          contractActivity(cfg, ACTIVITY_LIMIT).then(setActivity).catch(() => setActivity([]));
        }
      },
      { rootMargin: "200px" },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [cfg, activityLoaded]);

  useEffect(() => {
    setErr("");
    setRepos(null);
    (async () => {
      try {
        if (address) {
          const a = await resolveOwner(cfg, address);
          setRepos(await listRepos(cfg, a));
        } else {
          setRepos([]);
        }
      } catch (e) {
        setErr(String(e));
      }
    })();
  }, [address, cfg]);

  const filtered = repoFilter.trim()
    ? (repos ?? []).filter((r) =>
        r.name.includes(repoFilter.trim()) ||
        (r.description ?? "").toLowerCase().includes(repoFilter.trim().toLowerCase())
      )
    : repos ?? [];

  return (
    <div>
      {err && <div className="error" role="alert">{err}</div>}

      <div className="dashboard">
        {/* Left: repositories */}
        <div className="dashboard-main">
          <div className="dash-section-title">
            {address ? "Your repositories" : "Dashboard"}
          </div>

          {address && (
            <div className="dash-search">
              <input
                className="field"
                value={repoFilter}
                onChange={(e) => setRepoFilter(e.target.value)}
                placeholder="find a repository…"
              />
            </div>
          )}

          {!repos && !err && (
            <div className="spinner" aria-live="polite">loading repositories…</div>
          )}

          {repos && repos.length === 0 && !address && (
            <div className="card" style={{ padding: "24px", textAlign: "center" }}>
              <p className="muted" style={{ margin: "0 0 12px" }}>
                Connect a wallet to see your repositories, or browse the chain.
              </p>
              <Link to="/explorer" className="btn">Explore repositories</Link>
            </div>
          )}

          {repos && repos.length === 0 && address && (
            <div className="card" style={{ padding: "24px", textAlign: "center" }}>
              <p className="muted" style={{ margin: "0 0 12px" }}>
                No repositories yet. Use the CLI to create one:
              </p>
              <code style={{ background: "var(--bg-inset)", padding: "8px 14px", borderRadius: "var(--radius)", fontSize: "0.82rem", display: "inline-block" }}>
                igit init my-repo "hello chain"
              </code>
            </div>
          )}

          {filtered.map((r) => (
            <div className="repo-list-item" key={r.name}>
              <h3>
                <Link to={`/${address ?? "demo"}/${r.name}`}>{r.name}</Link>
                <span className={`badge ${r.moderation_status}`}>{r.moderation_status}</span>
                {r.forked_from && <span className="badge">fork</span>}
              </h3>
              <div className="meta muted">
                {r.description || <i>no description</i>} · default <code>{r.default_branch}</code> · updated {timeAgo(r.updated_at)}
              </div>
            </div>
          ))}
        </div>

        {/* Right: recent activity (lazy-loaded on scroll) */}
        <div className="dashboard-side" ref={sideRef}>
          <div className="dash-section-title">Recent on-chain activity</div>

          {!activityLoaded && (
            <div className="card" style={{ padding: "16px", textAlign: "center" }}>
              <div className="spinner" style={{ padding: 0 }}>loading activity…</div>
            </div>
          )}

          {activityLoaded && activity.length === 0 && (
            <div className="card" style={{ padding: "16px" }}>
              <div className="muted" style={{ fontSize: "0.84rem" }}>no recent activity.</div>
            </div>
          )}

          {activityLoaded && activity.length > 0 && (
            <div className="card" style={{ padding: 0, overflow: "hidden" }}>
              {activity.slice(0, ACTIVITY_LIMIT).map((tx) => (
                <div key={tx.txhash} style={{
                  padding: "8px 14px",
                  borderBottom: "1px solid var(--border)",
                  fontSize: "0.82rem",
                }}>
                  <span className={`action-badge a-${tx.action || "unknown"}`}>{tx.action || "?"}</span>
                  {" "}
                  <span className="muted">
                    {shortAddr(tx.sender, 6)} · {timeAgo(Date.parse(tx.timestamp) / 1000)}
                  </span>
                  {tx.code !== 0 && <span className="fail-tag">failed</span>}
                </div>
              ))}
              {activity.length >= ACTIVITY_LIMIT && (
                <div style={{ padding: "8px 14px" }}>
                  <Link to="/explorer" className="muted" style={{ fontSize: "0.8rem" }}>
                    View all transactions →
                  </Link>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <footer className="footer" style={{ marginTop: 24, borderTop: "1px solid var(--border)" }}>
        Injective testnet · Contract: <code>{shortAddr(cfg.contract, 10)}</code> ·{" "}
        <a href={`${INJ_EXPLORER}/contract/${cfg.contract}`} target="_blank" rel="noreferrer">
          Explorer ↗
        </a>
      </footer>
    </div>
  );
}
