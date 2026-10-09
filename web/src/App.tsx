import {
  Activity,
  AlertTriangle,
  Archive as ArchiveIcon,
  ArrowRight,
  BookOpen,
  CheckCircle2,
  Clock,
  Database,
  ExternalLink,
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
  User,
} from "lucide-react";
import { Icon as IconifyIcon } from "@iconify/react/offline";
import { createPortal } from "react-dom";
import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link, NavLink, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import AccountMenu from "./components/AccountMenu";
import Toast from "./components/Toast";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Alert, AlertDescription, AlertTitle } from "./components/ui/alert";
import { Button } from "./components/ui/button";
import { WalletModal } from "./components/WalletModal";
import { useWallet } from "./lib/WalletContext";
import { DOCS_URL, buildSearchPath, matchPageSuggestion, parseSearchQuery } from "./lib/search";
import { SEARCH_PREFIXES, parseSearchScope, searchSubmitTarget } from "./lib/search-prefix";
import {
  SEARCH_HISTORY_CATEGORIES,
  clearSearchHistory,
  loadSearchHistory,
  recordSearchHistory,
  type SearchHistory,
  type SearchHistoryCategory,
} from "./lib/search-history";

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
const LazyMonitor = lazy(() => import("./pages/Monitor"));
const LazyMapMonitor = lazy(() => import("./pages/MapMonitor"));
const LazyIpfsExplorer = lazy(() => import("./pages/IpfsExplorer"));
const LazyArchive = lazy(() => import("./pages/Archive"));
const LazyArchiveOwner = lazy(() => import("./pages/ArchiveOwner"));

const primaryNav = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/monitor", label: "Monitor", icon: Gauge },
  { to: "/mapmonitor", label: "MapMonitor", icon: MapIcon },
  { to: "/ipfs", label: "IPFS", icon: HardDrive },
  // V1 Archive is reachable at /archive/cosmwasm-v1 (direct URL, search, and
  // monitor links) but no longer has a nav entry; Search covers discovery.
  { to: "/settings", label: "Settings", icon: SettingsIcon },
];

// The top bar omits Monitor (MapMonitor covers the monitoring view); the
// left Workspace sidebar and mobile nav keep the full primaryNav list.
const topNav = primaryNav.filter((item) => item.to !== "/monitor");

// Entries beyond this index render in the top nav only on wide viewports
// (see the .topnav-link-more media query); side/mobile nav always show all.
const primaryTopnavCount = 4;

// Icons for built-in page suggestions surfaced inline in the global search
// dropdown (same mapping as the search results page's SUGGESTION_ICONS).
const PAGE_SUGGESTION_ICONS: Record<string, typeof LayoutDashboard> = {
  Dashboard: LayoutDashboard,
  Monitor: Gauge,
  MapMonitor: MapIcon,
  Activity,
  IPFS: HardDrive,
  "V1 Archive": ArchiveIcon,
  Settings: SettingsIcon,
  Docs: BookOpen,
};

type SuiteReadiness = "unconfigured" | "checking" | "ready" | "error";
type CacheNotification = "refreshing" | "refreshed" | null;

/** The search box draft is kept for the browser session only. */
const SEARCH_DRAFT_KEY = "search_draft";

/** Section icons for the categorized search history dropdown. */
const HISTORY_SECTION_ICONS: Record<SearchHistoryCategory, typeof User> = {
  owners: User,
  recent: Clock,
  repos: GitFork,
};

const HISTORY_SECTION_LABELS: Record<SearchHistoryCategory, string> = {
  owners: "Owners",
  recent: "Recent",
  repos: "Repos",
};

function readSearchDraft(): string {
  try {
    return sessionStorage.getItem(SEARCH_DRAFT_KEY) ?? "";
  } catch {
    return "";
  }
}

function RouteSpinner() {
  return <div className="spinner" aria-live="polite">loading...</div>;
}

