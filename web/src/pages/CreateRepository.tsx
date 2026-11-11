import { CheckCircle2, CircleAlert, GitBranch, LoaderCircle, WalletCards } from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import {
  EVMReceiptUnconfirmedError,
  EVMGasEstimationError,
  EVMTransactionBroadcastError,
  EVMTransactionRevertedError,
  formatError,
  formatWalletError,
  isSuiteDirectoryConfigured,
  loadConfig,
  networkProfile,
  repositoryNameAvailable,
  createRepositoryWithEvm,
  validateRepositoryMetadata,
  validateRepositoryName,
  verifySuite,
  type AppConfig,
} from "../lib/chain";
import { useWallet } from "../lib/WalletContext";
import type { Hex } from "viem";

type SuiteVerificationState = "idle" | "checking" | "verified" | "error";
type RepositoryNameAvailabilityState = "idle" | "checking" | "available" | "taken";

function formatRepositoryCreationError(error: unknown): string {
  if (error instanceof EVMReceiptUnconfirmedError) {
    return `Transaction ${error.txHash} was broadcast, but its receipt is still uncertain. Keep the hash and check the explorer before retrying.`;
  }
  if (error instanceof EVMTransactionRevertedError) {
    return `The creation transaction was mined and reverted (${error.txHash}). No repository was created.`;
  }
  if (error instanceof EVMGasEstimationError) return "Gas estimation failed before signing: " + formatError(error.cause) + ". No transaction was broadcast.";
  if (error instanceof EVMTransactionBroadcastError) return "Transaction could not be broadcast: " + formatError(error.cause) + ". No transaction hash was returned.";
  return formatWalletError(error) || formatError(error);
}

function StatusCheckRow({
  statusLabel,
  statusState,
  statusDetail,
}: {
  statusLabel: string;
  statusState: "ok" | "pending" | "error" | "idle";
  statusDetail: string;
}) {
  return (
    <div className={`new-repo-check new-repo-check-${statusState}`} role="status">
      {statusState === "pending" ? <LoaderCircle size={15} className="animate-spin" /> : statusState === "ok" ? <CheckCircle2 size={15} /> : statusState === "error" ? <CircleAlert size={15} /> : <GitBranch size={15} />}
      <span><b>{statusLabel}</b><small>{statusDetail}</small></span>
    </div>
  );
}

function getWalletNetworkStatus(
  walletConnection: ReturnType<typeof useWallet>["connected"],
  appConfig: AppConfig,
): { state: "ok" | "error" | "idle"; detail: string } {
  if (!walletConnection) return { state: "idle", detail: "Connect an EVM wallet to write to Injective." };
  if (!walletConnection.writable || walletConnection.chainId !== appConfig.evmChainId) {
    return { state: "error", detail: `Switch ${walletConnection.label} to Injective EVM testnet (chain ${appConfig.evmChainId}).` };
  }
  return { state: "ok", detail: `Injective EVM testnet · chain ${appConfig.evmChainId}` };
}

