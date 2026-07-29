import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useWallet } from "../lib/WalletContext";
import { sponsorWithKeplr } from "../lib/wallet";
import {
  badgesByRepo,
  formatFunds,
  formatInj,
  listCollaborators,
  listRefs,
  loadConfig,
  repoInfo,
  resolveOwner,
  revenueSplits,
  sponsorEvents,
  sponsorTotals,
  timeAgo,
  type AppConfig,
  type BadgeInfo,
  type CollaboratorInfo,
  type RefInfo,
  type RepoInfo,
  type SplitEntry,
  type SponsorEvent,
  type SponsorTotal,
} from "../lib/chain";
import {
  decodeText,
  getRepoStore,
  type CommitMeta,
  type FileChange,
  type RepoStore,
  type TreeItem,
} from "../lib/gitstore";
import { Markdown } from "../components/Markdown";
import { CodeBlock } from "../components/CodeBlock";

// ---- URL helpers: GitHub-style deep links -------------------------------
// /:owner/:repo                        -> tree @ default branch, root
// /:owner/:repo/tree/<ref>/<path...>   -> directory listing
// /:owner/:repo/blob/<ref>/<path...>   -> file view
// /:owner/:repo/commits/<ref>          -> history
// /:owner/:repo/commit/<sha>           -> single commit diff
// /:owner/:repo/refs | /sponsors       -> tables

function shortRef(refName: string): string {
  if (refName.startsWith("refs/heads/")) return refName.slice("refs/heads/".length);
  if (refName.startsWith("refs/")) return refName.slice("refs/".length);
  return refName;
}

function findRef(refs: RefInfo[], short: string): RefInfo | undefined {
  return (
    refs.find((r) => r.ref_name === `refs/heads/${short}`) ??
    refs.find((r) => r.ref_name === `refs/${short}`) ??
    refs.find((r) => r.ref_name === short)
  );
}

interface View {
  kind: "tree" | "blob" | "commits" | "commit" | "refs" | "sponsors";
  ref: string; // short ref name or commit sha (for kind=commit)
  path: string;
}

function parseView(splat: string, fallbackRef: string): View {
  const parts = splat.split("/").filter(Boolean);
  const kind = parts[0] ?? "";
  switch (kind) {
    case "tree":
    case "blob":
      return {
        kind,
        ref: decodeURIComponent(parts[1] ?? fallbackRef),
        path: parts.slice(2).map(decodeURIComponent).join("/"),
      };
    case "commits":
      return { kind, ref: decodeURIComponent(parts[1] ?? fallbackRef), path: "" };
    case "commit":
      return { kind, ref: parts[1] ?? "", path: "" };
    case "refs":
    case "sponsors":
      return { kind, ref: fallbackRef, path: "" };
    default:
      return { kind: "tree", ref: fallbackRef, path: "" };
  }
}

