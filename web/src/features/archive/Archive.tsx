import { Archive as ArchiveIcon, ArrowRight, GitBranch, Search } from "lucide-react";
import { FormEvent, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ContractTypeBadge } from "../../components/ContractTypeBadge";
import { useWallet } from "../../lib/WalletContext";
import {
  COSMWASM_V1_ARCHIVE,
  formatCosmWasmV1Error,
  getCosmWasmV1SnapshotHeight,
  listCosmWasmV1Repos,
  prepareCosmWasmV1Snapshot,
} from "../../lib/cosmwasm-v1";
import { timeAgo, type RepoInfo } from "../../lib/chain";
import { truncateAddress } from "../../lib/utils";

export default function Archive() {
  const navigate = useNavigate();
  const { address } = useWallet();
  const [owner, setOwner] = useState(address);
  const [repo, setRepo] = useState("");
  const [repos, setRepos] = useState<RepoInfo[] | null>(null);
  const [error, setError] = useState("");
  const [snapshotBlock, setSnapshotBlock] = useState<number | null>(getCosmWasmV1SnapshotHeight());

  useEffect(() => {
    let cancelled = false;
    void prepareCosmWasmV1Snapshot()
      .then((height) => {
        if (!cancelled) setSnapshotBlock(height);
      })
      .catch((cause) => {
        if (!cancelled) setError(formatCosmWasmV1Error(cause, "repository"));
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!address) {
      setRepos(null);
      return;
    }
    setOwner(address);
    setRepos(null);
    setError("");
    let cancelled = false;
    void listCosmWasmV1Repos(address)
      .then((items) => {
        if (!cancelled) setRepos(items);
      })
      .catch((cause) => {
        if (!cancelled) {
          setRepos([]);
          setError(formatCosmWasmV1Error(cause, "owner"));
        }
      });
    return () => { cancelled = true; };
  }, [address]);

  const openArchive = (event: FormEvent) => {
    event.preventDefault();
    const targetOwner = owner.trim();
    const targetRepo = repo.trim();
    if (!targetOwner) return;
    const base = `/archive/cosmwasm-v1/${encodeURIComponent(targetOwner)}`;
    navigate(targetRepo ? `${base}/${encodeURIComponent(targetRepo)}` : base);
  };

  return (
    <div className="archive-page">
      <div className="page-heading archive-heading">
        <div>
          <div className="archive-title-line">
            <ArchiveIcon size={22} aria-hidden="true" />
            <h1>Repository archive</h1>
            <ContractTypeBadge kind="cosmwasm-v1" />
          </div>
          <p>Historical repositories from the original Injective CosmWasm contract, shown in a live read-only snapshot.</p>
        </div>
      </div>

      <div className="archive-meta-strip" aria-label="Archive contract details">
        <span><b>Network</b>{COSMWASM_V1_ARCHIVE.network}</span>
        <span><b>Access</b>Live read-only</span>
        <span title={COSMWASM_V1_ARCHIVE.contract}>
          <b>Contract</b><code>{truncateAddress(COSMWASM_V1_ARCHIVE.contract, 13, 6)}</code>
        </span>
        <span><b>Snapshot block</b>{snapshotBlock ?? "Loading"}</span>
      </div>

      <section className="archive-lookup" aria-labelledby="archive-lookup-title">
        <div className="section-heading">
          <div>
            <h2 id="archive-lookup-title">Open archived repository</h2>
            <p>Look up the historical owner namespace or a specific repository.</p>
          </div>
        </div>
        <form onSubmit={openArchive}>
          <label>
            <span>Owner</span>
            <input
              className="field mono"
              value={owner}
              onChange={(event) => setOwner(event.target.value)}
              placeholder="inj1... or historical username"
              required
            />
          </label>
          <label>
            <span>Repository</span>
            <input
              className="field"
              value={repo}
              onChange={(event) => setRepo(event.target.value)}
              placeholder="optional"
            />
          </label>
          <button type="submit" className="archive-open-button">
            <Search size={15} aria-hidden="true" /> Open archive
          </button>
        </form>
      </section>

      {error && <div className="error" role="alert">{error}</div>}

      {address && repos && (
        <section className="archive-owned" aria-labelledby="archive-owned-title">
          <div className="section-heading">
            <div>
              <h2 id="archive-owned-title">Your archived repositories</h2>
              <p>{repos.length} repositories in the V1 contract.</p>
            </div>
            <Link to={`/archive/cosmwasm-v1/${encodeURIComponent(address)}`} className="page-heading-action">
              View namespace <ArrowRight size={14} />
            </Link>
          </div>
          {repos.length === 0 ? (
            <div className="surface-panel empty-state compact">
              <GitBranch size={20} />
              <p>No CosmWasm V1 repositories for this address.</p>
            </div>
          ) : (
            <div className="surface-panel repo-list">
              {repos.map((item) => (
                <div className="repo-list-item" key={item.name}>
                  <div className="repo-list-icon"><GitBranch size={15} /></div>
                  <div className="repo-list-content">
                    <h3>
                      <Link to={`/archive/cosmwasm-v1/${encodeURIComponent(item.owner)}/${encodeURIComponent(item.name)}`}>{item.name}</Link>
                      <ContractTypeBadge kind="cosmwasm-v1" />
                      {item.moderation_status !== "active" && (
                        <span className={`badge ${item.moderation_status}`}>{item.moderation_status}</span>
                      )}
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
        </section>
      )}
    </div>
  );
}
