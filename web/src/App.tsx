import {
  Activity,
  AlertTriangle,
  Archive as ArchiveIcon,
  CheckCircle2,
  Database,
  GitFork,
  HardDrive,
  Gauge,
  LayoutDashboard,
  LoaderCircle,
  Map as MapIcon,
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
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Alert, AlertDescription, AlertTitle } from "./components/ui/alert";
import { Button } from "./components/ui/button";
import { WalletModal } from "./components/WalletModal";
import { useWallet } from "./lib/WalletContext";
import { buildSearchPath, parseSearchQuery } from "./lib/search";

import { ContractTypeBadge } from "./components/ContractTypeBadge";
import { repoIndexShared, resolveEntryTarget, searchRepoEntries, type RepoIndexEntry, type RepoIndexStatus } from "./lib/repo-index";

import { truncateAddress } from "./lib/utils";
import {
  CONFIG_CHANGED_EVENT,
  listRepos,
  isSuiteDirectoryConfigured,
  loadConfig,
  verifySuite,
  onVerificationEvent,
} from "./lib/chain";
import Home from "./pages/Home";
import SearchPage from "./pages/Search";
import Settings from "./pages/Settings";
import { isCosmWasmV1ArchivePath } from "./pages/Repo/useRepoViews";
import "./lib/architecture-icons";

const LazyOwner = lazy(() => import("./pages/Owner"));
const LazyRepo = lazy(() => import("./pages/Repo/index"));
const LazyExplorer = lazy(() => import("./pages/Explorer"));
const LazyMonitor = lazy(() => import("./pages/Monitor"));
const LazyMapMonitor = lazy(() => import("./pages/MapMonitor"));
const LazyIpfsExplorer = lazy(() => import("./pages/IpfsExplorer"));
const LazyArchive = lazy(() => import("./pages/Archive"));
const LazyArchiveOwner = lazy(() => import("./pages/ArchiveOwner"));

const primaryNav = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/monitor", label: "Monitor", icon: Gauge },
  { to: "/mapmonitor", label: "MapMonitor", icon: MapIcon },
  { to: "/explorer", label: "Activity", icon: Activity },
  { to: "/ipfs", label: "IPFS", icon: HardDrive },
  { to: "/archive/cosmwasm-v1", label: "V1 Archive", icon: ArchiveIcon },
  { to: "/settings", label: "Settings", icon: SettingsIcon },
];

