/** Search target parsing shared by the global search UI and its tests. */
const SEARCH_SCHEME = /^(?:igit|inj|archive|cosmwasm|v1):\/\//i;

export interface SearchQuery {
  archive: boolean;
  parts: string[];
}

function decodePart(part: string): string {
  try {
    return decodeURIComponent(part);
  } catch {
    // Keep malformed history entries searchable; route encoding below
    // still keeps the value inside a single path segment.
    return part;
  }
}

/**
 * Parse the owner/repository forms accepted by the global search field.
 *
 * Usernames are commonly copied with an `@` display prefix, while clone URLs
 * may carry one of the supported schemes.  Neither belongs in the route
 * parameter, so normalize them before the router sees the value.
 */
export function parseSearchQuery(raw: string): SearchQuery {
  const query = raw.trim();
  const archive = /^(?:archive|cosmwasm|v1):\/\//i.test(query);
  const withoutScheme = query.replace(SEARCH_SCHEME, "");
  const parts = withoutScheme
    .split("/")
    .map((part) => decodePart(part.trim()).trim())
    .filter(Boolean);
  if (parts.length > 0) {
    parts[0] = parts[0].replace(/^@+/, "").trim();
    if (!parts[0]) return { archive, parts: [] };
  }
  return { archive, parts };
}

/** Build a router path for a search query, or null when it has no owner. */
export function buildSearchPath(raw: string): string | null {
  const { archive, parts } = parseSearchQuery(raw);
  if (parts.length === 0) return null;
  const prefix = archive ? "/archive/cosmwasm-v1" : "";
  const target = parts.slice(0, 2).map((part) => encodeURIComponent(part)).join("/");
  return `${prefix}/${target}`;
}

/**
 * Built-in app pages that a query may be aiming for. Suggesting them keeps
 * searches like "settings" or "ipfs" working even when the keyword collides
 * with a registered username or repository name.
 */
export interface PageSuggestion {
  label: string;
  /** Internal router path for in-app destinations. */
  to?: string;
  /** External URL (documentation site); opens in a new tab. */
  href?: string;
  keywords: readonly string[];
}

/** Project documentation site linked from the Workspace nav and search hints. */
export const DOCS_URL = "https://docs.igit.xyz/";

export const PAGE_SUGGESTIONS: readonly PageSuggestion[] = [
  { label: "Dashboard", to: "/", keywords: ["dashboard", "home"] },
  { label: "Monitor", to: "/monitor", keywords: ["monitor"] },
  { label: "MapMonitor", to: "/mapmonitor", keywords: ["mapmonitor", "map monitor", "map"] },
  { label: "Activity", to: "/explorer", keywords: ["explorer", "activity"] },
  { label: "IPFS", to: "/ipfs", keywords: ["ipfs"] },
  { label: "V1 Archive", to: "/archive/cosmwasm-v1", keywords: ["archive", "v1 archive", "cosmwasm"] },
  { label: "Settings", to: "/settings", keywords: ["settings", "setting"] },
  // "doc" needs no explicit keyword: 3+ character prefix matching maps it to
  // "docs" while keeping "docker"-style queries unmatched.
  { label: "Docs", href: DOCS_URL, keywords: ["docs", "documentation", "文档"] },
];

/**
 * Match a query against built-in page keywords: exact, query-prefix (sett ->
 * settings), or page-prefix for 3+ character queries (map -> mapmonitor).
 */
export function matchPageSuggestion(raw: string): PageSuggestion | null {
  const query = raw.trim().toLowerCase();
  if (query.length < 2) return null;
  for (const suggestion of PAGE_SUGGESTIONS) {
    for (const keyword of suggestion.keywords) {
      if (query === keyword) return suggestion;
      if (query.startsWith(keyword)) return suggestion;
      if (query.length >= 3 && keyword.startsWith(query)) return suggestion;
    }
  }
  return null;
}