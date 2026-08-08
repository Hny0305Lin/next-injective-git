import { useEffect, useState } from "react";
import { isWalletInstalled, SUPPORTED_WALLETS } from "../lib/wallet";
import { useWallet } from "../lib/WalletContext";

// A GitHub/RainbowKit-style wallet picker listing every supported Web3 wallet
// with an installed / install-link state. Cosmos wallets sign via CosmJS,
// MetaMask via EIP-712 — the context routes by wallet id.
export function WalletModal({ onClose }: { onClose: () => void }) {
  const { connect, connecting, error } = useWallet();
  const [installed, setInstalled] = useState<Record<string, boolean>>({});

  // extensions inject asynchronously; re-scan briefly after mount
  useEffect(() => {
    const scan = () => {
      const map: Record<string, boolean> = {};
      for (const w of SUPPORTED_WALLETS) map[w.id] = isWalletInstalled(w.id);
      setInstalled(map);
    };
    scan();
    const t = setInterval(scan, 400);
    const stop = setTimeout(() => clearInterval(t), 3000);
    return () => {
      clearInterval(t);
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
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="wallet-modal-title" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <b id="wallet-modal-title">Connect a wallet</b>
          <button className="modal-x" onClick={onClose} aria-label="close">
            ✕
          </button>
        </div>
        <div className="modal-sub muted">
          Cosmos wallets sign natively; EVM wallets sign via EIP-712 (auto-switches to Injective inEVM).
        </div>
        <div className="wallet-list">
          {SUPPORTED_WALLETS.map((w) => {
            const ok = installed[w.id];
            return (
              <div className="wallet-row" key={w.id}>
                <span className="wallet-ic">{w.icon}</span>
                <span className="wallet-name">
                  {w.label}
                  <span className="wallet-kind muted">{w.kind === "evm" ? "EVM" : "Cosmos"}</span>
                </span>
                {ok ? (
                  <button
                    className="wallet-connect"
                    disabled={connecting}
                    onClick={async () => {
                      await connect(w.id);
                      onClose();
                    }}
                  >
                    {connecting ? "…" : "Connect"}
                  </button>
                ) : (
                  <a className="wallet-install" href={w.installUrl} target="_blank" rel="noreferrer">
                    Install ↗
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