// Entries beyond this index render in the top nav only on wide viewports
// (see the .topnav-link-more media query); side/mobile nav always show all.
const primaryTopnavCount = 4;

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
  const [repoResults, setRepoResults] = useState<RepoIndexEntry[]>([]);
  const [indexStatus, setIndexStatus] = useState<RepoIndexStatus | null>(null);
  const [highlight, setHighlight] = useState(-1);
  const [configRevision, setConfigRevision] = useState(0);
  const [suiteReadiness, setSuiteReadiness] = useState<SuiteReadiness>("checking");
  const [cacheNotification, setCacheNotification] = useState<CacheNotification>(null);
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    const saved = localStorage.getItem("igit_theme");
    if (saved === "light" || saved === "dark") return saved;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });
  const { address, connected, walletModalOpen, openWalletModal, closeWalletModal } = useWallet();
  const searchRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const cfg = useMemo(() => loadConfig(), [configRevision]);
  const isArchiveRoute = isCosmWasmV1ArchivePath(location.pathname);
  // Monitor and MapMonitor are read-only public pages that must stay usable
  // before a SuiteDirectory is configured, like the archive routes.
  const isMonitorRoute = location.pathname === "/monitor" || location.pathname === "/mapmonitor";
  const isMapMonitorRoute = location.pathname === "/mapmonitor";

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

  useEffect(() => {
    const indexer = repoIndexShared(cfg);
    setIndexStatus(indexer.getStatus());
    const unsubscribe = indexer.subscribe(setIndexStatus);
    indexer.ensureStarted();
    return unsubscribe;
  }, [cfg]);
  useEffect(() => {
    if (!address) return;
    let cancelled = false;
    // Enumerate the connected wallet's repositories by contract reads so they
    // are searchable regardless of log retention.
    listRepos(cfg, address)
      .then((repos) => {
        if (cancelled) return;
        repoIndexShared(cfg).absorbOwnerRepos(repos.map((repo) => ({
          owner: repo.owner,
          name: repo.name,
          moderation: repo.moderation_status,
          suiteVersion: repo.suite_version != null ? Number(repo.suite_version) : undefined,
        })));
      })
      .catch(() => {
        // Enumeration is best-effort; search keeps its other sources.
      });
    return () => {
      cancelled = true;
    };
  }, [address, cfg]);

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

  useEffect(() => {
    const query = q.trim();
    if (query.length < 2) {
      setRepoResults([]);
      setHighlight(-1);
      return;
    }
    const timer = setTimeout(() => {
      const indexer = repoIndexShared(cfg);
      setRepoResults(searchRepoEntries(indexer.entries, query, 7));
      setHighlight(-1);
    }, 250);
    return () => clearTimeout(timer);
  }, [q, cfg, indexStatus]);

  const openRepo = async (entry: RepoIndexEntry) => {
    // The index is navigation-only; the contract read is authoritative.
    const { owner, name } = await resolveEntryTarget(cfg, entry);
    addToHistory(`${owner}/${name}`);
    nav(`/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`);
    setQ("");
    setShowHistory(false);
    setRepoResults([]);
    setHighlight(-1);
  };

  const submitSearch = (rawQuery = q) => {
    const query = rawQuery.trim();
    if (!query) return;
    addToHistory(query);
    // Archive schemes keep their direct paths; everything else goes to the
    // dedicated search route so keywords never collide with app routes.
    const parsed = parseSearchQuery(query);
    const target = parsed.archive && parsed.parts.length > 0
      ? buildSearchPath(query)
      : `/search?q=${encodeURIComponent(query)}`;
    if (!target) return;
    nav(target);
    setQ("");
    setShowHistory(false);
  };

  const clearHistory = () => {
    setHistory([]);
    localStorage.removeItem("search_history");
  };

  const indexBuilding = q.trim().length >= 2 && (indexStatus?.building ?? false);

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="brand" aria-label="igit dashboard">
          <span className="brand-mark"><GitFork size={16} strokeWidth={2.25} /></span>
          <span>igit</span>
          <span className="brand-sub">Injective</span>
        </Link>

        <nav className="topnav" aria-label="Primary navigation">
          {primaryNav.map((item, index) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `topnav-link${index >= primaryTopnavCount ? " topnav-link-more" : ""}${isActive ? " on" : ""}`
              }
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
                const optionCount = repoResults.length + history.length;
                if ((event.key === "ArrowDown" || event.key === "ArrowUp") && optionCount > 0) {
                  event.preventDefault();
                  setHighlight((current) => {
                    const delta = event.key === "ArrowDown" ? 1 : -1;
                    const next = current + delta;
                    if (next < 0) return optionCount - 1;
                    if (next >= optionCount) return 0;
                    return next;
                  });
                  return;
                }
                if (event.key === "Escape") {
                  setShowHistory(false);
                  return;
                }
                if (event.key === "Enter" && !event.nativeEvent.isComposing) {
                  event.preventDefault();
                  if (highlight >= 0 && highlight < repoResults.length) {
                    void openRepo(repoResults[highlight]);
                  } else if (highlight >= repoResults.length && highlight - repoResults.length < history.length) {
                    submitSearch(history[highlight - repoResults.length]);
                  } else {
                    submitSearch();
                  }
                }
              }}
              placeholder="Search repositories, owners, addresses..."
              aria-label="Search repositories, owners, or addresses"
              spellCheck={false}
            />
          </form>
          {showHistory && (q.trim().length > 0 || history.length > 0 || repoResults.length > 0 || indexBuilding) && (
            <div className="search-history" role="listbox">
              {q.trim().length > 0 && (
                <button
                  type="button"
                  className="search-history-item search-repo-action"
                  onClick={() => submitSearch()}
                  role="option"
                >
                  <span className="search-action-label">Search for</span> <span className="mono">{q.trim()}</span>
                </button>
              )}
              {repoResults.length === 0 && indexBuilding && (
                <div className="search-history-item search-repo-pending">Building repository index from chain...</div>
              )}
              {repoResults.length > 0 && (
                <>
                  <div className="search-history-head">
                    <span className="muted small">Repositories</span>
                    {indexBuilding && <span className="muted small">indexing...</span>}
                  </div>
                  {repoResults.map((entry, index) => (
                    <button
                      key={`${entry.suiteDirectory}:${entry.repoId ?? `${entry.owner}/${entry.name}/v${entry.suiteVersion}`}`}
                      type="button"
                      className={`search-history-item search-repo-item${highlight === index ? " on" : ""}`}
                      role="option"
                      aria-selected={highlight === index}
                      onMouseEnter={() => setHighlight(index)}
                      onClick={() => void openRepo(entry)}
                    >
                      <span className="search-repo-name">
                        {entry.name}
                        <ContractTypeBadge kind="evm-v2" suiteVersion={BigInt(entry.suiteVersion)} />
                      </span>
                      <span className="search-repo-meta">
                        {truncateAddress(entry.owner, 12)}
                        <span className={`badge ${entry.status === 1 ? "frozen" : entry.status === 2 ? "delisted" : "active"}`}>
                          {entry.status === 1 ? "frozen" : entry.status === 2 ? "delisted" : "active"}
                        </span>
                      </span>
                    </button>
                  ))}
                </>
              )}
              <div className="search-history-head">
                <span className="muted small">Recent</span>
                <button type="button" className="search-history-clear" onClick={clearHistory}>Clear</button>
              </div>
              {history.map((item, index) => (
                <button
                  key={item}
                  type="button"
                  className={`search-history-item${highlight === repoResults.length + index ? " on" : ""}`}
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
              <span><b>EVM V2/V3</b><small>Current repositories · packs on IPFS</small></span>
            </div>
            <div className="side-nav-meta">
              <span className="side-nav-meta-icon"><Database size={14} aria-hidden="true" /></span>
              <span><b>EVM V4</b><small>Successor suite · BYOS storage buckets</small></span>
            </div>
            <div className="side-nav-meta">
              <span className="side-nav-meta-icon"><IconifyIcon icon="brand:cosmwasm" width={14} height={14} aria-hidden="true" /></span>
              <span><b>CosmWasm V1</b><small>Read-only repository archive</small></span>
            </div>
            <div className="side-nav-meta">
              <span className="side-nav-meta-icon"><IconifyIcon icon="simple-icons:ipfs" width={14} height={14} aria-hidden="true" /></span>
              <span><b>IPFS</b><small>V2/V3 packfile storage</small></span>
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

          <main className={isMapMonitorRoute ? "content content-mapmonitor" : "content"}>
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
            <ErrorBoundary>
              <Routes>
                <Route path="/" element={<Home />} />
                <Route path="/search" element={<SearchPage />} />
                <Route path="/settings" element={<Settings />} />
                <Route path="/monitor" element={<Suspense fallback={<RouteSpinner />}><LazyMonitor /></Suspense>} />
                <Route path="/mapmonitor" element={<Suspense fallback={<RouteSpinner />}><LazyMapMonitor /></Suspense>} />
                <Route path="/explorer" element={<Suspense fallback={<RouteSpinner />}><LazyExplorer /></Suspense>} />
                <Route path="/ipfs" element={<Suspense fallback={<RouteSpinner />}><LazyIpfsExplorer /></Suspense>} />
                <Route path="/archive/cosmwasm-v1" element={<Suspense fallback={<RouteSpinner />}><LazyArchive /></Suspense>} />
                <Route path="/archive/cosmwasm-v1/:owner" element={<Suspense fallback={<RouteSpinner />}><LazyArchiveOwner /></Suspense>} />
                <Route path="/archive/cosmwasm-v1/:owner/:repo/*" element={<Suspense fallback={<RouteSpinner />}><LazyRepo key="cosmwasm-v1" contractKind="cosmwasm-v1" /></Suspense>} />
                <Route path="/:owner" element={<Suspense fallback={<RouteSpinner />}><LazyOwner /></Suspense>} />
                <Route path="/:owner/:repo/*" element={<Suspense fallback={<RouteSpinner />}><LazyRepo key="evm-v2" /></Suspense>} />
              </Routes>
            </ErrorBoundary>
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
