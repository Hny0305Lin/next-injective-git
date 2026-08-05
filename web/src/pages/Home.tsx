import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useWallet } from "../lib/WalletContext";
import {
  contractActivity,
  contractConfig,
  listRepos,
  loadConfig,
  resolveOwner,
  timeAgo,
  type ContractConfig,
  type RepoInfo,
  type ContractTx,
} from "../lib/chain";

const INJ_EXPLORER = "https://testnet.explorer.injective.network";

function shortAddr(s: string, n = 8) {
  return s.length > n * 2 ? `${s.slice(0, n)}…${s.slice(-4)}` : s;
}

export default function Home() {
  const cfg = useMemo(() => loadConfig(), []);
  const { address } = useWallet();
  const [cc, setCc] = useState<ContractConfig | null>(null);
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [activity, setActivity] = useState<ContractTx[]>([]);
  const [repoFilter, setRepoFilter] = useState("");
  const [err, setErr] = useState("");

  useEffect(() => {
    contractConfig(cfg).then(setCc).catch(() => setCc(null));
  }, [cfg]);

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

  useEffect(() => {
    contractActivity(cfg, 20).then(setActivity).catch(() => setActivity([]));
  }, [cfg]);

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

        {/* Right: recent activity */}
        <div className="dashboard-side">
          <div className="dash-section-title">Recent on-chain activity</div>
          <div className="card" style={{ padding: 0, overflow: "hidden" }}>
            {activity.length === 0 && (
              <div className="muted" style={{ padding: "16px", fontSize: "0.84rem" }}>
                no recent activity.
              </div>
            )}
            {activity.map((tx) => (
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
            {activity.length > 0 && (
              <div style={{ padding: "8px 14px" }}>
                <Link to="/explorer" className="muted" style={{ fontSize: "0.8rem" }}>
                  View all transactions →
                </Link>
              </div>
            )}
          </div>

          {cc && (
            <div style={{ marginTop: 16 }}>
              <div className="dash-section-title">Network</div>
              <div className="card" style={{ padding: "10px 14px", fontSize: "0.78rem" }}>
                <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 4 }}>
                  <span className="muted">network</span>
                  <span><b>injective-888</b></span>
                </div>
                <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 4 }}>
                  <span className="muted">contract</span>
                  <code style={{ fontSize: "0.72rem" }}>{shortAddr(cc.admin, 8)}</code>
                </div>
                <div style={{ display: "flex", justifyContent: "space-between" }}>
                  <span className="muted">platform fee</span>
                  <span>{(cc.platform_fee_bps / 100).toFixed(2)}%</span>
                </div>
              </div>
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
