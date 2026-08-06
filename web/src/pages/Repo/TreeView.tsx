import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { FolderOpen, File, ChevronLeft } from "lucide-react";
import { useLoadedRef, shortRef } from "./useRepoViews";
import type { AppConfig, RefInfo } from "../../lib/chain";
import type { RepoStore, TreeItem, CommitMeta } from "../../lib/gitstore";
import { Markdown } from "../../components/Markdown";
import { decodeText } from "../../lib/gitstore";

import { memo } from "react";

function RefSelect({
  refs,
  value,
  onChange,
}: {
  refs: RefInfo[];
  value: string;
  onChange: (short: string) => void;
}) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} aria-label="branch or tag">
      {refs.map((r) => (
        <option key={r.ref_name} value={shortRef(r.ref_name)}>
          {r.ref_name.startsWith("refs/tags/") ? `⌂ ${shortRef(r.ref_name)}` : shortRef(r.ref_name)}
        </option>
      ))}
    </select>
  );
}

const MemoizedRefSelect = memo(RefSelect);

export default function TreeView({
  cfg,
  store,
  refs,
  current,
  path,
  base,
}: {
  cfg: AppConfig;
  store: RepoStore;
  refs: RefInfo[];
  current: RefInfo;
  path: string;
  base: string;
}) {
  const nav = useNavigate();
  const { ready, status, err } = useLoadedRef(cfg, store, current);
  const [items, setItems] = useState<TreeItem[] | null>(null);
  const [head, setHead] = useState<CommitMeta | null>(null);
  const [readme, setReadme] = useState<string | null>(null);
  const [err2, setErr2] = useState("");
  const short = shortRef(current.ref_name);

  useEffect(() => {
    if (!ready) return;
    setItems(null);
    setReadme(null);
    setErr2("");
    (async () => {
      try {
        const list = await store.listTree(current.commit_sha, path);
        setItems(list);
        setHead((await store.log(current.commit_sha, 1))[0] ?? null);
        const md = list.find((i) => i.type === "blob" && /^readme\.md$/i.test(i.name));
        if (md) {
          const bytes = await store.readFile(
            current.commit_sha,
            path ? `${path}/${md.name}` : md.name,
          );
          setReadme(decodeText(bytes));
        }
      } catch (e) {
        setErr2(String(e));
      }
    })();
  }, [ready, path, current.commit_sha, store]);

  const crumbs = path.split("/").filter(Boolean);
  const dirLink = (p: string) => `${base}/tree/${encodeURIComponent(short)}${p ? "/" + p : ""}`;

  const headMsg = head ? head.message.split("\n")[0] : "";
  const headTime = head?.timestamp ? new Date(head.timestamp).toLocaleString() : "";

  return (
    <div>
      <div className="toolbar">
        <MemoizedRefSelect
          refs={refs}
          value={short}
          onChange={(s) => nav(dirLink("").replace(encodeURIComponent(short), encodeURIComponent(s)))}
        />
        <span className="crumbs">
          <Link to={dirLink("")}>{base.split("/")[2]}</Link>
          {crumbs.map((c, i) => (
            <span key={i}>
              {" / "}
              <Link to={dirLink(crumbs.slice(0, i + 1).join("/"))}>{c}</Link>
            </span>
          ))}
        </span>
        {status && !ready && <span className="muted" aria-live="polite">{status}</span>}
      </div>

      {(err || err2) && <div className="error" role="alert">{err || err2}</div>}
      {!items && !err && !err2 && (
        <div className="spinner" aria-live="polite">{status || "loading objects from IPFS…"}</div>
      )}

      {items && (
        <div className="filelist" role="list">
          <div className="row head-row">
            <span style={{ width: 16, flexShrink: 0 }} />
            <span className="name">Name</span>
            <span className="muted-cell" style={{ flex: 1 }}>Last commit</span>
            <span className="muted-cell">Updated</span>
          </div>
          {path && (
            <Link className="row" to={dirLink(crumbs.slice(0, -1).join("/"))} aria-label="go to parent directory">
              <span className="icon"><ChevronLeft size={16} /></span>
              <span className="name">..</span>
              <span className="muted-cell" style={{ flex: 1 }} />
              <span className="muted-cell" />
            </Link>
          )}
          {[...items]
            .sort((a, b) =>
              a.type === b.type ? a.name.localeCompare(b.name) : a.type === "tree" ? -1 : 1,
            )
            .map((it) => {
              const p = path ? `${path}/${it.name}` : it.name;
              const to =
                it.type === "tree"
                  ? dirLink(p)
                  : `${base}/blob/${encodeURIComponent(short)}/${p}`;
              return (
                <Link className="row" key={it.oid + it.name} to={to} role="listitem">
                  <span className="icon">
                    {it.type === "tree" ? <FolderOpen size={16} /> : <File size={16} />}
                  </span>
                  <span className="name">{it.name}</span>
                  <span className="muted-cell ellipsis" style={{ flex: 1 }}>{headMsg}</span>
                  <span className="muted-cell">{headTime}</span>
                </Link>
              );
            })}
        </div>
      )}

      {readme && (
        <div className="readme">
          <div className="readme-head muted">README.md</div>
          <Markdown text={readme} />
        </div>
      )}
    </div>
  );
}
