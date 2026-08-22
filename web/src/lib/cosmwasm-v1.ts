import { base64 } from "@scure/base";
import { toInjectiveAddress } from "./address";
import type { ModerationStatus, RefInfo, RepoInfo } from "./registry";

/**
 * Browser-reachable LCD candidates for the V1 archive, tried in order. The
 * official sentry LCD drops a meaningful share of browser connections
 * (observed roughly 40% "Failed to fetch"), which used to render the whole V1
 * surface unavailable in the Monitor because it was the only endpoint. The
 * Polkachu community endpoint serves the same chain and contract state and
 * answers plain GETs with CORS `*`, but rejects CORS preflights, so requests
 * routed there must not carry the custom snapshot-height header.
 */
export interface CosmWasmV1LcdEndpoint {
  /** Base URL serving both /cosmwasm and /cosmos LCD routes. */
  lcd: string;
  /** Human-readable name used in diagnostics. */
  label: string;
  /**
   * Whether the custom x-cosmos-block-height header may be sent. The header
   * triggers a CORS preflight; endpoints whose preflight fails are queried
   * header-less at their latest committed state instead of failing hard.
   */
  pinnedHeightHeader: boolean;
}

export const COSMWASM_V1_LCD_ENDPOINTS: readonly CosmWasmV1LcdEndpoint[] = [
  {
    lcd: "https://testnet.sentry.lcd.injective.network",
    label: "Injective sentry LCD",
    pinnedHeightHeader: true,
  },
  {
    lcd: "https://injective-testnet-api.polkachu.com",
    label: "Polkachu community LCD",
    pinnedHeightHeader: false,
  },
];

export const COSMWASM_V1_ARCHIVE = {
  network: "Injective Testnet",
  lcd: COSMWASM_V1_LCD_ENDPOINTS[0].lcd,
  contract: "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh",
  explorer: "https://testnet.explorer.injective.network/contract/inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh/",
} as const;

const PAGE_SIZE = 100;
const MAX_PAGES = 1_000;
const MAX_RESPONSE_BYTES = 16 * 1024 * 1024;
const FETCH_ATTEMPTS = 3;
const FETCH_RETRY_DELAYS_MS = [200, 600] as const;
// Individual attempts are capped so a hanging LCD connection cannot leave the
// Monitor stuck on "Checking" forever; the budget stays inside the 90s
// refresh window even when every attempt times out.
const FETCH_TIMEOUT_MS = 10_000;
const latestBlockPath = "/cosmos/base/tendermint/v1beta1/blocks/latest";
let snapshotHeight: number | null = null;
let snapshotHeightPromise: Promise<number> | null = null;
// Sessions stick to whichever endpoint last answered so a flaky primary is
// not retried on every 90-second Monitor refresh; resetCosmWasmV1Snapshot()
// restores the canonical order.
let preferredEndpoint = 0;

type JsonRecord = Record<string, unknown>;

export class CosmWasmV1ArchiveError extends Error {
  constructor(message: string, public readonly status?: number) {
    super(message);
    this.name = "CosmWasmV1ArchiveError";
  }
}

export function formatCosmWasmV1Error(error: unknown, resource: "owner" | "repository"): string {
  if (!(error instanceof CosmWasmV1ArchiveError)) {
    return error instanceof Error ? error.message : String(error);
  }
  const message = error.message.toLowerCase();
  if (message.includes("not found") || message.includes("notfound") || message.includes("does not exist")) {
    return `Could not find this archived ${resource}. Check the ${resource === "owner" ? "address or username" : "owner and repository name"}.`;
  }
  if (message.includes("network error") || message.includes("failed to fetch")) {
    return "The CosmWasm V1 archive is temporarily unreachable. Check your connection and try again.";
  }
  return `CosmWasm V1 archive query failed: ${error.message}`;
}

function record(value: unknown, label: string): JsonRecord {
  if (value == null || typeof value !== "object" || Array.isArray(value)) {
    throw new CosmWasmV1ArchiveError(`CosmWasm V1 returned invalid ${label}`);
  }
  return value as JsonRecord;
}