export default function Repo() {
  const params = useParams();
  const owner = params.owner ?? "";
  const repo = params.repo ?? "";
  const splat = params["*"] ?? "";
  const cfg = useMemo(loadConfig, []);
  const [addr, setAddr] = useState("");
  const [info, setInfo] = useState<RepoInfo | null>(null);
  const [refs, setRefs] = useState<RefInfo[]>([]);
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

  const fallbackRef =
    shortRef(
      refs.find((r) => r.ref_name === `refs/heads/${info.default_branch}`)?.ref_name ??
        refs[0]?.ref_name ??
        "",
    ) || info.default_branch;
  const view = parseView(splat, fallbackRef);
  const base = `/${owner}/${repo}`;
  const cloneURL = `inj://${owner}/${repo}`;
  const store = getRepoStore(`${addr}/${repo}`);
  const current = view.kind === "commit" ? undefined : findRef(refs, view.ref);

  const tab =
    view.kind === "tree" || view.kind === "blob"
      ? "code"
      : view.kind === "commit"
        ? "commits"
        : view.kind;

  return (
    <div>
      <div className="repo-head">
        <h2>
          <Link to={`/${owner}`}>
            {owner.startsWith("inj1") ? `${owner.slice(0, 12)}…` : owner}
          </Link>{" "}
          / <b>{repo}</b>{" "}
          <span className={`badge ${info.moderation_status}`}>{info.moderation_status}</span>
        </h2>
        <p className="muted" style={{ margin: "6px 0" }}>
          {info.description || <i>no description</i>} · created {timeAgo(info.created_at)} ·
          updated {timeAgo(info.updated_at)}
        </p>
        {info.forked_from && (
          <p className="muted" style={{ margin: "2px 0 8px" }}>
            ⥂ forked from <Link to={`/${info.forked_from}`}>{info.forked_from}</Link>
          </p>
        )}
        <div className="clone-box">
          <code>git clone {cloneURL}</code>
          <button onClick={() => navigator.clipboard.writeText(`git clone ${cloneURL}`)}>
            copy
          </button>
        </div>
      </div>

      <div className="tabs">
        <Link className={tab === "code" ? "on" : ""} to={base}>
          code
        </Link>
        <Link
          className={tab === "commits" ? "on" : ""}
          to={`${base}/commits/${encodeURIComponent(fallbackRef)}`}
        >
          commits
        </Link>
        <Link className={tab === "refs" ? "on" : ""} to={`${base}/refs`}>
          refs
        </Link>
        <Link className={tab === "sponsors" ? "on" : ""} to={`${base}/sponsors`}>
          sponsors
        </Link>
      </div>

      {refs.length === 0 && tab === "code" ? (
        <p className="muted">empty repository — push something first.</p>
      ) : view.kind === "tree" && current ? (
        <TreeView cfg={cfg} store={store} refs={refs} current={current} path={view.path} base={base} />
      ) : view.kind === "blob" && current ? (
        <BlobView cfg={cfg} store={store} current={current} path={view.path} base={base} />
      ) : view.kind === "commits" && current ? (
        <CommitsView cfg={cfg} store={store} refs={refs} current={current} base={base} />
      ) : view.kind === "commit" ? (
        <CommitView cfg={cfg} store={store} refs={refs} sha={view.ref} base={base} />
      ) : view.kind === "refs" ? (
        <RefsTab refs={refs} base={base} />
      ) : view.kind === "sponsors" ? (
        <SponsorsTab cfg={cfg} addr={addr} repo={repo} owner={owner} />
      ) : (
        <div className="error">ref not found: {view.ref}</div>
      )}
    </div>
  );
}

// ---- branch/tag selector --------------------------------------------------

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
    <select value={value} onChange={(e) => onChange(e.target.value)}>
      {refs.map((r) => (
        <option key={r.ref_name} value={shortRef(r.ref_name)}>
          {r.ref_name.startsWith("refs/tags/") ? `⌂ ${shortRef(r.ref_name)}` : shortRef(r.ref_name)}
        </option>
      ))}
    </select>
  );
}

function useLoadedRef(cfg: AppConfig, store: RepoStore, current: RefInfo) {
  const [ready, setReady] = useState(false);
  const [status, setStatus] = useState("");
  const [err, setErr] = useState("");
  useEffect(() => {
    setReady(false);
    setErr("");
    store
      .loadRef(cfg, current, setStatus)
      .then(() => setReady(true))
      .catch((e) => setErr(String(e)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current.ref_name, current.commit_sha]);
  return { ready, status, err };
}

// ---- tree ------------------------------------------------------------------

function TreeView({
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, path, current.commit_sha]);

  const crumbs = path.split("/").filter(Boolean);
  const dirLink = (p: string) => `${base}/tree/${encodeURIComponent(short)}${p ? "/" + p : ""}`;

  return (
    <div>
      <div className="toolbar">
        <RefSelect refs={refs} value={short} onChange={(s) => nav(dirLink("").replace(encodeURIComponent(short), encodeURIComponent(s)))} />
        <span className="crumbs">
          <Link to={dirLink("")}>{base.split("/")[2]}</Link>
          {crumbs.map((c, i) => (
            <span key={i}>
              {" / "}
              <Link to={dirLink(crumbs.slice(0, i + 1).join("/"))}>{c}</Link>
            </span>
          ))}
        </span>
        {status && !ready && <span className="muted">{status}</span>}
      </div>

      {(err || err2) && <div className="error">{err || err2}</div>}
      {!items && !err && !err2 && (
        <div className="spinner">{status || "loading objects from IPFS…"}</div>
      )}

      {items && (
        <div className="filelist">
          {head && (
            <div className="row head-row">
              <span className="icon">◉</span>
              <Link to={`${base}/commit/${head.oid}`} className="mono sha">
                {head.oid.slice(0, 8)}
              </Link>
              <span className="ellipsis" style={{ flex: 1 }}>
                {head.message.split("\n")[0]}
              </span>
              <span className="muted">{head.author}</span>
              <span className="muted">{timeAgo(head.timestamp)}</span>
            </div>
          )}
          {path && (
            <Link className="row" to={dirLink(crumbs.slice(0, -1).join("/"))}>
              <span className="icon">↩</span>
              <span>..</span>
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
                <Link className="row" key={it.oid + it.name} to={to}>
                  <span className="icon">{it.type === "tree" ? "📁" : "📄"}</span>
                  <span>{it.name}</span>
                </Link>
              );
            })}
        </div>
      )}

      {readme && (
        <div className="readme card">
          <div className="readme-head muted">README.md</div>
          <Markdown text={readme} />
        </div>
      )}
    </div>
  );
}

