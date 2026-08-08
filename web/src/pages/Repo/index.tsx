import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { FileCode2, GitCommit, GitBranch, Users, Copy, Check } from "lucide-react";
import { loadConfig } from "../../lib/chain";
import { showToast } from "../../components/Toast";
import {
  listRefs,
  repoInfo,
  resolveOwner,
  type RefInfo,
  type RepoInfo,
  formatResourceError,
} from "../../lib/chain";
import { getRepoStore } from "../../lib/gitstore";
import TreeView from "./TreeView";
import BlobView from "./BlobView";
import CommitsView from "./CommitsView";
import CommitView from "./CommitView";
import RefsTab from "./RefsTab";
import SponsorsTab from "./SponsorsTab";
import { findRef, parseView, shortRef } from "./useRepoViews";

export default function Repo() {
  const params = useParams();
  const owner = params.owner ?? "";
  const repo = params.repo ?? "";
  const splat = params["*"] ?? "";
  const cfg = useMemo(() => loadConfig(), []);
  const [addr, setAddr] = useState("");
  const [info, setInfo] = useState<RepoInfo | null>(null);
  const [refs, setRefs] = useState<RefInfo[]>([]);
  const [err, setErr] = useState("");
  const [copied, setCopied] = useState(false);
  const [cloneProtocol, setCloneProtocol] = useState<"igit" | "https">("igit");

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
        setErr(formatResourceError(e, "repository"));
      }
    })();
  }, [owner, repo, cfg]);

  if (err) return <div className="error" role="alert">{err}</div>;
  if (!info) return <div className="spinner" aria-live="polite">querying chain…</div>;

  const fallbackRef =
    shortRef(
      refs.find((r) => r.ref_name === `refs/heads/${info.default_branch}`)?.ref_name ??
        refs[0]?.ref_name ??
        "",
    ) || info.default_branch;
  const view = parseView(splat, fallbackRef);
  const base = `/${owner}/${repo}`;
  const store = getRepoStore(`${addr}/${repo}`);
  const current = view.kind === "commit" ? undefined : findRef(refs, view.ref);

  const tab =
    view.kind === "tree" || view.kind === "blob"
      ? "code"
      : view.kind === "commit"
        ? "commits"
        : view.kind;

  const headShort = current?.commit_sha?.slice(0, 8) ?? "—";
  const branchesCount = refs.filter((r) => r.ref_name.startsWith("refs/heads/")).length;
  const tagsCount = refs.filter((r) => r.ref_name.startsWith("refs/tags/")).length;
  const packfilesCount = current?.pack_uris?.length ?? 0;

  return (
    <div>
      <div className="repo-head">
        <div className="repo-head-top">
          <span className="repo-icon">
            <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M2 4.5A2.5 2.5 0 0 1 4.5 2h3.172a2.5 2.5 0 0 1 2.236 1.5H12.5A2.5 2.5 0 0 1 15 6v5.5a2.5 2.5 0 0 1-2.5 2.5h-9A2.5 2.5 0 0 1 1 11.5v-7z"/>
              <path d="M5.5 2v3.5A2.5 2.5 0 0 0 8 8H12.5"/>
            </svg>
          </span>
          <h2>
            <Link to={`/${owner}`} className="owner-link">
              {owner.startsWith("inj1") ? `${owner.slice(0, 12)}…` : owner}
            </Link>
            {" / "}
            <b>{repo}</b>
            {info.moderation_status !== "active" && (
              <span className={`badge ${info.moderation_status}`}>{info.moderation_status}</span>
            )}
          </h2>
        </div>
        {info.description && (
          <p className="repo-desc">{info.description}</p>
        )}
        {info.forked_from && (
          <p className="repo-forked muted">
            forked from <Link to={`/${info.forked_from}`}>{info.forked_from}</Link>
          </p>
        )}
        <div className="repo-stats">
          <span><b>{headShort}</b> HEAD</span>
          <span><b>{branchesCount}</b> branches</span>
          <span><b>{tagsCount}</b> tags</span>
          <span><b>{packfilesCount}</b> packfiles</span>
        </div>
        <div className="repo-actions">
          <div className="clone-box">
            <div className="clone-command">
              <select
                className="clone-protocol-select"
                value={cloneProtocol}
                onChange={(e) => setCloneProtocol(e.target.value as "igit" | "https")}
                aria-label="clone protocol"
              >
                <option value="igit">igit://</option>
                <option value="https">https://</option>
              </select>
              <code title={cloneProtocol === "igit" ? `igit clone igit://${owner}/${repo}` : `${cloneProtocol}://${owner}/${repo}`}>
                {cloneProtocol === "igit" ? `igit clone igit://${owner}/${repo}` : `${cloneProtocol}://${owner}/${repo}`}
              </code>
            </div>
            <button
              className="btn"
              style={{ padding: "3px 8px", fontSize: "0.78rem" }}
              onClick={async () => {
                const url = cloneProtocol === "igit"
                  ? `igit clone igit://${owner}/${repo}`
                  : `${cloneProtocol}://${owner}/${repo}`;
                try {
                  await navigator.clipboard.writeText(url);
                  setCopied(true);
                  showToast("Clone URL copied");
                  setTimeout(() => setCopied(false), 1500);
                } catch {
                  showToast("Could not copy clone URL");
                }
              }}
              title="copy clone URL"
              aria-label={copied ? "clone URL copied" : "copy clone URL"}
            >
              {copied ? <Check size={14} /> : <Copy size={14} />}
            </button>
          </div>
        </div>
      </div>

      <div className="tabs" role="tablist">
        <Link className={tab === "code" ? "on" : ""} to={base} role="tab">
          <FileCode2 size={14} /> Code
        </Link>
        <Link
          className={tab === "commits" ? "on" : ""}
          to={`${base}/commits/${encodeURIComponent(fallbackRef)}`}
          role="tab"
        >
          <GitCommit size={14} /> Commits
        </Link>
        <Link className={tab === "refs" ? "on" : ""} to={`${base}/refs`} role="tab">
          <GitBranch size={14} /> Refs
        </Link>
        <Link className={tab === "sponsors" ? "on" : ""} to={`${base}/sponsors`} role="tab">
          <Users size={14} /> Sponsors
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
        <div className="error" role="alert">ref not found: {view.ref}</div>
      )}
    </div>
  );
}
