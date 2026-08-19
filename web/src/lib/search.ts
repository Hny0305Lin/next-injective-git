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
