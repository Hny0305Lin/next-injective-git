import {
  Activity,
  AlertTriangle,
  Archive as ArchiveIcon,
  CheckCircle2,
  GitFork,
  HardDrive,
  Gauge,
  LayoutDashboard,
  LoaderCircle,
  Moon,
  Search,
  Settings as SettingsIcon,
  Sun,
} from "lucide-react";
import { Icon as IconifyIcon } from "@iconify/react/offline";
import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, NavLink, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import AccountMenu from "./components/AccountMenu";
import Toast from "./components/Toast";
import { Alert, AlertDescription, AlertTitle } from "./components/ui/alert";
import { Button } from "./components/ui/button";
import { WalletModal } from "./components/WalletModal";
import { useWallet } from "./lib/WalletContext";
import { buildSearchPath } from "./lib/search";
import {
  CONFIG_CHANGED_EVENT,
  isSuiteDirectoryConfigured,
  loadConfig,
  verifySuite,
  onVerificationEvent,
} from "./lib/chain";
import Home from "./pages/Home";
import Settings from "./pages/Settings";
import { isCosmWasmV1ArchivePath } from "./pages/Repo/useRepoViews";
import "./lib/architecture-icons";

const LazyOwner = lazy(() => import("./pages/Owner"));
const LazyRepo = lazy(() => import("./pages/Repo/index"));
const LazyExplorer = lazy(() => import("./pages/Explorer"));
const LazyMonitor = lazy(() => import("./pages/Monitor"));
const LazyIpfsExplorer = lazy(() => import("./pages/IpfsExplorer"));
const LazyArchive = lazy(() => import("./pages/Archive"));
const LazyArchiveOwner = lazy(() => import("./pages/ArchiveOwner"));

const primaryNav = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/monitor", label: "Monitor", icon: Gauge },
  { to: "/explorer", label: "Activity", icon: Activity },
  { to: "/ipfs", label: "IPFS", icon: HardDrive },
  { to: "/archive/cosmwasm-v1", label: "V1 Archive", icon: ArchiveIcon },
  { to: "/settings", label: "Settings", icon: SettingsIcon },
];

type SuiteReadiness = "unconfigured" | "checking" | "ready" | "error";
type CacheNotification = "refreshing" | "refreshed" | null;

function RouteSpinner() {
  return <div className="spinner" aria-live="polite">loading...</div>;
}