// ---- blob ------------------------------------------------------------------

function BlobView({
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
  const { ready, status, err } = useLoadedRef(cfg, store, current);
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, path, current.commit_sha]);

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
        <span className="muted">{data ? `${data.size} bytes` : status}</span>
      </div>
      {(err || err2) && <div className="error">{err || err2}</div>}
      {!data && !err && !err2 && <div className="spinner">loading…</div>}
      {data &&
        (data.text === null ? (
          <div className="card muted">binary file ({data.size} bytes) — not rendered</div>
        ) : /\.(md|markdown)$/i.test(fileName) ? (
          <div className="readme card">
            <Markdown text={data.text} />
          </div>
        ) : (
          <CodeBlock code={data.text} fileName={fileName} />
        ))}
    </div>
  );
}

// ---- commits ----------------------------------------------------------------

function CommitsView({
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, current.commit_sha]);

  return (
    <div>
      <div className="toolbar">
        <RefSelect
          refs={refs}
          value={short}
          onChange={(s) => nav(`${base}/commits/${encodeURIComponent(s)}`)}
        />
      </div>
      {err && <div className="error">{err}</div>}
      {!commits && !err && <div className="spinner">{status || "reconstructing history…"}</div>}
      {commits?.map((c) => (
        <div className="commit" key={c.oid}>
          <Link to={`${base}/commit/${c.oid}`} className="mono sha">
            {c.oid.slice(0, 8)}
          </Link>
          <span style={{ flex: 1 }}>{c.message.split("\n")[0]}</span>
          <span className="muted">{c.author}</span>
          <span className="muted">{timeAgo(c.timestamp)}</span>
        </div>
      ))}
    </div>
  );
}

// ---- single commit diff -------------------------------------------------------

function CommitView({
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
        // the commit may live in any ref's packs: load them all (cached)
        for (const r of refs) await store.loadRef(cfg, r);
        setMeta(await store.commitMeta(sha));
        setChanges(await store.diffCommit(sha));
      } catch (e) {
        setErr(String(e));
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sha]);

  if (err) return <div className="error">{err}</div>;
  if (!meta) return <div className="spinner">loading commit…</div>;

  return (
    <div>
      <div className="card" style={{ marginBottom: 16 }}>
        <b>{meta.message.split("\n")[0]}</b>
        <div className="muted" style={{ marginTop: 6 }}>
          <code className="mono">{meta.oid}</code> · {meta.author} · {timeAgo(meta.timestamp)}
          {meta.parents.length > 0 && (
            <>
              {" · parent "}
              <Link to={`${base}/commit/${meta.parents[0]}`} className="mono">
                {meta.parents[0].slice(0, 8)}
              </Link>
            </>
          )}
        </div>
      </div>
      {!changes && <div className="spinner">computing diff…</div>}
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
  // drop the file header lines, keep hunks
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
          <div key={i} className={`diff-line ${cls}`}>
            {l || " "}
          </div>
        );
      })}
    </pre>
  );
}

// ---- refs / sponsors (unchanged data, richer links) ----------------------------

