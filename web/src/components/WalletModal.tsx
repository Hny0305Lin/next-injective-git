import { useEffect, useState } from "react";
import { CircleAlert, ExternalLink, X } from "lucide-react";
import { isWalletInstalled, subscribeWalletProviders, SUPPORTED_WALLETS } from "../lib/wallet";
import { useWallet } from "../lib/WalletContext";

// A compact injected EVM wallet picker for Suite transactions.
export function WalletModal({ onClose }: { onClose: () => void }) {
  const { connect, connecting, error } = useWallet();
  const [installed, setInstalled] = useState<Record<string, boolean>>({});
  const [scanComplete, setScanComplete] = useState(false);

  // extensions inject asynchronously; re-scan briefly after mount
  useEffect(() => {
    const scan = () => {
      const map: Record<string, boolean> = {};
      for (const w of SUPPORTED_WALLETS) map[w.id] = isWalletInstalled(w.id);
      setInstalled(map);
    };
    const unsubscribe = subscribeWalletProviders(scan);
    scan();
    const t = setInterval(scan, 400);
    const complete = setTimeout(() => setScanComplete(true), 700);
    const stop = setTimeout(() => clearInterval(t), 3000);
    return () => {
      unsubscribe();
      clearInterval(t);
      clearTimeout(complete);
      clearTimeout(stop);
    };
  }, []);

  // close on Escape
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    const { body, documentElement } = document;
    const previousOverflow = body.style.overflow;
    const previousPaddingRight = body.style.paddingRight;
    const scrollbarWidth = window.innerWidth - documentElement.clientWidth;

    body.style.overflow = "hidden";
    if (scrollbarWidth > 0) body.style.paddingRight = `${scrollbarWidth}px`;

    return () => {
      body.style.overflow = previousOverflow;
      body.style.paddingRight = previousPaddingRight;
    };
  }, []);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal wallet-modal" role="dialog" aria-modal="true" aria-labelledby="wallet-modal-title" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <b id="wallet-modal-title">Connect a wallet</b>
          <button className="modal-x" onClick={onClose} aria-label="Close wallet dialog" title="Close">
            <X size={16} />
          </button>
        </div>
        <div className="modal-sub muted">
          Connect an EVM wallet on Injective testnet.
        </div>
        {scanComplete && !Object.values(installed).some(Boolean) && (
          <div className="wallet-empty" role="status">
            <CircleAlert size={16} />
            <span>No wallet extension was detected in this browser. Open iGit in Chrome or Brave where your wallet extension is installed and enabled.</span>
          </div>
        )}
        <div className="wallet-list">
          {SUPPORTED_WALLETS.map((w) => {
            const ok = installed[w.id];
            return (
              <div className="wallet-row" key={w.id}>
                <span className="wallet-ic">{w.icon}</span>
                <span className="wallet-name">
                  {w.label}
                  <span className="wallet-kind muted">EVM</span>
                </span>
                {ok ? (
                  <button
                    className="wallet-connect"
                    disabled={connecting}
                    onClick={async () => {
                      if (await connect(w.id)) onClose();
                    }}
                  >
                    {connecting ? "…" : "Connect"}
                  </button>
                ) : (
                  <a className="wallet-install" href={w.installUrl} target="_blank" rel="noreferrer">
                    Install <ExternalLink size={13} />
                  </a>
                )}
              </div>
            );
          })}
        </div>
        {error && <div className="error" style={{ margin: "0 16px 12px" }}>{error}</div>}
      </div>
    </div>
  );
}
