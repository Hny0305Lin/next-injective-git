import { useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import {
  formatInj,
  listCollaborators,
  listRefs,
  loadConfig,
  repoInfo,
  resolveOwner,
  revenueSplits,
  sponsorTotals,
  timeAgo,
  type CollaboratorInfo,
  type RefInfo,
  type RepoInfo,
  type SplitEntry,
  type SponsorTotal,
} from "../lib/chain";
import { decodeText, RepoStore, type CommitMeta, type TreeItem } from "../lib/gitstore";

type Tab = "code" | "commits" | "refs" | "sponsors";

export default function Repo() {
  const { owner = "", repo = "" } = useParams();
  const cfg = useMemo(loadConfig, []);
  const [addr, setAddr] = useState("");
  const [info, setInfo] = useState<RepoInfo | null>(null);
  const [refs, setRefs] = useState<RefInfo[]>([]);
  const [tab, setTab] = useState<Tab>("code");
  const [err, setErr] = useState("");

  useEffect(() => {
    setErr("");
    setInfo(null);
    (async () => {
      try {
        const a = await resolveOwner(cfg, owner);
        setAddr(a);
        const [ri, rf] = await Promise.all([repoInfo(cfg, a, repo), listRefs(cfg, a, repo)]);
        setInfo(ri);
        setRefs(rf);
      } catch (e) {
        setErr(String(e));
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [owner, repo]);

  if (err) return <div className="error">{err}</div>;
  if (!info) return <div className="spinner">querying chain…</div>;

  const cloneURL = `inj://${owner}/${repo}`;

  return (
    <div>
      <div className="repo-head">
        <h2>
          <a href={`#/${owner}`}>{owner.startsWith("inj1") ? `${owner.slice(0, 12)}…` : owner}</a>{" "}
          / <b>{repo}</b>{" "}
          <span className={`badge ${info.moderation_status}`}>{info.moderation_status}</span>
        </h2>
        <p className="muted" style={{ margin: "6px 0" }}>
          {info.description || <i>no description</i>} · created {timeAgo(info.created_at)} ·
          updated {timeAgo(info.updated_at)}
        </p>
        <div className="clone-box">
          <code>git clone {cloneURL}</code>
          <button onClick={() => navigator.clipboard.writeText(`git clone ${cloneURL}`)}>
            copy
          </button>
        </div>
      </div>

      <div className="tabs">
        {(["code", "commits", "refs", "sponsors"] as Tab[]).map((t) => (
          <button key={t} className={tab === t ? "on" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>

      {tab === "code" && <CodeTab cfg={cfg} refs={refs} info={info} storeKey={`${addr}/${repo}`} />}
      {tab === "commits" && (
        <CommitsTab cfg={cfg} refs={refs} info={info} storeKey={`${addr}/${repo}`} />
      )}
      {tab === "refs" && <RefsTab refs={refs} />}
      {tab === "sponsors" && <SponsorsTab cfg={cfg} addr={addr} repo={repo} owner={owner} />}
    </div>
  );
}

// pick the default branch's ref, else the first one
function defaultRef(refs: RefInfo[], info: RepoInfo): RefInfo | undefined {
  return refs.find((r) => r.ref_name === `refs/heads/${info.default_branch}`) ?? refs[0];
}

function useStore(storeKey: string) {
  const ref = useRef<RepoStore | null>(null);
  if (!ref.current) ref.current = new RepoStore(`igit-${storeKey}`);
  return ref.current;
}

function CodeTab({
  cfg,
  refs,
  info,
  storeKey,
}: {
  cfg: ReturnType<typeof loadConfig>;
  refs: RefInfo[];
  info: RepoInfo;
  storeKey: string;
}) {
  const store = useStore(storeKey);
  const [sel, setSel] = useState(() => defaultRef(refs, info)?.ref_name ?? "");
  const [path, setPath] = useState("");
  const [items, setItems] = useState<TreeItem[] | null>(null);
  const [file, setFile] = useState<{ path: string; text: string | null; size: number } | null>(
    null,
  );
  const [status, setStatus] = useState("");
  const [err, setErr] = useState("");

  const current = refs.find((r) => r.ref_name === sel);

  useEffect(() => {
    if (!current) return;
    setErr("");
    setItems(null);
    setFile(null);
    (async () => {
      try {
        await store.loadRef(cfg, current, setStatus);
        setStatus("");
        setItems(await store.listTree(current.commit_sha, path));
      } catch (e) {
        setErr(String(e));
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel, path]);

  const open = async (item: TreeItem) => {
    if (!current) return;
    const next = path ? `${path}/${item.name}` : item.name;
    if (item.type === "tree") {
      setPath(next);
    } else {
      const bytes = await store.readFile(current.commit_sha, next);
      setFile({ path: next, text: decodeText(bytes), size: bytes.length });
    }
  };

  if (refs.length === 0) return <p className="muted">empty repository — push something first.</p>;

  const crumbs = path.split("/").filter(Boolean);

  return (
    <div>
      <div className="toolbar">
        <select value={sel} onChange={(e) => { setSel(e.target.value); setPath(""); }}>
          {refs.map((r) => (
            <option key={r.ref_name} value={r.ref_name}>
              {r.ref_name.replace("refs/heads/", "").replace("refs/tags/", "tag: ")}
            </option>
          ))}
        </select>
        <span className="crumbs">
          <a onClick={() => { setPath(""); setFile(null); }} style={{ cursor: "pointer" }}>
            {info.name}
          </a>
          {crumbs.map((c, i) => (
            <span key={i}>
              {" / "}
              <a
                style={{ cursor: "pointer" }}
                onClick={() => { setPath(crumbs.slice(0, i + 1).join("/")); setFile(null); }}
              >
                {c}
              </a>
            </span>
          ))}
          {file && <span> / <b>{file.path.split("/").pop()}</b></span>}
        </span>
        {status && <span className="muted">{status}</span>}
      </div>

      {err && <div className="error">{err}</div>}

      {file ? (
        <div className="fileview">
          <pre>
            {file.text ?? `binary file (${file.size} bytes) — not rendered`}
          </pre>
        </div>
      ) : items ? (
        <div className="filelist">
          {path && (
            <div className="row" onClick={() => setPath(crumbs.slice(0, -1).join("/"))}>
              <span className="icon">↩</span>
              <span>..</span>
            </div>
          )}
          {[...items]
            .sort((a, b) =>
              a.type === b.type ? a.name.localeCompare(b.name) : a.type === "tree" ? -1 : 1,
            )
            .map((it) => (
              <div className="row" key={it.oid + it.name} onClick={() => open(it)}>
                <span className="icon">{it.type === "tree" ? "▸" : "·"}</span>
                <span>{it.name}</span>
              </div>
            ))}
        </div>
      ) : (
        !err && <div className="spinner">{status || "loading objects from IPFS…"}</div>
      )}
    </div>
  );
}

function CommitsTab({
  cfg,
  refs,
  info,
  storeKey,
}: {
  cfg: ReturnType<typeof loadConfig>;
  refs: RefInfo[];
  info: RepoInfo;
  storeKey: string;
}) {
  const store = useStore(`${storeKey}-log`);
  const [sel, setSel] = useState(() => defaultRef(refs, info)?.ref_name ?? "");
  const [commits, setCommits] = useState<CommitMeta[] | null>(null);
  const [err, setErr] = useState("");
  const current = refs.find((r) => r.ref_name === sel);

  useEffect(() => {
    if (!current) return;
    setCommits(null);
    setErr("");
    (async () => {
      try {
        await store.loadRef(cfg, current);
        setCommits(await store.log(current.commit_sha));
      } catch (e) {
        setErr(String(e));
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel]);

  if (refs.length === 0) return <p className="muted">empty repository.</p>;
  return (
    <div>
      <div className="toolbar">
        <select value={sel} onChange={(e) => setSel(e.target.value)}>
          {refs.map((r) => (
            <option key={r.ref_name} value={r.ref_name}>
              {r.ref_name.replace("refs/heads/", "").replace("refs/tags/", "tag: ")}
            </option>
          ))}
        </select>
      </div>
      {err && <div className="error">{err}</div>}
      {!commits && !err && <div className="spinner">reconstructing history…</div>}
      {commits?.map((c) => (
        <div className="commit" key={c.oid}>
          <code className="sha">{c.oid.slice(0, 8)}</code>
          <span style={{ flex: 1 }}>{c.message.split("\n")[0]}</span>
          <span className="muted">{c.author}</span>
          <span className="muted">{timeAgo(c.timestamp)}</span>
        </div>
      ))}
    </div>
  );
}

function RefsTab({ refs }: { refs: RefInfo[] }) {
  return (
    <table className="plain">
      <thead>
        <tr>
          <th>ref</th>
          <th>commit</th>
          <th>packs</th>
          <th>updated</th>
          <th>by</th>
        </tr>
      </thead>
      <tbody>
        {refs.map((r) => (
          <tr key={r.ref_name}>
            <td><code>{r.ref_name}</code></td>
            <td><code>{r.commit_sha.slice(0, 10)}</code></td>
            <td>{r.pack_uris.length}</td>
            <td>{timeAgo(r.updated_at)}</td>
            <td><code>{r.updated_by.slice(0, 12)}…</code></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

const BAR_COLORS = ["#4493f8", "#3fb950", "#d29922", "#a371f7", "#f85149", "#39c5cf"];

function SponsorsTab({
  cfg,
  addr,
  repo,
  owner,
}: {
  cfg: ReturnType<typeof loadConfig>;
  addr: string;
  repo: string;
  owner: string;
}) {
  const [totals, setTotals] = useState<SponsorTotal[] | null>(null);
  const [splits, setSplits] = useState<SplitEntry[]>([]);
  const [collabs, setCollabs] = useState<CollaboratorInfo[]>([]);
  const [err, setErr] = useState("");

  useEffect(() => {
    (async () => {
      try {
        const [t, s, c] = await Promise.all([
          sponsorTotals(cfg, addr, repo),
          revenueSplits(cfg, addr, repo),
          listCollaborators(cfg, addr, repo),
        ]);
        setTotals(t);
        setSplits(s);
        setCollabs(c);
      } catch (e) {
        setErr(String(e));
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addr, repo]);

  if (err) return <div className="error">{err}</div>;
  if (!totals) return <div className="spinner">querying chain…</div>;

  const splitTotal = splits.reduce((a, s) => a + s.bps, 0);

  return (
    <div>
      <h3>Lifetime sponsorship</h3>
      {totals.length === 0 ? (
        <p className="muted">
          no sponsorships yet — be the first: <code>igit sponsor {owner} {repo} 0.1</code>
        </p>
      ) : (
        totals.map((t) => (
          <div key={t.denom} className="card" style={{ marginBottom: 8 }}>
            <b>{formatInj(t.amount, t.denom)}</b> <span className="muted">total received</span>
          </div>
        ))
      )}

      <h3>Revenue split</h3>
      <div className="split-bar">
        {splits.map((s, i) => (
          <div
            key={s.address}
            style={{ width: `${s.bps / 100}%`, background: BAR_COLORS[i % BAR_COLORS.length] }}
            title={`${s.address} ${s.bps / 100}%`}
          />
        ))}
        <div style={{ width: `${(10000 - splitTotal) / 100}%`, background: "#6e7681" }} title="owner" />
      </div>
      <table className="plain">
        <tbody>
          {splits.map((s, i) => (
            <tr key={s.address}>
              <td><span style={{ color: BAR_COLORS[i % BAR_COLORS.length] }}>■</span></td>
              <td><code>{s.address}</code></td>
              <td>{s.bps / 100}%</td>
            </tr>
          ))}
          <tr>
            <td><span style={{ color: "#6e7681" }}>■</span></td>
            <td>owner (remainder)</td>
            <td>{(10000 - splitTotal) / 100}%</td>
          </tr>
        </tbody>
      </table>

      {collabs.length > 0 && (
        <>
          <h3>Collaborators</h3>
          <table className="plain">
            <tbody>
              {collabs.map((c) => (
                <tr key={c.address}>
                  <td><code>{c.address}</code></td>
                  <td>{c.role}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  );
}
