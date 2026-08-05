import { useState, useRef, useEffect } from "react";
import { Copy, LogOut } from "lucide-react";
import { useWallet } from "../lib/WalletContext";
import { showToast } from "./Toast";

export default function AccountMenu() {
  const { connected, disconnect } = useWallet();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  if (!connected) return null;

  const copy = () => {
    navigator.clipboard.writeText(connected.address);
    showToast("address copied");
    setOpen(false);
  };

  return (
    <div className="account-menu-wrap" ref={ref}>
      <button className="wallet-btn connected" onClick={() => setOpen(!open)} aria-expanded={open}>
        {connected.label}
        {" · "}
        {connected.address.slice(0, 7)}…{connected.address.slice(-4)}
      </button>
      {open && (
        <div className="account-menu" role="menu">
          <div className="account-menu-row">
            <span className="muted small">address</span>
            <button className="copy-btn" onClick={copy} title="copy address" aria-label="copy address">
              <Copy size={14} />
            </button>
          </div>
          <code className="mono account-addr">{connected.address}</code>
          <div className="account-menu-divider" />
          <button className="account-menu-item" onClick={() => { disconnect(); setOpen(false); }}>
            <LogOut size={14} /> Disconnect
          </button>
        </div>
      )}
    </div>
  );
}
