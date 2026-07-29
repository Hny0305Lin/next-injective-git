import { Link, Route, Routes, useNavigate } from "react-router-dom";
import { useState } from "react";
import Home from "./pages/Home";
import Owner from "./pages/Owner";
import Repo from "./pages/Repo";
import Settings from "./pages/Settings";
import { useWallet } from "./lib/WalletContext";
import { WalletModal } from "./components/WalletModal";
import { formatInj } from "./lib/chain";

export default function App() {
  const nav = useNavigate();
  const [q, setQ] = useState("");
  const { address, balance, connected, disconnect } = useWallet();
  const [walletModal, setWalletModal] = useState(false);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const s = q.trim();
    if (!s) return;
    // "owner/repo" jumps straight into the repo, otherwise owner page
    const parts = s.replace(/^(igit|inj):\/\//, "").split("/").filter(Boolean);
    if (parts.length >= 2) nav(`/${parts[0]}/${parts[1]}`);
    else nav(`/${parts[0]}`);
    setQ("");
  };

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="brand">
          <span className="brand-mark">⎇</span> igit
          <span className="brand-sub">on Injective</span>
        </Link>
        <form className="search" onSubmit={submit}>
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="address, username or owner/repo…"
            spellCheck={false}
          />
        </form>
        <nav className="topnav">
          <Link to="/settings">Settings</Link>
          <a href="https://github.com/Hny0305Lin/next-injective-git" target="_blank" rel="noreferrer">
            CLI
          </a>
          {address ? (
            <button
              className="wallet-btn connected"
              title={`${connected?.label ?? ""} · ${address}\nclick to disconnect`}
              onClick={disconnect}
            >
              {balance ? `${formatInj(balance, "inj")} · ` : ""}
              {address.slice(0, 7)}…{address.slice(-4)}
            </button>
          ) : (
            <button className="wallet-btn" onClick={() => setWalletModal(true)}>
              Connect Wallet
            </button>
          )}
        </nav>
      </header>
      {walletModal && <WalletModal onClose={() => setWalletModal(false)} />}
      <main className="content">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/:owner" element={<Owner />} />
          <Route path="/:owner/:repo/*" element={<Repo />} />
        </Routes>
      </main>
      <footer className="footer">
        refs on <b>Injective</b> · packfiles on <b>IPFS</b> · no backend — this page talks to the
        chain directly
      </footer>
    </div>
  );
}
