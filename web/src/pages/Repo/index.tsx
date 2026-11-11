import { useEffect, useMemo, useState } from "react";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { AlertTriangle, ArrowRightLeft, Check, CheckCircle2, Copy, FileCode2, GitBranch, GitCommit, GitFork, LoaderCircle, Pencil, Save, Users, WalletCards, X } from "lucide-react";
import { loadConfig } from "../../lib/chain";
import { ContractTypeBadge, type RepositoryContractKind } from "../../components/ContractTypeBadge";
import type { AppConfig } from "../../lib/chain";
import { versionLabel, isKnownVersion } from "../../lib/suite-compat";
import { showToast } from "../../components/Toast";
import {
  formatError,
  listRefs,
  resolveRepo,
  repoInfoById,
  resolveOwner,
  type RefInfo,
  type RepoInfo,
  type ResolvedRepo,
  formatResourceError,
  forkRepositoryWithEvm,
  EVMForkMetadataError,
  repositoryNameAvailable,
  validateRepositoryName,
  validateRepositoryMetadata,
  verifySuite,
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
import { repositoryPath } from "../../lib/repository-path";
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

// The https clone URL must reference the public site, not whatever origin
// this UI is served from (localhost dev, preview deployments).
const SITE_ORIGIN = "https://www.igit.xyz";

function compactRepositoryOwner(owner: string): string {
  if (!owner.startsWith("inj1") || owner.length <= 20) return owner;
  return `${owner.slice(0, 12)}…${owner.slice(-6)}`;
}

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
  const { connected, provider, openWalletModal } = useWallet();
  const navigate = useNavigate();
  const location = useLocation();
  const forkMetadataNotice = location.state && typeof location.state === "object"
    ? location.state as { forkMetadataError?: string; forkTxHash?: string }
    : null;
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
  const [forkOpen, setForkOpen] = useState(false);
  const [forkName, setForkName] = useState("");
  const [forkDescription, setForkDescription] = useState("");
  const [forkBranch, setForkBranch] = useState("main");
  const [forkBusy, setForkBusy] = useState(false);
  const [forkError, setForkError] = useState("");
  const [forkSuiteError, setForkSuiteError] = useState("");
  const [forkNameState, setForkNameState] = useState<"idle" | "checking" | "available" | "taken">("idle");
  const [forkSource, setForkSource] = useState<RepoInfo | null>(null);
  const [forkSuiteState, setForkSuiteState] = useState<"idle" | "checking" | "verified" | "error">("idle");
  const sourceExceedsForkRefLimit = info?.suite_version === 3n && refs.length > 64;

  // The ?suite=3|4 link parameter selects which generation's same-named copy
  // this page operates on; every read and write then runs against that
  // repository's own SuiteDirectory instead of the first configured one.
  const [searchParams] = useSearchParams();
  const suiteParam = Number(searchParams.get("suite"));
  // Memoized: a fresh object every render would retrigger the load effect.
  const suitePref = useMemo(
    () => (suiteParam === 3 || suiteParam === 4 ? { suiteVersion: BigInt(suiteParam) } : undefined),
    [suiteParam],
  );
  const suiteCfg = useMemo(
    () => (resolvedRepo?.suiteDirectory != null ? { ...cfg, suiteDirectory: resolvedRepo.suiteDirectory } : cfg),
    [cfg, resolvedRepo?.suiteDirectory],
  );

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
        const identity = await resolveRepo(cfg, address, repo, suitePref);
        const repoCfg = identity.suiteDirectory != null
          ? { ...cfg, suiteDirectory: identity.suiteDirectory }
          : cfg;
        const evmRefs = await listRefs(repoCfg, address, repo);
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
  }, [owner, repo, cfg, isLegacy, suitePref]);

  useEffect(() => {
    let cancelled = false;
    setTransferLoaded(false);
    setTransferError("");
    if (isLegacy || !resolvedRepo?.repoId) {
      setPendingTransfer(null);
      return () => { cancelled = true; };
    }
    void pendingOwnershipTransferWithEvm(suiteCfg, resolvedRepo.repoId)
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
    let cancelled = false;
    setForkSource(null);
    if (isLegacy || !info?.forked_from || !resolvedRepo?.suiteDirectory) return () => { cancelled = true; };
    void repoInfoById(suiteCfg, info.forked_from as Hex)
      .then((source) => { if (!cancelled) setForkSource(source); })
      .catch(() => { /* Lineage remains visible as a repo ID when the source is unavailable. */ });
    return () => { cancelled = true; };
  }, [info?.forked_from, isLegacy, resolvedRepo?.suiteDirectory, suiteCfg]);

  useEffect(() => {
    if (!forkOpen || !connected || !resolvedRepo?.repoId || info?.suite_version !== 3n) return;
    const candidate = forkName.trim();
    setForkNameState("idle");
    setForkError("");
    if (!candidate) return;
    let normalized: string;
    try {
      normalized = validateRepositoryName(candidate);
    } catch (error) {
      setForkNameState("taken");
      setForkError(formatError(error));
      return;
    }
    let cancelled = false;
    setForkNameState("checking");
    void repositoryNameAvailable(suiteCfg, connected.address, normalized)
      .then(() => { if (!cancelled) setForkNameState("available"); })
      .catch((error) => {
        if (!cancelled) {
          setForkNameState("taken");
          setForkError(formatError(error));
        }
      });
    return () => { cancelled = true; };
  }, [connected, forkName, forkOpen, info?.suite_version, resolvedRepo?.repoId, suiteCfg]);

  useEffect(() => {
    if (!forkOpen || isLegacy || !resolvedRepo?.repoId) return;
    let cancelled = false;
    setForkSuiteState("checking");
    setForkSuiteError("");
    void verifySuite(suiteCfg, true)
      .then((binding) => {
        if (cancelled) return;
        if (binding.version !== 3n) {
          setForkSuiteState("error");
          setForkSuiteError("Web Fork is disabled for Suite v4 BYOS because its storage is user-controlled. Use a Suite v3 public-IPFS repository.");
          return;
        }
        setForkSuiteState("verified");
      })
      .catch((error) => {
        if (!cancelled) {
          setForkSuiteState("error");
          setForkSuiteError("Suite verification failed: " + formatError(error));
        }
      });
    return () => { cancelled = true; };
  }, [forkOpen, isLegacy, resolvedRepo?.repoId, sourceExceedsForkRefLimit, suiteCfg]);

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
      const refreshed = await pendingOwnershipTransferWithEvm(suiteCfg, resolvedRepo.repoId);
      setPendingTransfer(refreshed);
      setTransferLoaded(true);
      const refreshedRepo = await resolveRepo(
        cfg,
        resolvedRepo.requested.owner,
        resolvedRepo.requested.name,
        suitePref,
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
      await beginOwnershipTransferWithEvm(provider, suiteCfg, resolvedRepo.repoId, transferTarget.trim());
      setPendingTransfer(await pendingOwnershipTransferWithEvm(suiteCfg, resolvedRepo.repoId));
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
      await updateRepoInfoWithEvm(provider, suiteCfg, resolvedRepo.repoId, {
        ...(descriptionChanged ? { description: draftDescription } : {}),
        ...(branchChanged ? { defaultBranch: draftBranch } : {}),
      });
      const refreshed = await resolveRepo(cfg, addr, repo, suitePref);
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

  const openForkDialog = () => {
    if (isLegacy || !resolvedRepo.repoId || info.suite_version !== 3n || sourceExceedsForkRefLimit) return;
    if (!connected) {
      openWalletModal();
      return;
    }
    setForkName(info.name + "-fork");
    setForkDescription(info.description);
    setForkBranch(info.default_branch || "main");
    setForkError("");
    setForkSuiteError("");
    setForkNameState("idle");
    setForkSuiteState("idle");
    setForkOpen(true);
  };

  const submitFork = async () => {
    if (forkBusy || !provider || !connected || !resolvedRepo.repoId || forkSuiteState !== "verified") return;
    setForkError("");
    try {
      const normalizedName = validateRepositoryName(forkName);
      const metadata = validateRepositoryMetadata(forkDescription, forkBranch);
      await repositoryNameAvailable(suiteCfg, connected.address, normalizedName);
      setForkBusy(true);
      let result: Awaited<ReturnType<typeof forkRepositoryWithEvm>>;
      try {
        result = await forkRepositoryWithEvm(provider, suiteCfg, resolvedRepo.repoId, normalizedName, metadata, connected.ethAddress);
      } catch (error) {
        if (!(error instanceof EVMForkMetadataError)) throw error;
        setForkError(error.message + " The fork was created. Its page will open so you can verify it and retry metadata editing.");
        setForkOpen(false);
        navigate(repositoryPath(connected.address, normalizedName, location.search), {
          state: { forkMetadataError: error.message, forkTxHash: error.forkTxHash },
        });
        return;
      }
      const sourceLink = location.search;
      setForkOpen(false);
      navigate(repositoryPath(connected.address, normalizedName, sourceLink));
      showToast(result.metadataTxHash ? "Fork created and metadata updated" : "Fork created");
    } catch (error) {
      if (error instanceof EVMForkMetadataError) {
        setForkError(error.message + " The fork remains available; refresh its page and retry metadata editing. Fork transaction: " + error.forkTxHash);
      } else {
        setForkError(formatError(error));
      }
    } finally {
      setForkBusy(false);
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
            <strong id="legacy-migration-title">Migration to EVM V2 or newer required</strong>
            <span id="legacy-migration-copy">
              This repository remains readable from the CosmWasm V1 archive. Editing and current
              repository features require a future migration to the EVM V2 or newer Suite.
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
            <ContractTypeBadge kind={contractKind} suiteVersion={info.suite_version} />
          {!isLegacy && <SuiteVersionBadge cfg={suiteCfg} />}
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
            forked from {forkSource ? <Link to={`${sourcePrefix}/${forkSource.owner}/${forkSource.name}?suite=${info.suite_version}`}>{forkSource.owner}/{forkSource.name}</Link> : <code>{info.forked_from}</code>}
          </p>
        )}
        {forkMetadataNotice?.forkMetadataError && (
          <div className="repo-fork-created-notice" role="alert">
            <b>Fork created; metadata update needs a retry.</b>
            <span>{forkMetadataNotice.forkMetadataError}</span>
            {forkMetadataNotice.forkTxHash && <span>Confirmed fork transaction: <code>{forkMetadataNotice.forkTxHash}</code></span>}
          </div>
        )}
        <div className="repo-stats">
          <span><b>{headShort}</b> HEAD</span>
          <span><b>{branchesCount}</b> branches</span>
          <span><b>{tagsCount}</b> tags</span>
          <span><b>{packfilesCount}</b> packfiles</span>
        </div>
        {info.forked_from && info.suite_version === 3n && (
          <div className="repo-fork-created-notice" role="status">
            <b>Fork created from the source repository.</b>
            <span>Suite v3 copied the source refs and public-IPFS pack references at fork time. Push later changes with the CLI.</span>
            <code>igit remote add inj igit://{resolvedRepo.canonical.owner}/{resolvedRepo.canonical.name}</code>
          </div>
        )}
        {info.forked_from && info.suite_version === 4n && refs.length === 0 && (
          <div className="repo-fork-created-notice" role="status">
            <b>Fork created from the source metadata.</b>
            <span>Suite v4 BYOS forks do not copy Git refs or packfiles. Add the remote and push the first content with the CLI.</span>
            <code>igit remote add inj igit://{resolvedRepo.canonical.owner}/{resolvedRepo.canonical.name}</code>
          </div>
        )}
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
              <code title={cloneProtocol === "igit" ? `igit clone igit://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}` : `${SITE_ORIGIN}/${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`}>
                {cloneProtocol === "igit" ? `igit clone igit://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}` : `${SITE_ORIGIN}/${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`}
              </code>
            </div>
            <button
              className="btn"
              style={{ padding: "3px 8px", fontSize: "0.78rem" }}
              onClick={async () => {
                const url = cloneProtocol === "igit"
                  ? `igit clone igit://${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`
                  : `${SITE_ORIGIN}/${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`;
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
            {!isLegacy && info.suite_version !== 3n && (
              <span className="repo-fork-v3-disabled" role="note" title="Web Fork requires the public-IPFS Suite v3 path.">
                {info.suite_version === 4n ? "Fork unavailable: Suite v4 BYOS storage is user-controlled" : "Fork unavailable: requires verified Suite v3 public IPFS"}
              </span>
            )}
            {!isLegacy && connected && !connected.writable && (
              <span className="repo-fork-v3-disabled" role="status">Switch to Injective EVM testnet to fork.</span>
            )}
            {!isLegacy && info.suite_version === 3n && sourceExceedsForkRefLimit && (
              <span className="repo-fork-v3-disabled" role="note">This repository has more than 64 refs and cannot be forked by the Suite v3 contract.</span>
            )}
            {!isLegacy && info.moderation_status !== "active" && (
              <span className="repo-fork-v3-disabled" role="status">This repository cannot be forked while {info.moderation_status}.</span>
            )}
            {!isLegacy && info.suite_version === 3n && (
              <button
                className="repo-fork-trigger"
                type="button"
                onClick={openForkDialog}
                disabled={Boolean(connected && !connected.writable) || !resolvedRepo.repoId || !resolvedRepo.isCanonical || info.moderation_status !== "active" || sourceExceedsForkRefLimit}
                title={!connected ? "Connect a wallet to fork to your account" : !connected.writable ? "Switch to Injective EVM testnet" : info.moderation_status !== "active" ? "This repository cannot be forked while inactive" : "Fork this public-IPFS repository"}
                aria-label="Fork repository"
              >
                <GitFork size={15} /> Fork
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
                    onClick={() => void runOwnershipAction((provider, repoId) => cancelOwnershipTransferWithEvm(provider, suiteCfg, repoId))}
                  >
                    Cancel
                  </button>
                )}
                {transferCapabilities.canReject && (
                  <>
                    <button
                      type="button"
                      disabled={transferBusy}
                      onClick={() => void runOwnershipAction((provider, repoId) => rejectOwnershipTransferWithEvm(provider, suiteCfg, repoId))}
                    >
                      Reject
                    </button>
                    <button
                      className="repo-transfer-primary"
                      type="button"
                      disabled={transferBusy}
                      onClick={() => void runOwnershipAction((provider, repoId) => acceptOwnershipWithEvm(provider, suiteCfg, repoId))}
                    >
                      Accept
                    </button>
                  </>
                )}
                {transferCapabilities.canExpire && (
                  <button
                    type="button"
                    disabled={transferBusy}
                    onClick={() => void runOwnershipAction((provider, repoId) => expireOwnershipTransferWithEvm(provider, suiteCfg, repoId))}
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
      {forkOpen && (
        <div className="modal-overlay" onClick={() => { if (!forkBusy) setForkOpen(false); }}>
          <div
            className="modal repo-fork-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="repo-fork-title"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="modal-head">
              <b
                id="repo-fork-title"
                title={`Fork ${resolvedRepo.canonical.owner}/${resolvedRepo.canonical.name}`}
              >
                Fork {compactRepositoryOwner(resolvedRepo.canonical.owner)}/{resolvedRepo.canonical.name}
              </b>
              <button className="modal-x" type="button" onClick={() => setForkOpen(false)} disabled={forkBusy} aria-label="close fork dialog" title="close"><X size={17} /></button>
            </div>
            <form className="repo-metadata-form" onSubmit={(event) => { event.preventDefault(); void submitFork(); }}>
              {!connected && (
                <div className="repo-fork-warning" role="status">
                  <WalletCards size={16} /> Connect a wallet to set the new repository owner to your address.
                  <button type="button" onClick={openWalletModal}>Connect wallet</button>
                </div>
              )}
              {info.moderation_status !== "active" && <div className="repo-fork-warning" role="alert">This repository is {info.moderation_status} and cannot be forked.</div>}
              <div className="repo-fork-warning" role="note">
                <GitFork size={16} /> This creates a separate repository you own. Suite v3 copies up to 64 refs and iGit's public-IPFS pack references; the immutable pack bytes remain shared in public IPFS. Use the CLI to push later changes.
              </div>
              <div className="repo-fork-warning" role="status">
                <CheckCircle2 size={16} /> {forkSuiteState === "checking" ? "Verifying Suite v3 public-IPFS path…" : forkSuiteState === "verified" ? "Suite v3 public-IPFS verification passed." : forkSuiteState === "error" ? forkSuiteError : "Suite verification will run before submission."}
              </div>
              <label className="repo-metadata-field">
                <span>New repository name</span>
                <input className="field mono" value={forkName} maxLength={64} autoFocus disabled={forkBusy} onChange={(event) => setForkName(event.target.value)} required />
                {forkNameState === "checking" && <small>Checking name availability…</small>}
                {forkNameState === "available" && <small className="success-message">Available for {connected?.address}.</small>}
              </label>
              <label className="repo-metadata-field">
                <span>Description</span>
                <textarea className="field" value={forkDescription} maxLength={1024} rows={4} disabled={forkBusy} onChange={(event) => setForkDescription(event.target.value)} />
              </label>
              <label className="repo-metadata-field">
                <span>Default branch</span>
                <input className="field mono" value={forkBranch} maxLength={64} disabled={forkBusy} onChange={(event) => setForkBranch(event.target.value)} />
              </label>
              {forkError && <div className="error" role="alert">{forkError}</div>}
              <div className="repo-metadata-actions">
                <button type="button" onClick={() => setForkOpen(false)} disabled={forkBusy}>Cancel</button>
                <button className="repo-metadata-save" type="submit" disabled={forkBusy || !connected || !provider || !resolvedRepo.repoId || !resolvedRepo.isCanonical || info.moderation_status !== "active" || forkSuiteState !== "verified" || forkNameState === "checking" || forkNameState === "taken"}>
                  {forkBusy ? <LoaderCircle size={15} className="animate-spin" /> : <GitFork size={15} />}
                  {forkBusy ? "Waiting for confirmation…" : "Create fork"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}

/** Displays the verified suite protocol version alongside the repo title. */
function SuiteVersionBadge({ cfg }: { cfg: AppConfig }) {
  const [version, setVersion] = useState<bigint | null>(null);
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const { verifySuite } = await import("../../lib/transport");
        const binding = await verifySuite(cfg);
        if (!cancelled) setVersion(binding.version);
      } catch { /* badge is decorative; swallow errors */ }
    })();
    return () => { cancelled = true; };
  }, [cfg]);
  if (version === null) return null;
  return (
    <span
      className={`badge suite-version ${isKnownVersion(version) ? "" : "untested"}`}
      title={`SuiteDirectory protocol version: v${version}. ${isKnownVersion(version) ? "Fully supported" : "Untested version — using best-effort reader"}`}
    >
      {versionLabel(version)}
    </span>
  );
}