async function readJsonBounded(response: Response, label: string): Promise<unknown> {
  const contentLength = response.headers?.get("content-length");
  if (contentLength) {
    const length = Number(contentLength);
    if (Number.isFinite(length) && length > MAX_RESPONSE_BYTES) {
      throw new CosmWasmV1ArchiveError(`CosmWasm V1 ${label} response is too large`);
    }
  }
  if (typeof response.arrayBuffer === "function") {
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.byteLength > MAX_RESPONSE_BYTES) {
      throw new CosmWasmV1ArchiveError(`CosmWasm V1 ${label} response is too large`);
    }
    try {
      return JSON.parse(new TextDecoder().decode(bytes)) as unknown;
    } catch {
      throw new CosmWasmV1ArchiveError(`CosmWasm V1 returned malformed ${label} JSON`, response.status);
    }
  }
  return response.json();
}

function stringField(value: unknown, label: string): string {
  if (typeof value !== "string") {
    throw new CosmWasmV1ArchiveError(`CosmWasm V1 returned invalid ${label}`);
  }
  return value;
}

function numberField(value: unknown, label: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    throw new CosmWasmV1ArchiveError(`CosmWasm V1 returned invalid ${label}`);
  }
  return value;
}

function moderationStatus(value: unknown): ModerationStatus {
  if (value === "active" || value === "frozen" || value === "delisted") return value;
  throw new CosmWasmV1ArchiveError("CosmWasm V1 returned an invalid moderation status");
}

function archiveOwner(value: string): string {
  try {
    return toInjectiveAddress(value);
  } catch {
    throw new CosmWasmV1ArchiveError(`invalid Injective owner address: ${value}`);
  }
}

function encodeQuery(query: JsonRecord): string {
  const bytes = new TextEncoder().encode(JSON.stringify(query));
  return encodeURIComponent(base64.encode(bytes));
}

async function fetchWithRetry(input: string, init: RequestInit): Promise<Response> {
  let lastError: unknown;
  for (let attempt = 0; attempt < FETCH_ATTEMPTS; attempt += 1) {
    const controller = new AbortController();
    // The timer only bounds awaiting the response headers; it is cleared as
    // soon as fetch resolves so bounded body reads are never aborted mid-way.
    const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
    try {
      return await fetch(input, { ...init, signal: controller.signal });
    } catch (error) {
      lastError = error;
      if (attempt + 1 < FETCH_ATTEMPTS) {
        const delay = FETCH_RETRY_DELAYS_MS[Math.min(attempt, FETCH_RETRY_DELAYS_MS.length - 1)];
        await new Promise((resolve) => setTimeout(resolve, delay));
      }
    } finally {
      clearTimeout(timer);
    }
  }
  throw lastError;
}

function endpointOrder(): CosmWasmV1LcdEndpoint[] {
  const endpoints = [...COSMWASM_V1_LCD_ENDPOINTS];
  // Rotate so the preferred endpoint is tried first without reordering the
  // exported canonical list.
  return endpoints.slice(preferredEndpoint).concat(endpoints.slice(0, preferredEndpoint));
}

async function loadSnapshotHeight(): Promise<number> {
  let lastNetworkError: unknown;
  let lastStatus: number | null = null;
  for (const endpoint of endpointOrder()) {
    let response: Response;
    try {
      response = await fetchWithRetry(`${endpoint.lcd}${latestBlockPath}`, {
        method: "GET",
        headers: { Accept: "application/json" },
      });
    } catch (cause) {
      lastNetworkError = cause;
      continue;
    }
    if (!response.ok) {
      lastStatus = response.status;
      continue;
    }
    const payload = record(await readJsonBounded(response, "latest block"), "latest block response");
    const block = record(payload.block, "latest block");
    const header = record(block.header, "latest block header");
    const rawHeight = stringField(header.height, "latest block height");
    const height = Number(rawHeight);
    if (!Number.isSafeInteger(height) || height <= 0) {
      throw new CosmWasmV1ArchiveError("CosmWasm V1 returned an invalid latest block height");
    }
    preferredEndpoint = COSMWASM_V1_LCD_ENDPOINTS.indexOf(endpoint);
    return height;
  }
  if (lastNetworkError != null) {
    const labels = COSMWASM_V1_LCD_ENDPOINTS.map((endpoint) => endpoint.label).join(", ");
    throw new CosmWasmV1ArchiveError(
      lastNetworkError instanceof Error
        ? `CosmWasm V1 network error (${labels}): ${lastNetworkError.message}`
        : `CosmWasm V1 network error (${labels})`,
    );
  }
  throw new CosmWasmV1ArchiveError(
    `CosmWasm V1 latest block query failed on every endpoint (last HTTP ${lastStatus ?? "unknown"})`,
    lastStatus ?? undefined,
  );
}

