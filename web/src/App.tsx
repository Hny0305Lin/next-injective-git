import { Link, Route, Routes, useNavigate } from "react-router-dom";
import { Suspense, lazy, useState } from "react";
import Home from "./pages/Home";
import Settings from "./pages/Settings";
import { useWallet } from "./lib/WalletContext";
import { WalletModal } from "./components/WalletModal";
import AccountMenu from "./components/AccountMenu";
import Toast from "./components/Toast";

const LazyOwner = lazy(() => import("./pages/Owner"));
const LazyRepo = lazy(() => import("./pages/Repo/index"));
const LazyExplorer = lazy(() => import("./pages/Explorer"));
const LazyIpfsExplorer = lazy(() => import("./pages/IpfsExplorer"));

function RouteSpinner() {
  return <div className="spinner" aria-live="polite">loading…</div>;
}

export default function App() {
  const nav = useNavigate();
  const [q, setQ] = useState("");
  const { connected, walletModalOpen, openWalletModal, closeWalletModal } = useWallet();

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const s = q.trim();
    if (!s) return;
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
        <nav className="topnav">
          <Link to="/" className="topnav-link">Dashboard</Link>
          <Link to="/explorer" className="topnav-link">Explore</Link>
          <Link to="/ipfs" className="topnav-link">IPFS</Link>
        </nav>
        <form className="search" onSubmit={submit}>
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="search owner, repo, address…"
            spellCheck={false}
          />
        </form>
        <div className="topbar-end">
          <Link to="/settings" className="topnav-link">Settings</Link>
          {connected ? <AccountMenu /> : (
            <button className="wallet-btn" onClick={openWalletModal}>Connect</button>
          )}
        </div>
      </header>
      {walletModalOpen && <WalletModal onClose={closeWalletModal} />}
      <Toast />
      <main className="content">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/explorer" element={<Suspense fallback={<RouteSpinner />}><LazyExplorer /></Suspense>} />
          <Route path="/ipfs" element={<Suspense fallback={<RouteSpinner />}><LazyIpfsExplorer /></Suspense>} />
          <Route path="/:owner" element={<Suspense fallback={<RouteSpinner />}><LazyOwner /></Suspense>} />
          <Route path="/:owner/:repo/*" element={<Suspense fallback={<RouteSpinner />}><LazyRepo /></Suspense>} />
        </Routes>
      </main>
      <footer className="footer">
        refs on <b>Injective</b> · packfiles on <b>IPFS</b> · no backend — this page talks to the chain directly
      </footer>
    </div>
  );
}
