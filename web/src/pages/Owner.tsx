import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
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
  const cfg = loadConfig();
  const [addr, setAddr] = useState("");
  const [alias, setAlias] = useState<string | null>(null);
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [badges, setBadges] = useState<BadgeInfo[]>([]);
  const [err, setErr] = useState("");

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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [owner]);

  if (err) return <div className="error">{err}</div>;
  if (!repos) return <div className="spinner">querying chain…</div>;

  return (
    <div>
      <h2>
        {alias ? `@${alias}` : `${owner.slice(0, 14)}…`}{" "}
        <span className="muted mono" style={{ fontSize: "0.75rem" }}>
          {addr}
        </span>
      </h2>
      {repos.length === 0 && <p className="muted">no repositories on chain.</p>}
      {repos.map((r) => (
        <div className="repo-item" key={r.name}>
          <h3>
            <Link to={`/${owner}/${r.name}`}>{r.name}</Link>{" "}
            <span className={`badge ${r.moderation_status}`}>{r.moderation_status}</span>
            {r.forked_from && <span className="badge">fork</span>}
          </h3>
          <div className="muted">
            {r.description || <i>no description</i>} · default <code>{r.default_branch}</code> ·
            updated {timeAgo(r.updated_at)}
          </div>
        </div>
      ))}

      {badges.length > 0 && (
        <>
          <h3 style={{ marginTop: 28 }}>🏆 Contribution badges</h3>
          {badges.map((b) => (
            <div className="card sponsor-entry" key={b.id}>
              <div>
                <b>#{b.id}</b>{" "}
                <span className="muted">
                  from{" "}
                  <Link to={`/${b.repo_owner}/${b.repo_name}`}>
                    {b.repo_owner.slice(0, 12)}…/{b.repo_name}
                  </Link>{" "}
                  · {timeAgo(b.awarded_at)}
                </span>
              </div>
              <div className="sponsor-msg">“{b.reason}”</div>
            </div>
          ))}
        </>
      )}
    </div>
  );
}