export default function App() {
  const nav = useNavigate();
  const location = useLocation();
  // The draft survives navigation and reloads within the tab session, so
  // reopening the search box restores the last input instead of clearing it.
  const [q, setQ] = useState(readSearchDraft);
  // GitHub-style search card: opened from the topbar trigger, rendered as a
  // centered modal (same overlay as the wallet dialog).
  const [searchOpen, setSearchOpen] = useState(false);
  const [history, setHistory] = useState<SearchHistory>(loadSearchHistory);
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
  const searchInputRef = useRef<HTMLInputElement>(null);
  const historyListRef = useRef<HTMLDivElement>(null);
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

  // Persist the search draft for this tab session so reopening the search
  // box (or reloading the page) restores the last input.
  useEffect(() => {
    try {
      if (q) sessionStorage.setItem(SEARCH_DRAFT_KEY, q);
      else sessionStorage.removeItem(SEARCH_DRAFT_KEY);
    } catch {
      // Storage unavailable: the draft only lives in state.
    }
  }, [q]);

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

  /** Record a submitted query into its history category (prefix-aware). */
  const recordQueryHistory = useCallback((query: string) => {
    const scope = parseSearchScope(query);
    const category: SearchHistoryCategory = scope?.kind === "user" && !scope.keywords
      ? "owners"
      : scope?.kind === "repo"
        ? "repos"
        : "recent";
    setHistory(recordSearchHistory(category, query));
  }, []);

  /** Open the centered search card (trigger click or "/" shortcut). */
  const openSearch = useCallback(() => {
    // Re-read storage: the /search page records into the shared history
    // while the card is closed. A fresh open never carries a stale
    // keyboard/mouse highlight from the previous session.
    setHistory(loadSearchHistory());
    setHighlight(-1);
    setSearchOpen(true);
  }, []);

  const closeSearch = useCallback(() => setSearchOpen(false), []);

  // Autofocus the card input on open and select the restored draft so fresh
  // typing replaces it in one stroke.
  useEffect(() => {
    if (!searchOpen) return;
    const input = searchInputRef.current;
    input?.focus();
    input?.select();
  }, [searchOpen]);

  // Lock background scrolling while the card is open (wallet modal pattern).
  useEffect(() => {
    if (!searchOpen) return;
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
  }, [searchOpen]);

  // Escape closes the card from anywhere (input events bubble to window).
  useEffect(() => {
    if (!searchOpen) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeSearch();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [searchOpen, closeSearch]);

  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      const isEditing = target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable;
      if (event.key === "/" && !isEditing) {
        event.preventDefault();
        openSearch();
      }
    };
    window.addEventListener("keydown", focusSearch);
    return () => window.removeEventListener("keydown", focusSearch);
  }, [openSearch]);

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
    setHistory(recordSearchHistory("repos", `${owner}/${name}`));
    nav(`/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`);
    setSearchOpen(false);
    setRepoResults([]);
    setHighlight(-1);
  };

  const submitSearch = (rawQuery = q) => {
    const query = rawQuery.trim();
    if (!query) return;
    recordQueryHistory(query);
    // Archive schemes keep their direct paths; a bare `user:x` prefix points
    // straight at the owner page; everything else goes to the dedicated
    // search route (qualifier preserved) so keywords never collide with
    // app routes. The input draft is intentionally kept for the next open.
    const parsed = parseSearchQuery(query);
    if (parsed.archive && parsed.parts.length > 0) {
      const archiveTarget = buildSearchPath(query);
      if (!archiveTarget) return;
      nav(archiveTarget);
      setSearchOpen(false);
      return;
    }
    const target = searchSubmitTarget(query);
    if (!target) return;
    if (target.kind === "owner") {
      nav(`/${encodeURIComponent(target.owner)}`);
    } else {
      nav(`/search?q=${encodeURIComponent(target.query)}`);
    }
    setSearchOpen(false);
  };

  const clearHistorySection = (category?: SearchHistoryCategory) => {
    setHistory(clearSearchHistory(category));
  };

  const indexBuilding = q.trim().length >= 2 && (indexStatus?.building ?? false);

  const inputScope = useMemo(() => parseSearchScope(q), [q]);
  const submitTarget = useMemo(() => searchSubmitTarget(q), [q]);

  // Built-in page queries ("docs", "settings", "ipfs", ...) surface their
  // target directly in the dropdown before repo results — Enter still submits
  // a normal search. An explicit qualifier prefix means the user already
  // chose a direction, so no page hint is shown then.
  const pageSuggestion = useMemo(() => {
    if (inputScope) return null;
    return matchPageSuggestion(q);
  }, [q, inputScope]);

  const PageHintIcon =
    pageSuggestion === null ? null : PAGE_SUGGESTION_ICONS[pageSuggestion.label] ?? LayoutDashboard;

  const closeSearchUi = () => {
    setSearchOpen(false);
    setRepoResults([]);
    setHighlight(-1);
  };

  const openDocsHint = () => {
    closeSearchUi();
    window.open(DOCS_URL, "_blank", "noopener,noreferrer");
  };

  const openPageHint = (to: string) => {
    closeSearchUi();
    nav(to);
  };

  // Flat option list for keyboard navigation: live repo results first, then
  // the Owners / Recent / Repos history sections in display order.
  const historyOptions = useMemo(
    () => SEARCH_HISTORY_CATEGORIES.flatMap((category) => history[category].map((value) => ({ category, value }))),
    [history],
  );
  const optionCount = repoResults.length + historyOptions.length;
  const historyTotal = historyOptions.length;
  const sectionOffsets = useMemo(() => {
    let offset = repoResults.length;
    const offsets: Record<SearchHistoryCategory, number> = { owners: 0, recent: 0, repos: 0 };
    for (const category of SEARCH_HISTORY_CATEGORIES) {
      offsets[category] = offset;
      offset += history[category].length;
    }
    return offsets;
  }, [repoResults.length, history]);

  // Keep the keyboard-highlighted option visible inside the scrollable panel.
  useEffect(() => {
    if (highlight < 0) return;
    historyListRef.current
      ?.querySelector<HTMLElement>('[data-hl="true"]')
      ?.scrollIntoView({ block: "nearest" });
  }, [highlight]);

  // Keyboard interaction inside the search card input: arrows walk the flat
  // option list (repo results, then Owners / Recent / Repos), Enter runs the
  // highlighted option (or submits), Escape closes the card.
  const onSearchCardKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
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
      closeSearch();
      return;
    }
    if (event.key === "Enter" && !event.nativeEvent.isComposing) {
      event.preventDefault();
      if (highlight >= 0 && highlight < repoResults.length) {
        void openRepo(repoResults[highlight]);
      } else {
        const option = highlight >= repoResults.length
          ? historyOptions[highlight - repoResults.length]
          : undefined;
        if (option) submitSearch(option.value);
        else submitSearch();
      }
    }
  };

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="brand" aria-label="igit dashboard">
          <img className="brand-mark" src="/igit-image.png" alt="" width={28} height={28} />
          <span>igit</span>
        </Link>

        <nav className="topnav" aria-label="Primary navigation">
          {topNav.map((item, index) => (
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

        <div className="search">
          <button
            type="button"
            className="search-trigger"
            onClick={openSearch}
            aria-label="Search repositories, owners, or addresses"
            title="Search (press /)"
          >
            <Search size={15} aria-hidden="true" />
            <span className="search-trigger-text">{q.trim() || "Search user:owner repo:name keywords..."}</span>
            <kbd className="search-trigger-kbd" aria-hidden="true">/</kbd>
          </button>
          {searchOpen && createPortal(
            // Portal to document.body: the topbar's backdrop-filter creates a
            // containing block that would otherwise pin this fixed overlay to
            // the header instead of centering it over the viewport.
            <div className="modal-overlay" onClick={closeSearch}>
              <div
                className="modal search-modal"
                role="dialog"
                aria-modal="true"
                aria-label="Search repositories, owners, or addresses"
                onClick={(event) => event.stopPropagation()}
              >
                <div className="search-modal-input-row">
                  <Search size={16} aria-hidden="true" />
                  <input
                    ref={searchInputRef}
                    value={q}
                    onChange={(event) => setQ(event.target.value)}
                    onKeyDown={onSearchCardKeyDown}
                    placeholder="Search user:owner repo:name keywords..."
                    aria-label="Search repositories, owners, or addresses"
                    spellCheck={false}
                  />
                  {indexBuilding ? (
                    <LoaderCircle size={14} className="animate-spin" aria-hidden="true" />
                  ) : (
                    <kbd className="search-trigger-kbd" aria-hidden="true">↵</kbd>
                  )}
                </div>
                <div className="search-modal-body" ref={historyListRef} role="listbox">
                  {pageSuggestion?.href && (
                    <button
                      type="button"
                      className="search-history-item search-docs-hint"
                      onClick={openDocsHint}
                      role="option"
                      aria-selected={false}
                    >
                      <BookOpen size={14} aria-hidden="true" />
                      <span className="search-docs-hint-text">
                        <span className="search-action-label">Looking for</span> <b>{pageSuggestion.label}</b>?
                        <span className="muted small">docs.igit.xyz</span>
                      </span>
                      <ExternalLink size={12} aria-hidden="true" />
                    </button>
                  )}
                  {pageSuggestion?.to && PageHintIcon && (
                    <button
                      type="button"
                      className="search-history-item search-page-hint"
                      onClick={() => openPageHint(pageSuggestion.to ?? "/")}
                      role="option"
                      aria-selected={false}
                    >
                      <PageHintIcon size={14} aria-hidden="true" />
                      <span className="search-docs-hint-text">
                        <span className="search-action-label">Go to</span> <b>{pageSuggestion.label}</b>
                      </span>
                      <ArrowRight size={12} aria-hidden="true" />
                    </button>
                  )}
                  {q.trim().length > 0 && (
                    <button
                      type="button"
                      className="search-history-item search-repo-action"
                      onClick={() => submitSearch()}
                      role="option"
                    >
                      {submitTarget?.kind === "owner" ? (
                        <>
                          <span className="search-action-label">Go to owner</span>{" "}
                          <span className="mono">{submitTarget.owner}</span>
                        </>
                      ) : (
                        <>
                          <span className="search-action-label">Search for</span> <span className="mono">{q.trim()}</span>
                        </>
                      )}
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
                          data-hl={highlight === index ? "true" : undefined}
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
                  {SEARCH_HISTORY_CATEGORIES.map((category) => {
                    const items = history[category];
                    if (items.length === 0) return null;
                    const offset = sectionOffsets[category];
                    const Icon = HISTORY_SECTION_ICONS[category];
                    return (
                      <div key={category}>
                        <div className="search-history-head">
                          <span className="muted small">{HISTORY_SECTION_LABELS[category]}</span>
                          <button
                            type="button"
                            className="search-history-clear"
                            onClick={() => clearHistorySection(category)}
                          >
                            Clear
                          </button>
                        </div>
                        {items.map((item, index) => (
                          <button
                            key={`${category}:${item}`}
                            type="button"
                            className={`search-history-item search-history-entry${highlight === offset + index ? " on" : ""}`}
                            onClick={() => submitSearch(item)}
                            role="option"
                            aria-selected={highlight === offset + index}
                            data-hl={highlight === offset + index ? "true" : undefined}
                            onMouseEnter={() => setHighlight(offset + index)}
                          >
                            <Icon size={12} className="search-history-icon" aria-hidden="true" />
                            <span className="search-history-value">{item}</span>
                          </button>
                        ))}
                      </div>
                    );
                  })}
                  {q.trim().length === 0 && historyTotal === 0 && repoResults.length === 0 && !indexBuilding && (
                    <div className="search-history-item search-repo-pending">
                      No recent searches yet — try user:owner or repo:name.
                    </div>
                  )}
                </div>
                <div className="search-modal-footer">
                  <div className="search-prefix-hint" aria-hidden="true">
                    {SEARCH_PREFIXES.map((prefix) => (
                      <span key={prefix.name} className="search-prefix-hint-item">
                        <code>{prefix.name}:</code>
                        <span>{prefix.hint}</span>
                      </span>
                    ))}
                  </div>
                  <span className="search-modal-keys" aria-hidden="true">↑↓ navigate · ↵ open · esc close</span>
                </div>
              </div>
            </div>,
            document.body,
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
            <a
              className="side-nav-item side-nav-docs"
              href={DOCS_URL}
              target="_blank"
              rel="noreferrer"
            >
              <BookOpen size={15} aria-hidden="true" />
              <span>Docs</span>
              <ExternalLink size={11} className="side-nav-docs-external" aria-hidden="true" />
            </a>
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
            <span>EVM + Cosmwasm on Inj</span>
            <span>Packfiles on IPFS + Buckets</span>
            <span>Direct-to-Chain CLI</span>
          </footer>
        </div>
      </div>
    </div>
  );
}