export default function App() {
  const nav = useNavigate();
  const location = useLocation();
  const [q, setQ] = useState("");
  const [showHistory, setShowHistory] = useState(false);
  const [history, setHistory] = useState<string[]>([]);
  const [configRevision, setConfigRevision] = useState(0);
  const [suiteReadiness, setSuiteReadiness] = useState<SuiteReadiness>("checking");
  const [cacheNotification, setCacheNotification] = useState<CacheNotification>(null);
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    const saved = localStorage.getItem("igit_theme");
    if (saved === "light" || saved === "dark") return saved;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const { connected, walletModalOpen, openWalletModal, closeWalletModal } = useWallet();
  const searchRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const cfg = useMemo(() => loadConfig(), [configRevision]);
  const isArchiveRoute = isCosmWasmV1ArchivePath(location.pathname);
  const isMonitorRoute = location.pathname === "/monitor";

  useEffect(() => {
    const refreshConfig = () => setConfigRevision((revision) => revision + 1);
    window.addEventListener(CONFIG_CHANGED_EVENT, refreshConfig);
    return () => window.removeEventListener(CONFIG_CHANGED_EVENT, refreshConfig);
  }, []);

  useEffect(() => {
    const unsubscribe = onVerificationEvent((event) => {
      if (event.type === "started") {
        setCacheNotification("refreshing");
      } else if (event.type === "completed") {
        setCacheNotification("refreshed");
        setTimeout(() => setCacheNotification(null), 2000);
      } else if (event.type === "cached") {
        setCacheNotification(null);
      }
    });
    return unsubscribe;
  }, []);

  useEffect(() => {
    if (isArchiveRoute) {
      setSuiteReadiness("ready");
      return;
    }
    if (!isSuiteDirectoryConfigured(cfg.suiteDirectory)) {
      setSuiteReadiness("unconfigured");
      return;
    }
    let cancelled = false;
    setSuiteReadiness("checking");
    verifySuite(cfg)
      .then(() => {
        if (!cancelled) setSuiteReadiness("ready");
      })
      .catch(() => {
        if (!cancelled) setSuiteReadiness("error");
      });
    return () => {
      cancelled = true;
    };
  }, [cfg, isArchiveRoute]);

  useEffect(() => {
    document.documentElement.classList.toggle("dark", theme === "dark");
    localStorage.setItem("igit_theme", theme);
  }, [theme]);

  useEffect(() => {
    try {
      const saved = localStorage.getItem("search_history");
      if (saved) setHistory(JSON.parse(saved));
    } catch {}
  }, []);

  const addToHistory = useCallback((query: string) => {
    setHistory((previous) => {
      const next = [query, ...previous.filter((item) => item !== query)].slice(0, 5);
      localStorage.setItem("search_history", JSON.stringify(next));
      return next;
    });
  }, []);

  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      if (searchRef.current && !searchRef.current.contains(event.target as Node)) {
        setShowHistory(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      const isEditing = target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable;
      if (event.key === "/" && !isEditing) {
        event.preventDefault();
        searchInputRef.current?.focus();
      }
    };
    window.addEventListener("keydown", focusSearch);
    return () => window.removeEventListener("keydown", focusSearch);
  }, []);

  const submitSearch = (rawQuery = q) => {
    const query = rawQuery.trim();
    if (!query) return;
    const path = buildSearchPath(query);
    if (!path) return;
    addToHistory(query);
    nav(path);
    setQ("");
    setShowHistory(false);
  };

  const clearHistory = () => {
    setHistory([]);
    localStorage.removeItem("search_history");
  };

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="brand" aria-label="igit dashboard">
          <span className="brand-mark"><GitFork size={16} strokeWidth={2.25} /></span>
          <span>igit</span>
          <span className="brand-sub">Injective</span>
        </Link>

        <nav className="topnav" aria-label="Primary navigation">
          {primaryNav.slice(0, 4).map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) => `topnav-link${isActive ? " on" : ""}`}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="search" ref={searchRef}>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              submitSearch();
            }}
          >
            <Search className="search-icon" size={15} aria-hidden="true" />
            <input
              ref={searchInputRef}
              value={q}
              onChange={(event) => setQ(event.target.value)}
              onFocus={() => setShowHistory(true)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.nativeEvent.isComposing) {
                  event.preventDefault();
                  submitSearch();
                }
              }}
              placeholder="Search owner, repository, address..."
              aria-label="Search owner, repository, or address"
              spellCheck={false}
            />
          </form>
          {showHistory && history.length > 0 && (
            <div className="search-history" role="listbox">
              <div className="search-history-head">
                <span className="muted small">Recent</span>
                <button type="button" className="search-history-clear" onClick={clearHistory}>Clear</button>
              </div>
              {history.map((item) => (
                <button
                  key={item}
                  type="button"
                  className="search-history-item"
                  onClick={() => submitSearch(item)}
                  role="option"
                >
                  {item}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="topbar-end">
          <NavLink
            to="/settings"
            className={({ isActive }) => `topbar-icon${isActive ? " on" : ""}`}
            aria-label="Settings"
            title="Settings"
          >
            <SettingsIcon size={16} />
          </NavLink>
          <Button
            variant="ghost"
            size="icon"
            className="topbar-icon"
            onClick={() => setTheme(theme === "light" ? "dark" : "light")}
            aria-label={`Switch to ${theme === "light" ? "dark" : "light"} theme`}
            title={`Switch to ${theme === "light" ? "dark" : "light"} theme`}
          >
            {theme === "light" ? <Moon size={16} /> : <Sun size={16} />}
          </Button>
          {connected ? (
            <AccountMenu />
          ) : (
            <Button className="wallet-btn" onClick={openWalletModal}>Connect wallet</Button>
          )}
          {cacheNotification && (
            <div className="cache-notification">
              <Alert variant="default">
                {cacheNotification === "refreshing" ? (
                  <LoaderCircle className="h-4 w-4 animate-spin" />
                ) : (
                  <CheckCircle2 className="h-4 w-4" />
                )}
                <AlertTitle>
                  {cacheNotification === "refreshing" ? "Refreshing cache" : "Cache updated"}
                </AlertTitle>
                <AlertDescription>
                  {cacheNotification === "refreshing"
                    ? "Verifying Suite directory and modules..."
                    : "Suite verification cached for 30 seconds"}
                </AlertDescription>
              </Alert>
            </div>
          )}
        </div>
      </header>

      {walletModalOpen && <WalletModal onClose={closeWalletModal} />}
      <Toast />

      <div className="app-shell">
        <aside className="side-nav" aria-label="Workspace navigation">
          <div className="side-nav-section">
            <div className="side-nav-label">Workspace</div>
            {primaryNav.map((item) => {
              const Icon = item.icon;
              return (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) => `side-nav-item${isActive ? " on" : ""}`}
                >
                  <Icon size={15} />
                  <span>{item.label}</span>
                </NavLink>
              );
            })}
          </div>

          <div className="side-nav-section architecture">
            <div className="side-nav-label">Architecture</div>
            <div className="side-nav-meta">
              <span className="side-nav-meta-icon"><IconifyIcon icon="mdi:ethereum" width={14} height={14} aria-hidden="true" /></span>
              <span><b>EVM V2</b><small>Current repositories</small></span>
            </div>
            <div className="side-nav-meta">
              <span className="side-nav-meta-icon"><IconifyIcon icon="token:cosmos" width={14} height={14} aria-hidden="true" /></span>
              <span><b>CosmWasm V1</b><small>Read-only repository archive</small></span>
            </div>
            <div className="side-nav-meta">
              <span className="side-nav-meta-icon"><IconifyIcon icon="simple-icons:ipfs" width={14} height={14} aria-hidden="true" /></span>
              <span><b>IPFS</b><small>Packfile storage</small></span>
            </div>
          </div>
        </aside>

        <div className="page-column">
          <nav className="mobile-nav" aria-label="Mobile navigation">
            {primaryNav.map((item) => {
              const Icon = item.icon;
              return (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) => `mobile-nav-item${isActive ? " on" : ""}`}
                >
                  <Icon size={14} />
                  <span>{item.label}</span>
                </NavLink>
              );
            })}
          </nav>

          <main className="content">
            {!isArchiveRoute && !isMonitorRoute && suiteReadiness !== "ready" && (
              <div className={`suite-alert suite-${suiteReadiness}`} role="status">
                {suiteReadiness === "checking"
                  ? <LoaderCircle className="suite-alert-spinner" size={17} />
                  : <AlertTriangle size={17} />}
                <div>
                  <strong>
                    {suiteReadiness === "checking" && "Verifying EVM Suite"}
                    {suiteReadiness === "unconfigured" && "EVM Suite is not configured"}
                    {suiteReadiness === "error" && "EVM Suite verification failed"}
                  </strong>
                  <span>
                    {suiteReadiness === "checking" && "Checking the Directory, modules, bindings, and code hashes."}
                    {suiteReadiness === "unconfigured" && "Wallet connection and INJ balance remain available, but repository data needs a verified SuiteDirectory."}
                    {suiteReadiness === "error" && "The saved Directory is not a valid active Suite for this network. Review the address in Settings."}
                  </span>
                </div>
                {suiteReadiness !== "checking" && <Link to="/settings">Configure</Link>}
              </div>
            )}
            <Routes>
              <Route path="/" element={<Home />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="/monitor" element={<Suspense fallback={<RouteSpinner />}><LazyMonitor /></Suspense>} />
              <Route path="/explorer" element={<Suspense fallback={<RouteSpinner />}><LazyExplorer /></Suspense>} />
              <Route path="/ipfs" element={<Suspense fallback={<RouteSpinner />}><LazyIpfsExplorer /></Suspense>} />
              <Route path="/archive/cosmwasm-v1" element={<Suspense fallback={<RouteSpinner />}><LazyArchive /></Suspense>} />
              <Route path="/archive/cosmwasm-v1/:owner" element={<Suspense fallback={<RouteSpinner />}><LazyArchiveOwner /></Suspense>} />
              <Route path="/archive/cosmwasm-v1/:owner/:repo/*" element={<Suspense fallback={<RouteSpinner />}><LazyRepo key="cosmwasm-v1" contractKind="cosmwasm-v1" /></Suspense>} />
              <Route path="/:owner" element={<Suspense fallback={<RouteSpinner />}><LazyOwner /></Suspense>} />
              <Route path="/:owner/:repo/*" element={<Suspense fallback={<RouteSpinner />}><LazyRepo key="evm-v2" /></Suspense>} />
            </Routes>
          </main>

          <footer className="footer">
            <span>EVM V2 and CosmWasm V1</span>
            <span>Packfiles on IPFS</span>
            <span>Direct-to-chain client</span>
          </footer>
        </div>
      </div>
    </div>
  );
}
