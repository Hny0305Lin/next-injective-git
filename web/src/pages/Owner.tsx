import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Award } from "lucide-react";
import {
  addressUsername,
  badgesByRecipient,
  listRepos,
  loadConfig,
  resolveOwner,
  timeAgo,
  type BadgeInfo,
  type RepoInfo,
} from "../lib/chain";

export default function Owner() {
  const { owner = "" } = useParams();
  const cfg = useMemo(() => loadConfig(), []);
  const [addr, setAddr] = useState("");
  const [alias, setAlias] = useState<string | null>(null);
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [badges, setBadges] = useState<BadgeInfo[]>([]);
  const [err, setErr] = useState("");
  const [repoFilter, setRepoFilter] = useState("");
  const [tab, setTab] = useState<"repos" | "badges">("repos");

  useEffect(() => {
    setRepos(null);
    setErr("");
    (async () => {
      try {
        const a = await resolveOwner(cfg, owner);
        setAddr(a);
        setRepos(await listRepos(cfg, a));
        setAlias(owner.startsWith("inj1") ? await addressUsername(cfg, a) : owner);
        setBadges(await badgesByRecipient(cfg, a).catch(() => []));
      } catch (e) {
        setErr(String(e));
      }
    })();
  }, [owner, cfg]);

  if (err) return <div className="error" role="alert">{err}</div>;
  if (!repos) return <div className="spinner" aria-live="polite">querying chain…</div>;

  const filtered = repoFilter.trim()
    ? repos.filter((r) => r.name.includes(repoFilter.trim()))
    : repos;

  return (
    <div style={{ display: "grid", gridTemplateColumns: "280px 1fr", gap: 24, alignItems: "start" }}>
      {/* Sidebar */}
      <div>
        <div className="card" style={{ padding: "20px", textAlign: "center" }}>
          <div style={{
            width: 64,
            height: 64,
            borderRadius: "50%",
            background: "var(--accent-soft)",
            border: "1px solid var(--border)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            margin: "0 auto 12px",
            color: "var(--accent-text)",
            fontSize: "1.5rem",
            fontWeight: 700,
          }}>
            {(alias ?? owner).slice(0, 2).toUpperCase()}
          </div>
          {alias && (
            <div style={{ fontWeight: 600, fontSize: "1.05rem" }}>@{alias}</div>
          )}
          <div className="muted mono" style={{ fontSize: "0.72rem", marginTop: 4, wordBreak: "break-all" }}>
            {addr}
          </div>
          <div style={{ marginTop: 12, display: "flex", justifyContent: "center", gap: 16, fontSize: "0.82rem" }}>
            <div style={{ textAlign: "center" }}>
              <div style={{ fontWeight: 700, fontSize: "1.1rem" }}>{repos.length}</div>
              <div className="muted">repos</div>
            </div>
            <div style={{ textAlign: "center" }}>
              <div style={{ fontWeight: 700, fontSize: "1.1rem" }}>{badges.length}</div>
              <div className="muted">badges</div>
            </div>
          </div>
        </div>
      </div>

      {/* Main content */}
      <div>
        <div className="tabs" role="tablist" style={{ marginBottom: 14 }}>
          <button className={tab === "repos" ? "on" : ""} onClick={() => setTab("repos")} role="tab">
            Repositories
          </button>
          <button className={tab === "badges" ? "on" : ""} onClick={() => setTab("badges")} role="tab">
            <Award size={14} /> Badges
          </button>
        </div>

        {tab === "repos" && (
          <>
            <div className="dash-search">
              <input
                className="field"
                value={repoFilter}
                onChange={(e) => setRepoFilter(e.target.value)}
                placeholder="search repositories…"
              />
            </div>
            {filtered.length === 0 && (
              <div className="muted" style={{ padding: "16px 0" }}>
                {repos.length === 0 ? "no repositories on chain." : "no matching repositories."}
              </div>
            )}
            {filtered.map((r) => (
              <div className="repo-list-item" key={r.name}>
                <h3>
                  <Link to={`/${owner}/${r.name}`}>{r.name}</Link>
                  <span className={`badge ${r.moderation_status}`}>{r.moderation_status}</span>
                  {r.forked_from && <span className="badge">fork</span>}
                </h3>
                <div className="meta muted">
                  {r.description || <i>no description</i>} · default <code>{r.default_branch}</code> ·
                  updated {timeAgo(r.updated_at)}
                </div>
              </div>
            ))}
          </>
        )}

        {tab === "badges" && (
          <>
            {badges.length === 0 && (
              <div className="muted" style={{ padding: "16px 0" }}>no badges awarded yet.</div>
            )}
            {badges.map((b) => (
              <div className="repo-list-item" key={b.id}>
                <h3>
                  <span style={{ fontWeight: 700 }}>#{b.id}</span>
                  <span className="badge">badge</span>
                </h3>
                <div className="meta muted">
                  from <Link to={`/${b.repo_owner}/${b.repo_name}`}>{b.repo_owner.slice(0, 12)}…/{b.repo_name}</Link> · {timeAgo(b.awarded_at)}
                </div>
                <div style={{ marginTop: 4, fontSize: "0.84rem", fontStyle: "italic", color: "var(--fg-muted)" }}>
                  &ldquo;{b.reason}&rdquo;
                </div>
              </div>
            ))}
          </>
        )}
      </div>
    </div>
  );
}
