/**
 * Categorized search history for the global search dropdown: Owners,
 * Recent keyword searches, and opened Repos each keep their own records.
 *
 * Stored in localStorage under `search_history_v2`; the legacy flat
 * `search_history` list migrates into `recent` on first read. Both the
 * App search box and the /search page record through this module, so the
 * dropdown simply re-reads on focus.
 */

export type SearchHistoryCategory = "owners" | "recent" | "repos";

export interface SearchHistory {
  owners: string[];
  recent: string[];
  repos: string[];
}

const HISTORY_KEY = "search_history_v2";
const LEGACY_HISTORY_KEY = "search_history";
const HISTORY_LIMIT = 5;

export const SEARCH_HISTORY_CATEGORIES: readonly SearchHistoryCategory[] = ["owners", "recent", "repos"];

const EMPTY_HISTORY: SearchHistory = { owners: [], recent: [], repos: [] };

function cleanList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((item): item is string => typeof item === "string" && item.trim().length > 0)
    .slice(0, HISTORY_LIMIT);
}

export function loadSearchHistory(): SearchHistory {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<Record<SearchHistoryCategory, unknown>>;
      return {
        owners: cleanList(parsed.owners),
        recent: cleanList(parsed.recent),
        repos: cleanList(parsed.repos),
      };
    }
    // Migrate the legacy flat history into the Recent section once.
    const legacy = localStorage.getItem(LEGACY_HISTORY_KEY);
    if (legacy) {
      const parsed = JSON.parse(legacy);
      if (Array.isArray(parsed)) return { ...EMPTY_HISTORY, recent: cleanList(parsed) };
    }
  } catch {
    // Corrupted or unavailable storage falls back to empty history.
  }
  return { owners: [], recent: [], repos: [] };
}

function saveSearchHistory(history: SearchHistory): void {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(history));
  } catch {
    // Quota or privacy mode: keep history in memory for this render only.
  }
}

/**
 * Prepend a value to one category, deduplicating and capping the list.
 * Returns the updated history (also persisted).
 */
export function recordSearchHistory(category: SearchHistoryCategory, value: string): SearchHistory {
  const entry = value.trim();
  if (!entry) return loadSearchHistory();
  const current = loadSearchHistory();
  const next: SearchHistory = {
    ...current,
    [category]: [entry, ...current[category].filter((item) => item !== entry)].slice(0, HISTORY_LIMIT),
  };
  saveSearchHistory(next);
  return next;
}

/** Clear one category, or the whole history (including legacy) when omitted. */
export function clearSearchHistory(category?: SearchHistoryCategory): SearchHistory {
  if (!category) {
    try {
      localStorage.removeItem(HISTORY_KEY);
      localStorage.removeItem(LEGACY_HISTORY_KEY);
    } catch {
      // Storage unavailable; the in-memory copy below is still emptied.
    }
    return { owners: [], recent: [], repos: [] };
  }
  const next: SearchHistory = { ...loadSearchHistory(), [category]: [] };
  saveSearchHistory(next);
  return next;
}
