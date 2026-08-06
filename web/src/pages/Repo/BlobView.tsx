import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { FileText } from "lucide-react";
import { useLoadedRef, shortRef } from "./useRepoViews";
import type { AppConfig, RefInfo } from "../../lib/chain";
import type { RepoStore } from "../../lib/gitstore";
import { Markdown } from "../../components/Markdown";
import { decodeText } from "../../lib/gitstore";

export default function BlobView({
  cfg,
  store,
  current,
  path,
  base,
}: {
  cfg: AppConfig;
  store: RepoStore;
  current: RefInfo;
  path: string;
  base: string;
}) {
  const { ready, err } = useLoadedRef(cfg, store, current);
  const [data, setData] = useState<{ text: string | null; size: number } | null>(null);
  const [err2, setErr2] = useState("");
  const short = shortRef(current.ref_name);

  useEffect(() => {
    if (!ready) return;
    setData(null);
    store
      .readFile(current.commit_sha, path)
      .then((bytes) => setData({ text: decodeText(bytes), size: bytes.length }))
      .catch((e) => setErr2(String(e)));
  }, [ready, path, current.commit_sha, store]);

  const crumbs = path.split("/").filter(Boolean);
  const fileName = crumbs[crumbs.length - 1] ?? "";
  const dirLink = (p: string) => `${base}/tree/${encodeURIComponent(short)}${p ? "/" + p : ""}`;

  return (
    <div>
      <div className="toolbar">
        <span className="crumbs">
          <Link to={dirLink("")}>{base.split("/")[2]}</Link>
          {crumbs.slice(0, -1).map((c, i) => (
            <span key={i}>
              {" / "}
              <Link to={dirLink(crumbs.slice(0, i + 1).join("/"))}>{c}</Link>
            </span>
          ))}
          <span> / <b>{fileName}</b></span>
        </span>
        {data && <span className="muted" style={{ fontSize: "0.78rem" }}>{data.size.toLocaleString()} bytes</span>}
      </div>

      {(err || err2) && <div className="error" role="alert">{err || err2}</div>}
      {!data && !err && !err2 && <div className="spinner" aria-live="polite">loading…</div>}

      {data && data.text === null && (
        <div className="card muted" style={{ padding: "20px", textAlign: "center" }}>
          <FileText size={24} style={{ opacity: 0.4, marginBottom: 8 }} />
          <p style={{ margin: 0 }}>binary file ({data.size.toLocaleString()} bytes) — not rendered</p>
        </div>
      )}

      {data && data.text !== null && /\.(md|markdown)$/i.test(fileName) && (
        <div className="readme">
          <div className="readme-head">
            <FileText size={14} style={{ verticalAlign: "-2px", marginRight: 6 }} />
            {fileName}
          </div>
          <Markdown text={data.text} />
        </div>
      )}

      {data && data.text !== null && !/\.(md|markdown)$/i.test(fileName) && (
        <div className="fileview">
          <pre>{data.text}</pre>
        </div>
      )}
    </div>
  );
}
