import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { FileCode2, GitCommit, GitBranch, Users, Copy, Check, Pencil, Save, X, ArrowRightLeft } from "lucide-react";
import { loadConfig } from "../../lib/chain";
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
} from "../../lib/chain";
import { useWallet } from "../../lib/WalletContext";
import { getEvmProvider } from "../../lib/wallet";
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
  const { connected } = useWallet();
  const [addr, setAddr] = useState("");
  const [info, setInfo] = useState<RepoInfo | null>(null);
  const [resolvedRepo, setResolvedRepo] = useState<ResolvedRepo | null>(null);
  const [refs, setRefs] = useState<RefInfo[]>([]);
  const [err, setErr] = useState("");
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
    setInfo(null);
    setResolvedRepo(null);
    (async () => {
      try {
        const a = await resolveOwner(cfg, owner);
        setAddr(a);
        const identity = await resolveRepo(cfg, a, repo);
        const rf = await listRefs(cfg, a, repo);
        setResolvedRepo(identity);
        setAddr(identity.canonical.owner);
        setInfo(identity.info);
        setRefs(rf);
      } catch (e) {
        setErr(formatResourceError(e, "repository"));
      }
    })();
  }, [owner, repo, cfg]);

  useEffect(() => {
    let cancelled = false;
    setTransferLoaded(false);
    setTransferError("");
    if (!resolvedRepo?.repoId || resolvedRepo.backend !== "evm" || !cfg.evmContract) {
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
  }, [cfg, resolvedRepo?.repoId, resolvedRepo?.backend]);

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

  if (err) return <div className="error" role="alert">{err}</div>;
  if (!info || !resolvedRepo) return <div className="spinner" aria-live="polite">querying chain…</div>;

  const fallbackRef =
    shortRef(
      refs.find((r) => r.ref_name === `refs/heads/${info.default_branch}`)?.ref_name ??
        refs[0]?.ref_name ??
        "",
    ) || info.default_branch;
  const view = parseView(splat, fallbackRef);
  const base = `/${owner}/${repo}`;
  const canonicalBase = `/${resolvedRepo?.canonical.owner ?? addr}/${resolvedRepo?.canonical.name ?? repo}`;
  const store = getRepoStore(resolvedRepo?.repoId ?? `${resolvedRepo?.canonical.owner ?? addr}/${repo}`);
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
    resolvedRepo?.isCanonical === true &&
    connected?.kind === "evm" &&
    connected.address === addr &&
    /^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract);
  const canManageOwnership =
    resolvedRepo.backend === "evm" &&
    resolvedRepo.isCanonical &&
    connected?.kind === "evm" &&
    /^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract) &&
    Boolean(resolvedRepo.repoId);
  const isCurrentOwner = canManageOwnership && connected?.address === resolvedRepo.canonical.owner;
  const badgeProvider =
    isCurrentOwner && /^0x[0-9a-fA-F]{40}$/.test(cfg.evmBadgeModule) && connected?.kind === "evm"
      ? getEvmProvider(connected.id)
      : undefined;
  const economicProvider =
    isCurrentOwner && /^0x[0-9a-fA-F]{40}$/.test(cfg.evmEconomicModule) && connected?.kind === "evm"
      ? getEvmProvider(connected.id)
      : undefined;
  const transferCapabilities = ownershipTransferCapabilities(
    pendingTransfer,
    canManageOwnership ? connected?.address ?? "" : "",
    resolvedRepo.canonical.owner,
  );

  const runOwnershipAction = async (
    action: (provider: NonNullable<ReturnType<typeof getEvmProvider>>, repoId: string) => Promise<string>,
  ) => {
    if (!canManageOwnership || !connected || connected.kind !== "evm" || !resolvedRepo.repoId) return;
    const provider = getEvmProvider(connected.id);
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
    if (!canManageOwnership || !isCurrentOwner || !connected || connected.kind !== "evm" || !resolvedRepo.repoId) return;
    const provider = getEvmProvider(connected.id);
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
    if (!canEditMetadata || !connected || connected.kind !== "evm") return;
    const descriptionChanged = draftDescription !== info.description;
    const branchChanged = draftBranch !== info.default_branch;
    if (!descriptionChanged && !branchChanged) {
      setEditingMetadata(false);
      return;
    }
    const provider = getEvmProvider(connected.id);
    if (!provider) {
      setMetadataError(`${connected.label} is no longer available`);
      return;
    }
    setSavingMetadata(true);
    setMetadataError("");
    try {
      await updateRepoInfoWithEvm(provider, cfg, repo, {
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
      <div className="repo-head">
        <div className="repo-head-top">
          <span className="repo-icon">
            <svg width="20" height="20" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M2 4.5A2.5 2.5 0 0 1 4.5 2h3.172a2.5 2.5 0 0 1 2.236 1.5H12.5A2.5 2.5 0 0 1 15 6v5.5a2.5 2.5 0 0 1-2.5 2.5h-9A2.5 2.5 0 0 1 1 11.5v-7z"/>
              <path d="M5.5 2v3.5A2.5 2.5 0 0 0 8 8H12.5"/>
            </svg>
          </span>
          <h2>
            <Link to={`/${resolvedRepo.canonical.owner}`} className="owner-link">
              {resolvedRepo.canonical.owner.startsWith("inj1") ? `${resolvedRepo.canonical.owner.slice(0, 12)}…` : resolvedRepo.canonical.owner}
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
        <SponsorsTab
          cfg={cfg}
          addr={resolvedRepo.canonical.owner}
          repo={resolvedRepo.canonical.name}
          owner={resolvedRepo.canonical.owner}
          repoId={resolvedRepo.repoId}
          backend={resolvedRepo.backend}
          badgeProvider={badgeProvider}
          economicProvider={economicProvider}
        />
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