export default function CreateRepository() {
  const appConfig = useMemo(() => loadConfig(), []);
  const selectedNetworkProfile = networkProfile(appConfig);
  const { connected: connectedWallet, provider: walletProvider, openWalletModal } = useWallet();
  const [repositoryName, setRepositoryName] = useState("");
  const [repositoryDescription, setRepositoryDescription] = useState("");
  const [defaultBranchName, setDefaultBranchName] = useState("main");
  const [suiteVerificationState, setSuiteVerificationState] = useState<SuiteVerificationState>("idle");
  const [verifiedSuiteVersion, setVerifiedSuiteVersion] = useState<bigint | null>(null);
  const [suiteVerificationError, setSuiteVerificationError] = useState("");
  const [repositoryNameState, setRepositoryNameState] = useState<RepositoryNameAvailabilityState>("idle");
  const [repositoryNameError, setRepositoryNameError] = useState("");
  const [formSubmissionError, setFormSubmissionError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submittedTransactionHash, setSubmittedTransactionHash] = useState<Hex | null>(null);
  const [isCreationSuccessful, setIsCreationSuccessful] = useState(false);

  const suiteDirectoryConfigured = isSuiteDirectoryConfigured(appConfig.suiteDirectory);
  const walletNetworkStatus = getWalletNetworkStatus(connectedWallet, appConfig);
  const hasWritableWallet = Boolean(connectedWallet && walletProvider && walletNetworkStatus.state === "ok");

  useEffect(() => {
    if (!suiteDirectoryConfigured) {
      setSuiteVerificationState("error");
      setSuiteVerificationError("SuiteDirectory is not configured for this network profile.");
      return;
    }
    let verificationCancelled = false;
    setSuiteVerificationState("checking");
    setSuiteVerificationError("");
    verifySuite(appConfig, true)
      .then((suiteBinding) => {
        if (verificationCancelled) return;
        setVerifiedSuiteVersion(suiteBinding.version);
        if (suiteBinding.version !== 3n && suiteBinding.version !== 4n) {
          setSuiteVerificationState("error");
          setSuiteVerificationError("Web repository creation is disabled for an unrecognised Suite version.");
          return;
        }
        setSuiteVerificationState("verified");
      })
      .catch((verificationError) => {
        if (verificationCancelled) return;
        setVerifiedSuiteVersion(null);
        setSuiteVerificationState("error");
        setSuiteVerificationError(formatError(verificationError));
      });
    return () => { verificationCancelled = true; };
  }, [appConfig, suiteDirectoryConfigured]);

  useEffect(() => {
    setRepositoryNameError("");
    setRepositoryNameState("idle");
    if (!hasWritableWallet || suiteVerificationState !== "verified" || !repositoryName.trim()) return;
    let normalizedRepositoryName: string;
    try {
      normalizedRepositoryName = validateRepositoryName(repositoryName);
    } catch (validationError) {
      setRepositoryNameError(formatError(validationError));
      setRepositoryNameState("taken");
      return;
    }
    let availabilityCheckCancelled = false;
    setRepositoryNameState("checking");
    repositoryNameAvailable(appConfig, connectedWallet!.address, normalizedRepositoryName)
      .then(() => { if (!availabilityCheckCancelled) setRepositoryNameState("available"); })
      .catch((availabilityError) => {
        if (availabilityCheckCancelled) return;
        setRepositoryNameState("taken");
        setRepositoryNameError(formatError(availabilityError));
      });
    return () => { availabilityCheckCancelled = true; };
  }, [appConfig, connectedWallet, repositoryName, suiteVerificationState, hasWritableWallet]);

  const validateCreationForm = (): { repositoryName: string; description: string; defaultBranch: string } => {
    const normalizedRepositoryName = validateRepositoryName(repositoryName);
    const validatedMetadata = validateRepositoryMetadata(repositoryDescription, defaultBranchName);
    if (!isSuiteDirectoryConfigured(appConfig.suiteDirectory)) throw new Error("SuiteDirectory is not configured. Open Settings and verify it before creating a repository.");
    if (!connectedWallet || !walletProvider) throw new Error("Connect a wallet before creating a repository.");
    if (!hasWritableWallet) throw new Error(`Switch the wallet to Injective EVM testnet (chain ${appConfig.evmChainId}) before creating a repository.`);
    if (suiteVerificationState !== "verified") throw new Error(suiteVerificationError || "Suite verification has not completed successfully.");
    return { repositoryName: normalizedRepositoryName, ...validatedMetadata };
  };

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (isSubmitting || isCreationSuccessful || submittedTransactionHash) return;
    setFormSubmissionError("");
    setSubmittedTransactionHash(null);
    try {
      const repositoryFormValues = validateCreationForm();
      setIsSubmitting(true);
      const transactionHash = await createRepositoryWithEvm(
        walletProvider!,
        appConfig,
        repositoryFormValues.repositoryName,
        repositoryFormValues.description,
        repositoryFormValues.defaultBranch,
        connectedWallet!.ethAddress,
      );
      setSubmittedTransactionHash(transactionHash);
      setIsCreationSuccessful(true);
    } catch (creationError) {
      setFormSubmissionError(formatRepositoryCreationError(creationError));
      if (creationError instanceof EVMReceiptUnconfirmedError) setSubmittedTransactionHash(creationError.txHash as Hex);
    } finally {
      setIsSubmitting(false);
    }
  };

  const repositoryPath = connectedWallet && repositoryName.trim() ? `/${encodeURIComponent(connectedWallet.address)}/${encodeURIComponent(repositoryName.trim())}` : "";
  const cloneCommand = connectedWallet && repositoryName.trim() ? `igit clone igit://${connectedWallet.address}/${repositoryName.trim()}` : "";
  const firstPushCommand = connectedWallet && repositoryName.trim() ? `igit remote add inj igit://${connectedWallet.address}/${repositoryName.trim()}\nigit push inj ${defaultBranchName.trim() || "main"}` : "";

  return (
    <div className="new-repo-page">
      <div className="page-heading">
        <div>
          <h1>Create repository</h1>
          <p>Create an empty EVM repository from your Injective wallet.</p>
        </div>
        <Link className="page-heading-action" to="/">Back to dashboard</Link>
      </div>

      <div className="new-repo-layout">
        <section className="card new-repo-card" aria-labelledby="new-repo-form-title">
          <div className="new-repo-card-head">
            <div className="new-repo-icon"><GitBranch size={20} /></div>
            <div><h2 id="new-repo-form-title">Repository details</h2><p className="muted">The repository starts empty. Publish its first Git objects with the CLI after creation.</p></div>
          </div>
          <form onSubmit={handleSubmit} noValidate>
            <label className="new-repo-field">
              <span>Repository name</span>
              <div className="new-repo-field-control">
                <input className="field mono" value={repositoryName} maxLength={64} disabled={isSubmitting || isCreationSuccessful || Boolean(submittedTransactionHash)} onChange={(event) => setRepositoryName(event.target.value)} placeholder="my-repository" autoComplete="off" spellCheck={false} aria-invalid={Boolean(repositoryNameError)} required />
                <small>Letters, numbers, hyphens, underscores, and periods; up to 64 UTF-8 bytes.</small>
                {repositoryNameError && <em className="error">{repositoryNameError}</em>}
                {!repositoryNameError && repositoryNameState === "checking" && <em className="muted">Checking name availability…</em>}
                {!repositoryNameError && repositoryNameState === "available" && <em className="success-message">Name is available for {connectedWallet?.address}.</em>}
              </div>
            </label>
            <label className="new-repo-field">
              <span>Description <i>(optional)</i></span>
              <textarea className="field" value={repositoryDescription} maxLength={1024} rows={4} disabled={isSubmitting || isCreationSuccessful || Boolean(submittedTransactionHash)} onChange={(event) => setRepositoryDescription(event.target.value)} placeholder="What is this repository for?" />
              <small>Up to 1024 UTF-8 bytes.</small>
            </label>
            <label className="new-repo-field">
              <span>Default branch</span>
              <input className="field mono" value={defaultBranchName} maxLength={64} disabled={isSubmitting || isCreationSuccessful || Boolean(submittedTransactionHash)} onChange={(event) => setDefaultBranchName(event.target.value)} placeholder="main" />
              <small>Blank values use <code>main</code>.</small>
            </label>
            {formSubmissionError && <div className="error new-repo-error" role="alert">{formSubmissionError}</div>}
            <button className="primary new-repo-submit" type="submit" disabled={isSubmitting || isCreationSuccessful || Boolean(submittedTransactionHash) || !repositoryName.trim() || suiteVerificationState !== "verified" || !hasWritableWallet || repositoryNameState === "checking" || repositoryNameState === "taken"}>
              {isSubmitting ? <><LoaderCircle size={15} className="animate-spin" /> Waiting for confirmation…</> : isCreationSuccessful ? <><CheckCircle2 size={15} /> Repository created</> : "Create repository"}
            </button>
          </form>
        </section>

        <aside className="card new-repo-status-card" aria-labelledby="new-repo-status-title">
          <h2 id="new-repo-status-title">Before you create</h2>
          <div className="new-repo-checks">
            <StatusCheckRow statusLabel="Wallet" statusState={connectedWallet ? "ok" : "idle"} statusDetail={connectedWallet ? `${connectedWallet.label} · ${connectedWallet.address}` : "No wallet connected."} />
            <StatusCheckRow statusLabel="Network" statusState={walletNetworkStatus.state} statusDetail={walletNetworkStatus.detail} />
            <StatusCheckRow statusLabel="SuiteDirectory" statusState={suiteDirectoryConfigured ? (suiteVerificationState === "error" ? "error" : suiteVerificationState === "checking" ? "pending" : "ok") : "error"} statusDetail={suiteDirectoryConfigured ? (suiteVerificationState === "verified" ? `Verified Suite v${verifiedSuiteVersion}` : suiteVerificationState === "checking" ? "Verifying Directory, modules, and code hashes…" : suiteVerificationError) : "Not configured for this profile."} />
            <StatusCheckRow statusLabel="Repository name" statusState={repositoryNameState === "available" ? "ok" : repositoryNameState === "checking" ? "pending" : repositoryNameState === "taken" ? "error" : "idle"} statusDetail={repositoryNameState === "available" ? "Available" : repositoryNameState === "checking" ? "Checking on chain…" : repositoryNameState === "taken" ? (repositoryNameError || "Choose another name.") : "Enter a name to check it."} />
          </div>
          {!connectedWallet && <button className="new-repo-connect" type="button" onClick={openWalletModal}><WalletCards size={15} /> Connect wallet</button>}
          {suiteVerificationState === "error" && <Link className="new-repo-settings" to="/settings">Review Suite settings</Link>}
          <p className="new-repo-network-note muted">Network profile: <b>{selectedNetworkProfile.label}</b>. Web writes use the verified Core module and the same legacy transaction and receipt checks as the rest of the app.</p>
        </aside>
      </div>

      {isCreationSuccessful && connectedWallet && submittedTransactionHash && (
        <section className="card new-repo-success" aria-labelledby="new-repo-success-title">
          <div className="new-repo-success-head"><CheckCircle2 size={20} /><div><h2 id="new-repo-success-title">Repository created</h2><p className="muted">The transaction was confirmed on Injective EVM testnet.</p></div></div>
          <p><Link to={repositoryPath}>{connectedWallet.address}/{repositoryName.trim()}</Link></p>
          <p className="muted">Owner: <code>{connectedWallet.address}</code> · name: <code>{repositoryName.trim()}</code></p>
          <div className="new-repo-success-grid">
            <div><span className="muted">Clone</span><code className="command-line">{cloneCommand}</code></div>
            <div><span className="muted">Next step: first push</span><pre className="command-line">{firstPushCommand}</pre></div>
          </div>
          <a className="new-repo-tx" href={`${selectedNetworkProfile.evmExplorer}/tx/${submittedTransactionHash}`} target="_blank" rel="noreferrer">View transaction {submittedTransactionHash.slice(0, 10)}…</a>
        </section>
      )}
      {!isCreationSuccessful && connectedWallet && submittedTransactionHash && (
        <section className="card new-repo-uncertain" role="alert" aria-labelledby="new-repo-uncertain-title">
          <h2 id="new-repo-uncertain-title">Transaction submitted; receipt is uncertain</h2>
          <p>The wallet returned a transaction hash, but the app could not confirm its receipt. Check the explorer before taking any further action.</p>
          <code>{submittedTransactionHash}</code>
          <a href={selectedNetworkProfile.evmExplorer + "/tx/" + submittedTransactionHash} target="_blank" rel="noreferrer">Check this transaction on Blockscout</a>
        </section>
      )}
    </div>
  );
}
