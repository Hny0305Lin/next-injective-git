import { useCallback, useState } from "react";
import { useWallet } from "../../lib/WalletContext";
import { WalletModal } from "../../components/WalletModal";
import {
  formatError,
  type AppConfig,
} from "../../lib/chain";
import { getEvmProvider, sponsorWithKeplr } from "../../lib/wallet";

export default function SponsorForm({
  cfg,
  addr,
  repo,
}: {
  cfg: AppConfig;
  addr: string;
  repo: string;
}) {
  const { connected, walletModalOpen, openWalletModal, closeWalletModal, refreshBalance } = useWallet();
  const [amount, setAmount] = useState("0.1");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [txhash, setTxhash] = useState("");
  const [err, setErr] = useState("");

  const isOwnRepo = connected?.address === addr;

  const submit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!connected) return;
      setBusy(true);
      setErr("");
      setTxhash("");
      try {
        let hash: string;
        if (connected.kind === "cosmos") {
          hash = await sponsorWithKeplr(connected.cosmos, cfg, addr, repo, amount, message.trim());
        } else {
          const provider = getEvmProvider(connected.id);
          if (!provider) throw new Error(`${connected.label} not available`);
          const mm = await import("../../lib/metamask");
          hash = await mm.sponsorWithEvm(provider, cfg, addr, repo, amount, message.trim());
        }
        setTxhash(hash);
        setMessage("");
        await refreshBalance();
      } catch (e2) {
        console.error("[sponsor] failed:", e2);
        setErr(formatError(e2));
      } finally {
        setBusy(false);
      }
    },
    [connected, cfg, addr, repo, amount, message],
  );

  return (
    <div className="card sponsor-form">
      <div className="sponsor-form-title">Sponsor this repository</div>

      {!connected ? (
        <div className="sponsor-connect">
          <span className="muted">Connect a wallet to sponsor directly in the browser.</span>
          <button onClick={openWalletModal}>Connect Wallet</button>
        </div>
      ) : (
        <form className="sponsor-fields" onSubmit={submit}>
          <input
            className="field amount"
            type="number"
            min="0.0001"
            step="0.01"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            inputMode="decimal"
            aria-label="amount in INJ"
            required
          />
          <span className="muted">INJ</span>
          <input
            className="field"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder="message for the sponsor wall (optional)"
            maxLength={256}
            aria-label="sponsor message"
          />
          <button type="submit" disabled={busy}>
            {busy ? "signing…" : `Sponsor via ${connected.label}`}
          </button>
        </form>
      )}

      {isOwnRepo && (
        <p className="muted" style={{ marginTop: 8 }}>
          Note: this repo is owned by the connected wallet — you'd mostly be paying yourself
          (minus the platform fee).
        </p>
      )}
      {err && <div className="error" style={{ marginTop: 10 }} role="alert">{err}</div>}
      {txhash && (
        <div className="sponsor-ok" aria-live="polite">
          Sponsored! tx <code>{txhash.slice(0, 12)}…</code> — the split settled on-chain. Reload
          to see it on the wall.
        </div>
      )}
      {walletModalOpen && <WalletModal onClose={closeWalletModal} />}
    </div>
  );
}
