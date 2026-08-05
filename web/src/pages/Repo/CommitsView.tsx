import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { GitCommit } from "lucide-react";
import { useLoadedRef, shortRef } from "./useRepoViews";
import type { AppConfig, RefInfo } from "../../lib/chain";
import type { RepoStore, CommitMeta } from "../../lib/gitstore";

export default function CommitsView({
  cfg,
  store,
  refs,
  current,
  base,
}: {
  cfg: AppConfig;
  store: RepoStore;
  refs: RefInfo[];
  current: RefInfo;
  base: string;
}) {
  const nav = useNavigate();
  const { ready, status, err } = useLoadedRef(cfg, store, current);
  const [commits, setCommits] = useState<CommitMeta[] | null>(null);
  const short = shortRef(current.ref_name);

  useEffect(() => {
    if (!ready) return;
    setCommits(null);
    store.log(current.commit_sha).then(setCommits).catch(() => setCommits([]));
  }, [ready, current.commit_sha, store]);

  return (
    <div>
      <div className="toolbar">
        <select
          value={short}
          onChange={(e) => nav(`${base}/commits/${encodeURIComponent(e.target.value)}`)}
          aria-label="branch or tag"
        >
          {refs.map((r) => (
            <option key={r.ref_name} value={shortRef(r.ref_name)}>
              {r.ref_name.startsWith("refs/tags/") ? `⌂ ${shortRef(r.ref_name)}` : shortRef(r.ref_name)}
            </option>
          ))}
        </select>
      </div>
      {err && <div className="error" role="alert">{err}</div>}
      {!commits && !err && (
        <div className="spinner" aria-live="polite">{status || "reconstructing history…"}</div>
      )}
      {commits && commits.length === 0 && (
        <div className="muted" style={{ padding: "16px 0" }}>no commits yet.</div>
      )}
      {commits && commits.length > 0 && (
        <div className="filelist">
          <div className="row head-row">
            <span style={{ width: 16, flexShrink: 0 }} />
            <span style={{ flex: 1 }}>Message</span>
            <span className="muted-cell">Author</span>
            <span className="muted-cell">Date</span>
          </div>
          {commits.map((c) => {
            const msg = c.message.split("\n")[0];
            const dateStr = c.timestamp ? new Date(c.timestamp).toLocaleString() : "";
            return (
              <Link className="row" key={c.oid} to={`${base}/commit/${c.oid}`}>
                <span className="icon"><GitCommit size={14} /></span>
                <span className="name ellipsis" style={{ flex: 1 }} title={msg}>{msg}</span>
                <span className="muted-cell">{c.author}</span>
                <span className="muted-cell" style={{ maxWidth: 160 }}>{dateStr}</span>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