function RefsTab({ refs, base }: { refs: RefInfo[]; base: string }) {
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
            <td>
              <Link to={`${base}/tree/${encodeURIComponent(shortRef(r.ref_name))}`}>
                <code>{r.ref_name}</code>
              </Link>
            </td>
            <td>
              <Link to={`${base}/commit/${r.commit_sha}`}>
                <code>{r.commit_sha.slice(0, 10)}</code>
              </Link>
            </td>
            <td>{r.pack_uris.length}</td>
            <td>{timeAgo(r.updated_at)}</td>
            <td>
              <code>{r.updated_by.slice(0, 12)}…</code>
            </td>
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
  cfg: AppConfig;
  addr: string;
  repo: string;
  owner: string;
}) {
  const [totals, setTotals] = useState<SponsorTotal[] | null>(null);
  const [splits, setSplits] = useState<SplitEntry[]>([]);
  const [collabs, setCollabs] = useState<CollaboratorInfo[]>([]);
  const [events, setEvents] = useState<SponsorEvent[]>([]);
  const [badges, setBadges] = useState<BadgeInfo[]>([]);
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
        // best-effort: the tx index may lag or be pruned
        setEvents(await sponsorEvents(cfg, addr, repo).catch(() => []));
        setBadges(await badgesByRepo(cfg, addr, repo).catch(() => []));
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
      <SponsorForm cfg={cfg} addr={addr} repo={repo} />

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

      {events.length > 0 && (
        <>
          <h3>Sponsor wall</h3>
          {events.map((e) => (
            <div className="card sponsor-entry" key={e.txhash}>
              <div>
                <b>{formatFunds(e.funds)}</b>{" "}
                <span className="muted">
                  from <code>{e.sponsor.slice(0, 14)}…</code> ·{" "}
                  {e.timestamp.slice(0, 16).replace("T", " ")}
                </span>
              </div>
              {e.message && <div className="sponsor-msg">“{e.message}”</div>}
            </div>
          ))}
        </>
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
        <div
          style={{ width: `${(10000 - splitTotal) / 100}%`, background: "#6e7681" }}
          title="owner"
        />
      </div>
      <table className="plain">
        <tbody>
          {splits.map((s, i) => (
            <tr key={s.address}>
              <td>
                <span style={{ color: BAR_COLORS[i % BAR_COLORS.length] }}>■</span>
              </td>
              <td>
                <code>{s.address}</code>
              </td>
              <td>{s.bps / 100}%</td>
            </tr>
          ))}
          <tr>
            <td>
              <span style={{ color: "#6e7681" }}>■</span>
            </td>
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
                  <td>
                    <code>{c.address}</code>
                  </td>
                  <td>{c.role}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      {badges.length > 0 && (
        <>
          <h3>🏆 Badges awarded</h3>
          {badges.map((b) => (
            <div className="card sponsor-entry" key={b.id}>
              <div>
                <b>#{b.id}</b>{" "}
                <span className="muted">
                  to <Link to={`/${b.recipient}`}>{b.recipient.slice(0, 14)}…</Link> ·{" "}
                  {timeAgo(b.awarded_at)}
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

// In-browser sponsorship: connect Keplr, sign a Sponsor tx, funds split
// instantly in the contract. Turns the read-only wall into a write action.
function SponsorForm({
  cfg,
  addr,
  repo,
}: {
  cfg: AppConfig;
  addr: string;
  repo: string;
}) {
  const { wallet, connect, connecting, available, refreshBalance } = useWallet();
  const [amount, setAmount] = useState("0.1");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [txhash, setTxhash] = useState("");
  const [err, setErr] = useState("");

  const isOwnRepo = wallet?.address === addr;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!wallet) return;
    setBusy(true);
    setErr("");
    setTxhash("");
    try {
      const hash = await sponsorWithKeplr(wallet, cfg, addr, repo, amount, message.trim());
      setTxhash(hash);
      setMessage("");
      void refreshBalance();
    } catch (e2) {
      setErr(String(e2 instanceof Error ? e2.message : e2));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="card sponsor-form">
      <div className="sponsor-form-title">💚 Sponsor this repository</div>
      {!wallet ? (
        <div className="sponsor-connect">
          <span className="muted">Connect a wallet to sponsor directly in the browser.</span>
          {available.length <= 1 ? (
            <button
              onClick={() => connect(available[0]?.id ?? "keplr")}
              disabled={connecting}
            >
              {connecting ? "connecting…" : `Connect ${available[0]?.label ?? "Wallet"}`}
            </button>
          ) : (
            available.map((w) => (
              <button key={w.id} onClick={() => connect(w.id)} disabled={connecting}>
                {w.label}
              </button>
            ))
          )}
        </div>
      ) : isOwnRepo ? (
        <span className="muted">This is your own repository — sponsorships come from others.</span>
      ) : (
        <form className="sponsor-fields" onSubmit={submit}>
          <input
            className="field amount"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            inputMode="decimal"
            aria-label="amount in INJ"
          />
          <span className="muted">INJ</span>
          <input
            className="field"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder="message for the sponsor wall (optional)"
            maxLength={256}
          />
          <button type="submit" disabled={busy}>
            {busy ? "signing…" : "Sponsor"}
          </button>
        </form>
      )}
      {err && <div className="error" style={{ marginTop: 10 }}>{err}</div>}
      {txhash && (
        <div className="sponsor-ok">
          ✅ sponsored! tx <code>{txhash.slice(0, 12)}…</code> — the split settled on-chain. Reload
          to see it on the wall.
        </div>
      )}
    </div>
  );
}
