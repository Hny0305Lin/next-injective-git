import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { AlertTriangle, ArrowRightLeft, Check, Copy, FileCode2, GitBranch, GitCommit, Pencil, Save, Users, X } from "lucide-react";
import { loadConfig } from "../../lib/chain";
import { ContractTypeBadge, type RepositoryContractKind } from "../../components/ContractTypeBadge";
import { showToast } from "../../components/Toast";
import {
  formatError,
  listRefs,
  resolveRepo,
  resolveOwner,
  type RefInfo,
  type RepoInfo,
  type ResolvedRepo,
  formatResourceError,
  updateRepoInfoWithEvm,
  pendingOwnershipTransferWithEvm,
  beginOwnershipTransferWithEvm,
  cancelOwnershipTransferWithEvm,
  rejectOwnershipTransferWithEvm,
  expireOwnershipTransferWithEvm,
  acceptOwnershipWithEvm,
  ownershipTransferCapabilities,
  type PendingOwnershipTransfer,
  SuiteConfigurationError,
  EVMLocatorNotFoundError,
} from "../../lib/chain";
import { useWallet } from "../../lib/WalletContext";
import type { Eip1193 } from "../../lib/wallet";
import type { Hex } from "viem";
import { getRepoStore } from "../../lib/gitstore";
import {
  cosmWasmV1RepoInfo,
  formatCosmWasmV1Error,
  getCosmWasmV1SnapshotHeight,
  listCosmWasmV1Refs,
  resolveCosmWasmV1Owner,
} from "../../lib/cosmwasm-v1";
import TreeView from "./TreeView";
import BlobView from "./BlobView";
import CommitsView from "./CommitsView";
import CommitView from "./CommitView";
import RefsTab from "./RefsTab";
import SponsorsTab from "./SponsorsTab";
import { findRef, parseView, shortRef } from "./useRepoViews";

interface RepoProps {
  contractKind?: RepositoryContractKind;
}

interface PageResolvedRepo extends Omit<ResolvedRepo, "repoId"> {
  repoId: Hex | null;
}

