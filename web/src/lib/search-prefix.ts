/**
 * Extensible qualifier-prefix registry for the global search box, inspired
 * by GitHub's `org:` / `user:` / `repo:` search qualifiers.
 *
 * A prefix token looks like `user:alice` (no space after the colon). Tokens
 * whose prefix is not registered — a future `docs:` or `search:` until they
 * are added, URL schemes such as `igit://`, or ordinary words — are left
 * untouched and fall back to plain keyword search, so registering a new
 * prefix later is a one-entry change to SEARCH_PREFIXES plus optional
 * handling at the call sites (App submit routing, Search page scoping).
 */

/** Result scopes understood today; future prefixes add kinds here. */
export type SearchScopeKind = "user" | "repo";

export interface SearchPrefixDefinition {
  /** Token matched before the colon (case-insensitive). */
  readonly name: string;
  /** Scope kind applied to the query when the prefix matches. */
  readonly kind: SearchScopeKind;
  /** Short hint rendered in the global search dropdown footer. */
  readonly hint: string;
}

export const SEARCH_PREFIXES: readonly SearchPrefixDefinition[] = [
  { name: "user", kind: "user", hint: "限定到某个用户（owner），如 user:alice" },
  { name: "repo", kind: "repo", hint: "限定到仓库，如 repo:name 或 repo:owner/name" },
];

const PREFIX_TOKEN = /^([a-z][a-z0-9_-]*):([^\s:]*)$/i;
const registeredPrefixes = new Map(SEARCH_PREFIXES.map((prefix) => [prefix.name.toLowerCase(), prefix]));

export interface SearchScope {
  kind: SearchScopeKind;
  /** Qualifier value: username/address, repo name, or owner/name. */
  value: string;
  /** Remaining free-text keywords after the qualifier token. */
  keywords: string;
}

/**
 * Extract the first recognized qualifier prefix from a raw query.
 * Returns null when the query carries no registered prefix.
 */
export function parseSearchScope(raw: string): SearchScope | null {
  const tokens = raw.trim().split(/\s+/).filter(Boolean);
  for (let index = 0; index < tokens.length; index += 1) {
    const match = PREFIX_TOKEN.exec(tokens[index]);
    if (!match) continue;
    const definition = registeredPrefixes.get(match[1].toLowerCase());
    if (!definition) continue;
    const value = match[2].trim().replace(/^@+/, "");
    if (!value) continue;
    const keywords = tokens.filter((_, position) => position !== index).join(" ").trim();
    return { kind: definition.kind, value, keywords };
  }
  return null;
}

export type SearchSubmitTarget =
  | { kind: "owner"; owner: string }
  | { kind: "query"; query: string };

/**
 * Decide where the global search box submits a raw query: a bare `user:x`
 * points straight at the owner page; everything else goes to /search?q=
 * with the qualifier preserved so the search page can scope its results.
 */
export function searchSubmitTarget(raw: string): SearchSubmitTarget | null {
  const query = raw.trim();
  if (!query) return null;
  const scope = parseSearchScope(query);
  if (scope?.kind === "user" && !scope.keywords) return { kind: "owner", owner: scope.value };
  return { kind: "query", query };
}
