import { Archive as ArchiveIcon, GitBranch, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ContractTypeBadge } from "../components/ContractTypeBadge";
import {
  listCosmWasmV1Repos,
  formatCosmWasmV1Error,
  resolveCosmWasmV1Owner,
} from "../lib/cosmwasm-v1";
import { timeAgo, type RepoInfo } from "../lib/chain";

export default function ArchiveOwner() {
  const { owner = "" } = useParams();
  const [address, setAddress] = useState("");
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [filter, setFilter] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    setRepos(null);
    setError("");
    let cancelled = false;
    void (async () => {
      try {
        const resolved = await resolveCosmWasmV1Owner(owner);
        const items = await listCosmWasmV1Repos(resolved);
        if (!cancelled) {
          setAddress(resolved);
          setRepos(items);
        }
      } catch (cause) {
        if (!cancelled) setError(formatCosmWasmV1Error(cause, "owner"));
      }
    })();
    return () => { cancelled = true; };
  }, [owner]);

  const filtered = useMemo(() => {
    const query = filter.trim().toLowerCase();
    if (!repos || !query) return repos ?? [];
    return repos.filter((item) =>
      item.name.toLowerCase().includes(query) || item.description.toLowerCase().includes(query),
    );
  }, [filter, repos]);

  if (error) return <div className="error" role="alert">{error}</div>;
  if (!repos) return <div className="spinner" aria-live="polite">querying archive...</div>;

  return (
    <div className="archive-owner-page">
      <div className="archive-owner-head">
        <div className="archive-title-line">
          <ArchiveIcon size={20} aria-hidden="true" />
          <h1>{owner.startsWith("inj1") ? `${owner.slice(0, 13)}...${owner.slice(-6)}` : `@${owner}`}</h1>
          <ContractTypeBadge kind="cosmwasm-v1" />
        </div>
        <code title={address}>{address}</code>
      </div>

      <div className="dash-search archive-owner-search">
        <Search size={15} aria-hidden="true" />
        <input
          className="field"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder="Find an archived repository..."
          aria-label="Find an archived repository"
        />
      </div>

      {filtered.length === 0 ? (
        <div className="surface-panel empty-state">
          <GitBranch size={22} />
          <h3>{repos.length === 0 ? "No archived repositories" : "No matching repositories"}</h3>
          <p>{repos.length === 0 ? "This owner has no repositories in the V1 contract." : "Try a different repository name."}</p>
        </div>
      ) : (
        <div className="surface-panel repo-list">
          {filtered.map((item) => (
            <div className="repo-list-item" key={item.name}>
              <div className="repo-list-icon"><GitBranch size={15} /></div>
              <div className="repo-list-content">
                <h3>
                  <Link to={`/archive/cosmwasm-v1/${encodeURIComponent(address)}/${encodeURIComponent(item.name)}`}>{item.name}</Link>
                  <ContractTypeBadge kind="cosmwasm-v1" />
                  <span className={`badge ${item.moderation_status}`}>{item.moderation_status}</span>
                  {item.forked_from && <span className="badge">fork</span>}
                </h3>
                <div className="meta muted">
                  {item.description || <i>No description</i>}
                  <span>Default <code>{item.default_branch}</code></span>
                  <span>Updated {timeAgo(item.updated_at)}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