export default function Repo({ contractKind = "evm-v2" }: RepoProps) {
  const params = useParams();
  const owner = params.owner ?? "";
  const repo = params.repo ?? "";
  const splat = params["*"] ?? "";
  const isLegacy = contractKind === "cosmwasm-v1";
  const cfg = useMemo(() => loadConfig(), []);
  const { connected, provider } = useWallet();
  const [addr, setAddr] = useState("");
  const [info, setInfo] = useState<RepoInfo | null>(null);
  const [resolvedRepo, setResolvedRepo] = useState<PageResolvedRepo | null>(null);
  const [refs, setRefs] = useState<RefInfo[]>([]);
  const [err, setErr] = useState("");
  const [archiveDiscoveryAvailable, setArchiveDiscoveryAvailable] = useState(false);
  const [copied, setCopied] = useState(false);
  const [cloneProtocol, setCloneProtocol] = useState<"igit" | "https">("igit");
  const [editingMetadata, setEditingMetadata] = useState(false);
  const [draftDescription, setDraftDescription] = useState("");
  const [draftBranch, setDraftBranch] = useState("");
  const [savingMetadata, setSavingMetadata] = useState(false);
  const [metadataError, setMetadataError] = useState("");
  const [pendingTransfer, setPendingTransfer] = useState<PendingOwnershipTransfer | null>(null);
  const [transferLoaded, setTransferLoaded] = useState(false);
  const [transferTarget, setTransferTarget] = useState("");
  const [transferBusy, setTransferBusy] = useState(false);
  const [transferError, setTransferError] = useState("");

  useEffect(() => {
    setErr("");
    setArchiveDiscoveryAvailable(false);
    setInfo(null);
    setResolvedRepo(null);
    let cancelled = false;
    (async () => {
      try {
        if (isLegacy) {
          const address = await resolveCosmWasmV1Owner(owner);
          const [legacyInfo, legacyRefs] = await Promise.all([
            cosmWasmV1RepoInfo(address, repo),
            listCosmWasmV1Refs(address, repo),
          ]);
          if (cancelled) return;
          setResolvedRepo({
            repoId: null,
            requested: { owner: address, name: repo },
            canonical: { owner: legacyInfo.owner, name: legacyInfo.name },
            isCanonical: true,
            info: legacyInfo,
          });
          setAddr(legacyInfo.owner);
          setInfo(legacyInfo);
          setRefs(legacyRefs);
          return;
        }

        const address = await resolveOwner(cfg, owner);
        const identity = await resolveRepo(cfg, address, repo);
        const evmRefs = await listRefs(cfg, address, repo);
        if (cancelled) return;
        setResolvedRepo(identity);
        setAddr(identity.canonical.owner);
        setInfo(identity.info);
        setRefs(evmRefs);
      } catch (e) {
        if (!cancelled) {
          setArchiveDiscoveryAvailable(
            !isLegacy && (e instanceof SuiteConfigurationError || e instanceof EVMLocatorNotFoundError),
          );
          setErr(isLegacy ? formatCosmWasmV1Error(e, "repository") : formatResourceError(e, "repository"));
        }
      }
    })();
    return () => { cancelled = true; };
  }, [owner, repo, cfg, isLegacy]);

  useEffect(() => {
    let cancelled = false;
    setTransferLoaded(false);
    setTransferError("");
    if (isLegacy || !resolvedRepo?.repoId) {
      setPendingTransfer(null);
      return () => { cancelled = true; };
    }
    void pendingOwnershipTransferWithEvm(cfg, resolvedRepo.repoId)
      .then((pending) => {
        if (!cancelled) {
          setPendingTransfer(pending);
          setTransferLoaded(true);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setPendingTransfer(null);
          setTransferLoaded(true);
          setTransferError(formatError(error));
        }
      });
    return () => { cancelled = true; };
  }, [cfg, isLegacy, resolvedRepo?.repoId]);

  useEffect(() => {
    if (!editingMetadata) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !savingMetadata) setEditingMetadata(false);
    };
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [editingMetadata, savingMetadata]);

  if (err) {
    return (
      <div className="repo-load-error">
        <div className="error" role="alert">{err}</div>
        {archiveDiscoveryAvailable && (
          <Link
            className="archive-discovery-link"
            to={`/archive/cosmwasm-v1/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`}
          >
            Open this locator in the CosmWasm V1 Archive
          </Link>
        )}
      </div>
    );
  }
  if (!info || !resolvedRepo) return <div className="spinner" aria-live="polite">querying chain…</div>;

  const fallbackRef =
    shortRef(
      refs.find((r) => r.ref_name === `refs/heads/${info.default_branch}`)?.ref_name ??
        refs[0]?.ref_name ??
        "",
    ) || info.default_branch;
  const view = parseView(splat, fallbackRef);
  const archivePrefix = "/archive/cosmwasm-v1";
  const sourcePrefix = isLegacy ? archivePrefix : "";
  const base = `${sourcePrefix}/${owner}/${repo}`;
  const canonicalBase = `${sourcePrefix}/${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`;
  const ownerBase = `${sourcePrefix}/${resolvedRepo.canonical.owner}`;
  const store = getRepoStore(
    resolvedRepo.repoId ?? `cosmwasm-v1:${getCosmWasmV1SnapshotHeight() ?? "latest"}:${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`,
  );
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
  const canEditMetadata =
    !isLegacy &&
    resolvedRepo?.isCanonical === true &&
    resolvedRepo.repoId != null &&
    connected != null &&
    connected.writable &&
    connected.address === addr;
  const canManageOwnership =
    !isLegacy &&
    resolvedRepo.isCanonical &&
    connected != null &&
    connected.writable &&
    Boolean(resolvedRepo.repoId);
  const isCurrentOwner = canManageOwnership && connected?.address === resolvedRepo.canonical.owner;
  const badgeProvider =
    isCurrentOwner && connected != null
      ? provider ?? undefined
      : undefined;
  const economicProvider =
    isCurrentOwner && connected != null
      ? provider ?? undefined
      : undefined;
  const transferCapabilities = ownershipTransferCapabilities(
    pendingTransfer,
    canManageOwnership ? connected?.address ?? "" : "",
    resolvedRepo.canonical.owner,
  );

  const runOwnershipAction = async (
    action: (provider: Eip1193, repoId: Hex) => Promise<string>,
  ) => {
    if (!canManageOwnership || !connected || !resolvedRepo.repoId) return;
    if (!provider) {
      setTransferError(`${connected.label} is no longer available`);
      return;
    }
    setTransferBusy(true);
    setTransferError("");
    try {
      await action(provider, resolvedRepo.repoId);
      const refreshed = await pendingOwnershipTransferWithEvm(cfg, resolvedRepo.repoId);
      setPendingTransfer(refreshed);
      setTransferLoaded(true);
      const refreshedRepo = await resolveRepo(
        cfg,
        resolvedRepo.requested.owner,
        resolvedRepo.requested.name,
      );
      setResolvedRepo(refreshedRepo);
      setAddr(refreshedRepo.canonical.owner);
      setInfo(refreshedRepo.info);
      showToast("Ownership transfer updated");
    } catch (error) {
      setTransferError(formatError(error));
    } finally {
      setTransferBusy(false);
    }
  };

  const beginTransfer = async () => {
    if (!canManageOwnership || !isCurrentOwner || !connected || !resolvedRepo.repoId) return;
    if (!provider) {
      setTransferError(`${connected.label} is no longer available`);
      return;
    }
    setTransferBusy(true);
    setTransferError("");
    try {
      await beginOwnershipTransferWithEvm(provider, cfg, resolvedRepo.repoId, transferTarget.trim());
      setPendingTransfer(await pendingOwnershipTransferWithEvm(cfg, resolvedRepo.repoId));
      setTransferLoaded(true);
      setTransferTarget("");
      showToast("Ownership transfer started");
    } catch (error) {
      setTransferError(formatError(error));
    } finally {
      setTransferBusy(false);
    }
  };

  const openMetadataEditor = () => {
    setDraftDescription(info.description);
    setDraftBranch(info.default_branch);
    setMetadataError("");
    setEditingMetadata(true);
  };

  const saveMetadata = async () => {
    if (!canEditMetadata || !connected || !resolvedRepo.repoId) return;
    const descriptionChanged = draftDescription !== info.description;
    const branchChanged = draftBranch !== info.default_branch;
    if (!descriptionChanged && !branchChanged) {
      setEditingMetadata(false);
      return;
    }
    if (!provider) {
      setMetadataError(`${connected.label} is no longer available`);
      return;
    }
    setSavingMetadata(true);
    setMetadataError("");
    try {
      await updateRepoInfoWithEvm(provider, cfg, resolvedRepo.repoId, {
        ...(descriptionChanged ? { description: draftDescription } : {}),
        ...(branchChanged ? { defaultBranch: draftBranch } : {}),
      });
      const refreshed = await resolveRepo(cfg, addr, repo);
      setResolvedRepo(refreshed);
      setInfo(refreshed.info);
      setEditingMetadata(false);
      showToast("Repository updated");
    } catch (error) {
      setMetadataError(formatError(error));
    } finally {
      setSavingMetadata(false);
    }
  };

  return (
    <div>
      {resolvedRepo && !resolvedRepo.isCanonical && (
        <div className="repo-moved-notice" role="status">
          This repository has moved. <Link to={canonicalBase}>Open the current location</Link>.
        </div>
      )}
      {isLegacy && (
        <section className="legacy-migration-alert" role="status" aria-labelledby="legacy-migration-title">
          <AlertTriangle size={18} aria-hidden="true" />
          <div>
            <strong id="legacy-migration-title">Migration to EVM V2 required</strong>
            <span id="legacy-migration-copy">
              This repository remains readable from the CosmWasm V1 archive. Editing and current
              repository features require a future migration to the EVM Suite.
            </span>
          </div>
          <button type="button" disabled aria-describedby="legacy-migration-copy" title="Repository migration is not available yet">
            Migration unavailable
          </button>
        </section>
      )}
      <div className="repo-head">
        <div className="repo-head-top">
          <span className="repo-icon">
            <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M2 4.5A2.5 2.5 0 0 1 4.5 2h3.172a2.5 2.5 0 0 1 2.236 1.5H12.5A2.5 2.5 0 0 1 15 6v5.5a2.5 2.5 0 0 1-2.5 2.5h-9A2.5 2.5 0 0 1 1 11.5v-7z"/>
              <path d="M5.5 2v3.5A2.5 2.5 0 0 0 8 8H12.5"/>
            </svg>
          </span>
          <h2>
            <Link to={ownerBase} className="owner-link">
              {resolvedRepo.canonical.owner.startsWith("inj1") ? `${resolvedRepo.canonical.owner.slice(0, 12)}…` : resolvedRepo.canonical.owner}
            </Link>
            {" / "}
            <b>{resolvedRepo.canonical.name}</b>
            <ContractTypeBadge kind={contractKind} />
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
            forked from <Link to={`${sourcePrefix}/${info.forked_from}`}>{info.forked_from}</Link>
          </p>
        )}
        <div className="repo-stats">
          <span><b>{headShort}</b> HEAD</span>
          <span><b>{branchesCount}</b> branches</span>
          <span><b>{tagsCount}</b> tags</span>
          <span><b>{packfilesCount}</b> packfiles</span>
        </div>
        {!isLegacy && (
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
              <code title={cloneProtocol === "igit" ? `igit clone igit://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}` : `${cloneProtocol}://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`}>
                {cloneProtocol === "igit" ? `igit clone igit://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}` : `${cloneProtocol}://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`}
              </code>
            </div>
            <button
              className="btn"
              style={{ padding: "3px 8px", fontSize: "0.78rem" }}
              onClick={async () => {
                const url = cloneProtocol === "igit"
                  ? `igit clone igit://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`
                  : `${cloneProtocol}://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`;
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
            {canEditMetadata && (
              <button
                className="repo-edit-trigger"
                type="button"
                onClick={openMetadataEditor}
                title="edit repository metadata"
                aria-label="edit repository metadata"
              >
                <Pencil size={15} />
              </button>
            )}
          </div>
        )}
      </div>

      {canManageOwnership && (isCurrentOwner || pendingTransfer != null) && (
        <section className="repo-transfer-panel" aria-labelledby="repo-transfer-title">
          <div className="repo-transfer-heading">
            <ArrowRightLeft size={16} aria-hidden="true" />
            <b id="repo-transfer-title">Ownership transfer</b>
          </div>
          {!transferLoaded ? (
            <div className="muted" aria-live="polite">Loading transfer status...</div>
          ) : !pendingTransfer && isCurrentOwner ? (
            <form
              className="repo-transfer-start"
              onSubmit={(event) => {
                event.preventDefault();
                void beginTransfer();
              }}
            >
              <input
                className="field mono"
                value={transferTarget}
                onChange={(event) => setTransferTarget(event.target.value)}
                placeholder="inj1... or 0x... target address"
                aria-label="new repository owner"
                disabled={transferBusy}
                required
              />
              <button type="submit" disabled={transferBusy || transferTarget.trim().length === 0}>
                Start transfer
              </button>
            </form>
          ) : pendingTransfer ? (
            <div className="repo-transfer-pending">
              <div>
                <span className="muted">Proposed owner</span>{" "}
                <code>{pendingTransfer.newOwner}</code>
              </div>
              <div className="repo-transfer-deadlines muted">
                Accept after {new Date(pendingTransfer.executeAfter * 1000).toLocaleString()}; expires {new Date(pendingTransfer.expiresAt * 1000).toLocaleString()}.
              </div>
              <div className="repo-transfer-actions">
                {transferCapabilities.canCancel && (
                  <button
                    type="button"
                    disabled={transferBusy}
                    onClick={() => void runOwnershipAction((provider, repoId) => cancelOwnershipTransferWithEvm(provider, cfg, repoId))}
                  >
                    Cancel
                  </button>
                )}
                {transferCapabilities.canReject && (
                  <>
                    <button
                      type="button"
                      disabled={transferBusy}
                      onClick={() => void runOwnershipAction((provider, repoId) => rejectOwnershipTransferWithEvm(provider, cfg, repoId))}
                    >
                      Reject
                    </button>
                    <button
                      className="repo-transfer-primary"
                      type="button"
                      disabled={transferBusy}
                      onClick={() => void runOwnershipAction((provider, repoId) => acceptOwnershipWithEvm(provider, cfg, repoId))}
                    >
                      Accept
                    </button>
                  </>
                )}
                {transferCapabilities.canExpire && (
                  <button
                    type="button"
                    disabled={transferBusy}
                    onClick={() => void runOwnershipAction((provider, repoId) => expireOwnershipTransferWithEvm(provider, cfg, repoId))}
                  >
                    Clear expired transfer
                  </button>
                )}
              </div>
            </div>
          ) : null}
          {transferError && <div className="error" role="alert">{transferError}</div>}
        </section>
      )}

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
        {!isLegacy && (
          <Link className={tab === "sponsors" ? "on" : ""} to={`${base}/sponsors`} role="tab">
            <Users size={14} /> Sponsors
          </Link>
        )}
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
      ) : view.kind === "sponsors" && !isLegacy && resolvedRepo.repoId ? (
        <SponsorsTab
          cfg={cfg}
          addr={resolvedRepo.canonical.owner}
          repo={resolvedRepo.canonical.name}
          owner={resolvedRepo.canonical.owner}
          repoId={resolvedRepo.repoId}
          badgeProvider={badgeProvider}
          economicProvider={economicProvider}
        />
      ) : view.kind === "sponsors" && isLegacy ? (
        <div className="archive-readonly-message" role="status">
          Sponsorship is unavailable for archived CosmWasm V1 repositories.
        </div>
      ) : (
        <div className="error" role="alert">ref not found: {view.ref}</div>
      )}

      {editingMetadata && canEditMetadata && (
        <div
          className="modal-overlay"
          onClick={() => {
            if (!savingMetadata) setEditingMetadata(false);
          }}
        >
          <div
            className="modal repo-metadata-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="repo-metadata-title"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="modal-head">
              <b id="repo-metadata-title">Repository settings</b>
              <button
                className="modal-x"
                type="button"
                onClick={() => setEditingMetadata(false)}
                disabled={savingMetadata}
                title="close"
                aria-label="close repository settings"
              >
                <X size={17} />
              </button>
            </div>
            <form
              className="repo-metadata-form"
              onSubmit={(event) => {
                event.preventDefault();
                void saveMetadata();
              }}
            >
              <label className="repo-metadata-field">
                <span>Description</span>
                <textarea
                  className="field"
                  value={draftDescription}
                  maxLength={1024}
                  rows={4}
                  autoFocus
                  disabled={savingMetadata}
                  onChange={(event) => setDraftDescription(event.target.value)}
                />
              </label>
              <label className="repo-metadata-field">
                <span>Default branch</span>
                <input
                  className="field mono"
                  value={draftBranch}
                  maxLength={64}
                  disabled={savingMetadata}
                  onChange={(event) => setDraftBranch(event.target.value)}
                />
              </label>
              {metadataError && <div className="error" role="alert">{metadataError}</div>}
              <div className="repo-metadata-actions">
                <button
                  type="button"
                  onClick={() => setEditingMetadata(false)}
                  disabled={savingMetadata}
                >
                  Cancel
                </button>
                <button className="repo-metadata-save" type="submit" disabled={savingMetadata}>
                  <Save size={15} />
                  {savingMetadata ? "Saving" : "Save"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
