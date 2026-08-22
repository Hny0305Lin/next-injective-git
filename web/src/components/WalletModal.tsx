import { useEffect, useState } from "react";
import { Icon as IconifyIcon } from "@iconify/react/offline";
import { ArrowLeft, CircleAlert, ExternalLink, LoaderCircle, QrCode, X } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { isWalletInstalled, subscribeWalletProviders, SUPPORTED_WALLETS } from "../lib/wallet";
import { useWallet } from "../lib/WalletContext";
import "../lib/wallet-icons";

// A compact injected EVM wallet picker for Suite transactions.
export function WalletModal({ onClose }: { onClose: () => void }) {
  const {
    connect,
    connectWalletConnect,
    connecting,
    error,
    walletConnectConfigured,
    walletConnectPairing,
    walletConnectUri,
    cancelWalletConnect,
  } = useWallet();
  const [installed, setInstalled] = useState<Record<string, boolean>>({});
  const [scanComplete, setScanComplete] = useState(false);
  const [showWalletConnect, setShowWalletConnect] = useState(false);

  const close = () => {
    // Cancellation is a no-op when no pairing is active and avoids a stale
    // render allowing a just-started pairing to outlive the dialog.
    cancelWalletConnect();
    setShowWalletConnect(false);
    onClose();
  };

  const openWalletConnect = () => {
    setShowWalletConnect(true);
    void connectWalletConnect().then((ok) => {
      if (ok) close();
    });
  };

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
      if (e.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [walletConnectPairing, onClose, cancelWalletConnect]);

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
    <div className="modal-overlay" onClick={close}>
      <div className="modal wallet-modal" role="dialog" aria-modal="true" aria-labelledby="wallet-modal-title" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <div className="wallet-modal-title-wrap">
            {showWalletConnect && (
              <button className="modal-back" onClick={() => { cancelWalletConnect(); setShowWalletConnect(false); }} aria-label="Back to wallet list" title="Back">
                <ArrowLeft size={15} />
              </button>
            )}
            <b id="wallet-modal-title">{showWalletConnect ? "Use WalletConnect" : "Connect a wallet"}</b>
          </div>
          <button className="modal-x" onClick={close} aria-label="Close wallet dialog" title="Close">
            <X size={16} />
          </button>
        </div>
        {showWalletConnect ? (
          <div className="walletconnect-panel">
            <div className="walletconnect-copy">
              <span className="walletconnect-kicker">EVM mobile wallet</span>
              <b>Scan with a supported EVM wallet</b>
              <span className="muted">Use a WalletConnect wallet that supports Injective EVM testnet (chain 1439).</span>
            </div>
            <div className="walletconnect-qr" aria-live="polite">
              {walletConnectUri ? (
                <QRCodeSVG value={walletConnectUri} size={216} level="M" includeMargin />
              ) : walletConnectPairing ? (
                <div className="walletconnect-loading"><LoaderCircle size={28} className="spin" /><span>Waiting for a secure pairing code…</span></div>
              ) : (
                <div className="walletconnect-loading"><CircleAlert size={28} /><span>Pairing is unavailable. Try again.</span></div>
              )}
            </div>
            <div className="walletconnect-compatibility" role="note">
              <CircleAlert size={15} />
              <span>Keplr Mobile does not currently list chain 1439 for EVM WalletConnect, so this QR cannot connect in Keplr.</span>
            </div>
            {walletConnectPairing && <button className="walletconnect-cancel" onClick={() => { cancelWalletConnect(); setShowWalletConnect(false); }}>Cancel pairing</button>}
            {error && <div className="error" role="alert">{error}</div>}
          </div>
        ) : (
          <>
            <div className="modal-sub muted">
              Connect an EVM wallet on Igit to interact with the network.
            </div>
            <div className="walletconnect-entry">
              <button
                className="walletconnect-trigger"
                onClick={openWalletConnect}
                disabled={!walletConnectConfigured || connecting}
                title={walletConnectConfigured ? "Connect an EVM mobile wallet that supports chain 1439" : "Set VITE_WALLETCONNECT_PROJECT_ID to enable WalletConnect"}
              >
                <QrCode size={17} />
                <IconifyIcon className="walletconnect-trigger-brand" icon="thesvg-color:walletconnect" width={20} height={20} aria-hidden="true" />
                <span>Scan with WalletConnect</span>
              </button>
              {!walletConnectConfigured && <span className="walletconnect-config muted">WalletConnect is not configured for this site.</span>}
            </div>
            {scanComplete && !Object.values(installed).some(Boolean) && (
              <div className="wallet-empty" role="status">
                <CircleAlert size={16} />
                <span>No supported EVM wallet was detected in this browser. Open iGit in Chrome or Brave where your wallet extension is installed and enabled.</span>
              </div>
            )}
            <div className="wallet-list">
              {SUPPORTED_WALLETS.map((w) => {
                const ok = installed[w.id];
                return (
                  <div className="wallet-row" key={w.id}>
                    <span className="wallet-ic" aria-hidden="true">
                      {w.icon.includes(":") ? <IconifyIcon icon={w.icon} width={24} height={24} /> : w.icon}
                    </span>
                    <span className="wallet-name">
                      {w.label}
                      <span className="wallet-kind muted">EVM</span>
                    </span>
                    {ok ? (
                      <button
                        className="wallet-connect"
                        disabled={connecting}
                        onClick={async () => {
                          if (await connect(w.id)) close();
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
          </>
        )}
      </div>
    </div>
  );
}