async function getSnapshotHeight(): Promise<number> {
  if (snapshotHeight != null) return snapshotHeight;
  snapshotHeightPromise ??= loadSnapshotHeight()
    .then((height) => {
      snapshotHeight = height;
      return height;
    })
    .finally(() => {
      snapshotHeightPromise = null;
    });
  return snapshotHeightPromise;
}

export function getCosmWasmV1SnapshotHeight(): number | null {
  return snapshotHeight;
}

export function prepareCosmWasmV1Snapshot(): Promise<number> {
  return getSnapshotHeight();
}

export function resetCosmWasmV1Snapshot(): void {
  snapshotHeight = null;
  snapshotHeightPromise = null;
  preferredEndpoint = 0;
}

export async function queryCosmWasmV1<T>(query: JsonRecord): Promise<T> {
  const encoded = encodeQuery(query);
  const height = await getSnapshotHeight();
  let lastNetworkError: unknown;
  let lastServerError: CosmWasmV1ArchiveError | null = null;
  for (const endpoint of endpointOrder()) {
    const url = `${endpoint.lcd}/cosmwasm/wasm/v1/contract/${COSMWASM_V1_ARCHIVE.contract}/smart/${encoded}`;
    // The pinned-height header triggers a CORS preflight; endpoints whose
    // preflight rejects custom headers are queried at their latest committed
    // state instead. The archive is write-frozen, so the bounded drift is
    // preferable to failing the read outright.
    const headers: Record<string, string> = { Accept: "application/json" };
    if (endpoint.pinnedHeightHeader) headers["x-cosmos-block-height"] = String(height);
    let response: Response;
    try {
      response = await fetchWithRetry(url, { method: "GET", headers });
    } catch (cause) {
      lastNetworkError = cause;
      continue;
    }

    if (response.status >= 500) {
      // Server-side failures (rate limits, pruned pinned state, node errors)
      // are endpoint-local: move on to the next candidate.
      lastServerError = new CosmWasmV1ArchiveError(
        `CosmWasm V1 query failed (HTTP ${response.status})`,
        response.status,
      );
      continue;
    }

    let payload: unknown;
    try {
      payload = await readJsonBounded(response, "query");
    } catch (cause) {
      if (cause instanceof CosmWasmV1ArchiveError) throw cause;
      throw new CosmWasmV1ArchiveError(
        `CosmWasm V1 returned malformed JSON (HTTP ${response.status})`,
        response.status,
      );
    }

    const envelope = record(payload, "response");
    if (!response.ok) {
      // 4xx bodies are deterministic contract-level answers (e.g. unknown
      // repository); they must not be retried against another endpoint.
      const message = typeof envelope.message === "string"
        ? envelope.message
        : `CosmWasm V1 query failed (HTTP ${response.status})`;
      throw new CosmWasmV1ArchiveError(message, response.status);
    }
    if (!("data" in envelope)) {
      throw new CosmWasmV1ArchiveError("CosmWasm V1 response is missing data", response.status);
    }
    preferredEndpoint = COSMWASM_V1_LCD_ENDPOINTS.indexOf(endpoint);
    return envelope.data as T;
  }
  if (lastServerError != null) throw lastServerError;
  const labels = COSMWASM_V1_LCD_ENDPOINTS.map((endpoint) => endpoint.label).join(", ");
  throw new CosmWasmV1ArchiveError(
    lastNetworkError instanceof Error
      ? `CosmWasm V1 network error (${labels}): ${lastNetworkError.message}`
      : `CosmWasm V1 network error (${labels})`,
  );
}

