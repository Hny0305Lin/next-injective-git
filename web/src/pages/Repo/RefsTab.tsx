import { memo } from "react";
import { Link } from "react-router-dom";
import { GitBranch, Tag } from "lucide-react";
import type { RefInfo } from "../../lib/chain";
import { shortRef } from "./useRepoViews";

const RefTable = memo(function RefTable({ refs, base, kind }: { refs: RefInfo[]; base: string; kind: "branch" | "tag" }) {
  return (
    <div className="filelist">
      <div className="row head-row">
        <span style={{ width: 16, flexShrink: 0 }} />
        <span style={{ flex: 1 }}>{kind === "branch" ? "Branch" : "Tag"}</span>
        <span className="muted-cell">Commit</span>
        <span className="muted-cell">Packs</span>
        <span className="muted-cell">Updated</span>
      </div>
      {refs.map((r) => {
        const label = shortRef(r.ref_name);
        const dateStr = r.updated_at ? new Date(r.updated_at).toLocaleString() : "";
        return (
          <div className="row" key={r.ref_name}>
            <span className="icon">
              {kind === "branch" ? <GitBranch size={14} /> : <Tag size={14} />}
            </span>
            <span className="name mono">
              <Link to={`${base}/tree/${encodeURIComponent(label)}`}>{label}</Link>
            </span>
            <span className="muted-cell mono">
              <Link to={`${base}/commit/${r.commit_sha}`}>
                {r.commit_sha.slice(0, 10)}
              </Link>
            </span>
            <span className="muted-cell">{r.pack_uris.length}</span>
            <span className="muted-cell">{dateStr}</span>
          </div>
        );
      })}
    </div>
  );
});

export default function RefsTab({ refs, base }: { refs: RefInfo[]; base: string }) {
  const branches = refs.filter((r) => r.ref_name.startsWith("refs/heads/"));
  const tags = refs.filter((r) => r.ref_name.startsWith("refs/tags/"));

  return (
    <div>
      {branches.length > 0 && (
        <div style={{ marginBottom: 20 }}>
          <div className="dash-section-title" style={{ marginBottom: 8 }}>Branches ({branches.length})</div>
          <RefTable refs={branches} base={base} kind="branch" />
        </div>
      )}
      {tags.length > 0 && (
        <div>
          <div className="dash-section-title" style={{ marginBottom: 8 }}>Tags ({tags.length})</div>
          <RefTable refs={tags} base={base} kind="tag" />
        </div>
      )}
      {branches.length === 0 && tags.length === 0 && (
        <div className="muted" style={{ padding: "16px 0" }}>no branches or tags yet.</div>
      )}
    </div>
  );
}
