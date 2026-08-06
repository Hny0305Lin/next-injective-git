import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { GitCommit } from "lucide-react";
import type { AppConfig, RefInfo } from "../../lib/chain";
import type { CommitMeta, FileChange, RepoStore } from "../../lib/gitstore";

export default function CommitView({
  cfg,
  store,
  refs,
  sha,
  base,
}: {
  cfg: AppConfig;
  store: RepoStore;
  refs: RefInfo[];
  sha: string;
  base: string;
}) {
  const [meta, setMeta] = useState<CommitMeta | null>(null);
  const [changes, setChanges] = useState<FileChange[] | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    setMeta(null);
    setChanges(null);
    setErr("");
    (async () => {
      try {
        for (const r of refs) await store.loadRef(cfg, r);
        setMeta(await store.commitMeta(sha));
        setChanges(await store.diffCommit(sha));
      } catch (e) {
        setErr(String(e));
      }
    })();
  }, [sha, cfg, store, refs]);

  if (err) return <div className="error" role="alert">{err}</div>;
  if (!meta) return <div className="spinner" aria-live="polite">loading commit…</div>;

  const msg = meta.message.split("\n")[0];
  const dateStr = meta.timestamp ? new Date(meta.timestamp).toLocaleString() : "";

  return (
    <div>
      <div className="card" style={{ marginBottom: 14, padding: "14px 16px" }}>
        <div style={{ display: "flex", alignItems: "flex-start", gap: 10 }}>
          <GitCommit size={16} style={{ color: "var(--accent-text)", marginTop: 2, flexShrink: 0 }} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontWeight: 600, fontSize: "0.92rem", marginBottom: 4 }}>{msg}</div>
            <div className="muted" style={{ fontSize: "0.8rem", lineHeight: 1.5 }}>
              <code className="mono">{meta.oid.slice(0, 12)}</code>
              {" · "}
              {meta.author}
              {" · "}
              {dateStr}
              {meta.parents.length > 0 && (
                <>
                  {" · parent "}
                  <Link to={`${base}/commit/${meta.parents[0]}`} className="mono">
                    {meta.parents[0].slice(0, 10)}
                  </Link>
                </>
              )}
            </div>
          </div>
        </div>
      </div>
      {!changes && <div className="spinner" aria-live="polite">computing diff…</div>}
      {changes?.map((ch) => (
        <div className="diff-file" key={ch.path}>
          <div className={`diff-head ${ch.kind}`}>
            <span className="diff-kind">{ch.kind}</span> <code>{ch.path}</code>
          </div>
          {ch.patch ? <DiffBody patch={ch.patch} /> : <div className="muted" style={{ padding: 10 }}>binary or oversized file</div>}
        </div>
      ))}
      {changes && changes.length === 0 && <p className="muted">no changes (empty commit).</p>}
    </div>
  );
}

function DiffBody({ patch }: { patch: string }) {
  const lines = patch.split("\n").slice(4);
  return (
    <pre className="diff-body">
      {lines.map((l, i) => {
        const cls = l.startsWith("+")
          ? "add"
          : l.startsWith("-")
            ? "del"
            : l.startsWith("@@")
              ? "hunk"
              : "";
        return (
          <div key={i} className={`diff-line ${cls}`} aria-hidden="true">
            {l || " "}
          </div>
        );
      })}
    </pre>
  );
}