function parseRepo(value: unknown): RepoInfo {
  const raw = record(value, "repository");
  return {
    owner: archiveOwner(stringField(raw.owner, "repository owner")),
    name: stringField(raw.name, "repository name"),
    description: stringField(raw.description, "repository description"),
    default_branch: stringField(raw.default_branch, "default branch"),
    created_at: numberField(raw.created_at, "repository created timestamp"),
    updated_at: numberField(raw.updated_at, "repository updated timestamp"),
    moderation_status: moderationStatus(raw.moderation_status),
    forked_from: raw.forked_from == null ? null : stringField(raw.forked_from, "fork source"),
  };
}

function parseRef(value: unknown): RefInfo {
  const raw = record(value, "repository ref");
  if (!Array.isArray(raw.pack_uris)) {
    throw new CosmWasmV1ArchiveError("CosmWasm V1 returned invalid pack URIs");
  }
  return {
    ref_name: stringField(raw.ref_name, "ref name"),
    commit_sha: stringField(raw.commit_sha, "commit SHA"),
    pack_uris: raw.pack_uris.map((uri) => stringField(uri, "pack URI")),
    updated_at: numberField(raw.updated_at, "ref updated timestamp"),
    updated_by: archiveOwner(stringField(raw.updated_by, "ref updater")),
  };
}

export async function resolveCosmWasmV1Owner(owner: string): Promise<string> {
  const input = owner.trim();
  if (!input) throw new CosmWasmV1ArchiveError("owner is required");
  try {
    return toInjectiveAddress(input);
  } catch {
    const data = record(
      await queryCosmWasmV1<unknown>({ resolve_username: { name: input } }),
      "username",
    );
    return archiveOwner(stringField(data.owner, "username owner"));
  }
}

export async function cosmWasmV1RepoInfo(owner: string, repo: string): Promise<RepoInfo> {
  const address = await resolveCosmWasmV1Owner(owner);
  const name = repo.trim();
  if (!name) throw new CosmWasmV1ArchiveError("repository name is required");
  return parseRepo(await queryCosmWasmV1<unknown>({ repo_info: { owner: address, repo: name } }));
}

export async function listCosmWasmV1Repos(owner: string): Promise<RepoInfo[]> {
  const address = await resolveCosmWasmV1Owner(owner);
  const repositories: RepoInfo[] = [];
  let startAfter: string | null = null;

  for (let pageNumber = 0; pageNumber < MAX_PAGES; pageNumber += 1) {
    const data = record(await queryCosmWasmV1<unknown>({
      list_repos: { owner: address, start_after: startAfter, limit: PAGE_SIZE },
    }), "repository list");
    if (!Array.isArray(data.repos)) {
      throw new CosmWasmV1ArchiveError("CosmWasm V1 returned an invalid repository list");
    }
    const page = data.repos.map(parseRepo);
    repositories.push(...page);
    if (page.length < PAGE_SIZE) return repositories;
    const next = page.at(-1)?.name;
    if (!next || next === startAfter) {
      throw new CosmWasmV1ArchiveError("CosmWasm V1 repository cursor did not advance");
    }
    startAfter = next;
  }
  throw new CosmWasmV1ArchiveError("CosmWasm V1 repository pagination exceeded its safety limit");
}

export async function listCosmWasmV1Refs(owner: string, repo: string): Promise<RefInfo[]> {
  const address = await resolveCosmWasmV1Owner(owner);
  const name = repo.trim();
  if (!name) throw new CosmWasmV1ArchiveError("repository name is required");
  const refs: RefInfo[] = [];
  let startAfter: string | null = null;

  for (let pageNumber = 0; pageNumber < MAX_PAGES; pageNumber += 1) {
    const data = record(await queryCosmWasmV1<unknown>({
      list_refs: { owner: address, repo: name, start_after: startAfter, limit: PAGE_SIZE },
    }), "ref list");
    if (!Array.isArray(data.refs)) {
      throw new CosmWasmV1ArchiveError("CosmWasm V1 returned an invalid ref list");
    }
    const page = data.refs.map(parseRef);
    refs.push(...page);
    if (page.length < PAGE_SIZE) return refs;
    const next = page.at(-1)?.ref_name;
    if (!next || next === startAfter) {
      throw new CosmWasmV1ArchiveError("CosmWasm V1 ref cursor did not advance");
    }
    startAfter = next;
  }
  throw new CosmWasmV1ArchiveError("CosmWasm V1 ref pagination exceeded its safety limit");
}
