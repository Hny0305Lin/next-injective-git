// Chain access layer: wasm smart queries against the repo-registry
// contract through the public LCD REST endpoint. Mirrors cli/internal/chain.

export interface AppConfig {
  lcd: string;
  contract: string;
  ipfsGateway: string;
}

const DEFAULTS: AppConfig = {
  // NOTE: the sentry LCD LB intermittently sends a duplicate
  // Access-Control-Allow-Origin header which browsers reject;
  // the k8s endpoint behaves correctly for browser use.
  lcd: "https://k8s.testnet.lcd.injective.network",
  contract: "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh",
  ipfsGateway: "https://ipfs.io",
};

const LS_KEY = "igit-web-config";

export function loadConfig(): AppConfig {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) return { ...DEFAULTS, ...JSON.parse(raw) };
  } catch {
    /* fall through to defaults */
  }
  return { ...DEFAULTS };
}

export function saveConfig(cfg: AppConfig) {
  localStorage.setItem(LS_KEY, JSON.stringify(cfg));
}

// ---- contract response types (mirror msg.rs) ----

export interface RepoInfo {
  owner: string;
  name: string;
  description: string;
  default_branch: string;
  created_at: number;
  updated_at: number;
  moderation_status: "active" | "delisted" | "frozen";
  forked_from: string | null;
}

export interface RefInfo {
  ref_name: string;
  commit_sha: string;
  pack_uris: string[];
  updated_at: number;
  updated_by: string;
}

export interface SplitEntry {
  address: string;
  bps: number;
}

export interface SponsorTotal {
  denom: string;
  amount: string;
}

export interface ContractConfig {
  admin: string;
  moderation_committee: string | null;
  treasury: string;
  platform_fee_bps: number;
  username_deposit: { denom: string; amount: string };
}

export interface CollaboratorInfo {
  address: string;
  role: "maintainer" | "reader";
}

async function smartQuery<T>(cfg: AppConfig, query: unknown): Promise<T> {
  const encoded = btoa(JSON.stringify(query));
  const url = `${cfg.lcd.replace(/\/+$/, "")}/cosmwasm/wasm/v1/contract/${
    cfg.contract
  }/smart/${encodeURIComponent(encoded)}`;
  const resp = await fetch(url);
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`query failed (HTTP ${resp.status}): ${body.slice(0, 300)}`);
  }
  const json = await resp.json();
  return json.data as T;
}

export async function listRepos(cfg: AppConfig, owner: string): Promise<RepoInfo[]> {
  const out = await smartQuery<{ repos: RepoInfo[] }>(cfg, {
    list_repos: { owner, limit: 100 },
  });
  return out.repos;
}

export async function repoInfo(cfg: AppConfig, owner: string, repo: string): Promise<RepoInfo> {
  return smartQuery<RepoInfo>(cfg, { repo_info: { owner, repo } });
}

export async function listRefs(cfg: AppConfig, owner: string, repo: string): Promise<RefInfo[]> {
  const out = await smartQuery<{ refs: RefInfo[] }>(cfg, {
    list_refs: { owner, repo, limit: 100 },
  });
  return out.refs;
}

export async function listCollaborators(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<CollaboratorInfo[]> {
  const out = await smartQuery<{ collaborators: CollaboratorInfo[] }>(cfg, {
    list_collaborators: { owner, repo, limit: 100 },
  });
  return out.collaborators;
}

export async function revenueSplits(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<SplitEntry[]> {
  const out = await smartQuery<{ splits: SplitEntry[] }>(cfg, {
    revenue_splits: { owner, repo },
  });
  return out.splits;
}

export async function sponsorTotals(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<SponsorTotal[]> {
  const out = await smartQuery<{ totals: SponsorTotal[] }>(cfg, {
    sponsor_totals: { owner, repo },
  });
  return out.totals;
}

export async function contractConfig(cfg: AppConfig): Promise<ContractConfig> {
  return smartQuery<ContractConfig>(cfg, { config: {} });
}

export async function resolveUsername(cfg: AppConfig, name: string): Promise<string> {
  const out = await smartQuery<{ owner: string }>(cfg, {
    resolve_username: { name },
  });
  return out.owner;
}

export async function addressUsername(cfg: AppConfig, address: string): Promise<string | null> {
  const out = await smartQuery<{ name: string | null }>(cfg, {
    address_username: { address },
  });
  return out.name;
}

/// Resolve an owner input (address or username) to a bech32 address.
export async function resolveOwner(cfg: AppConfig, owner: string): Promise<string> {
  if (owner.startsWith("inj1")) return owner;
  return resolveUsername(cfg, owner);
}

/// Format base-unit INJ (1e18) into a short decimal string.
export function formatInj(amount: string, denom: string): string {
  if (denom !== "inj") return `${amount} ${denom}`;
  const s = amount.padStart(19, "0");
  const whole = s.slice(0, -18);
  const frac = s.slice(-18).replace(/0+$/, "");
  return frac ? `${whole}.${frac.slice(0, 6)} INJ` : `${whole} INJ`;
}

export function timeAgo(seconds: number): string {
  const diff = Date.now() / 1000 - seconds;
  if (diff < 60) return "just now";
  if (diff < 3600) return `${Math.floor(diff / 60)} min ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} h ago`;
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} d ago`;
  return new Date(seconds * 1000).toISOString().slice(0, 10);
}

// ---- sponsor wall: individual sponsorship events from tx history ----

export interface SponsorEvent {
  txhash: string;
  sponsor: string;
  funds: string; // e.g. "50000000000000000inj"
  message: string;
  timestamp: string; // ISO
}

/** Query past `sponsor` executions for a repo straight from the LCD tx index. */
export async function sponsorEvents(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<SponsorEvent[]> {
  const query = [
    `wasm.action='sponsor'`,
    `wasm._contract_address='${cfg.contract}'`,
    `wasm.owner='${owner}'`,
    `wasm.repo='${repo}'`,
  ].join(" AND ");
  const url =
    `${cfg.lcd.replace(/\/+$/, "")}/cosmos/tx/v1beta1/txs?query=${encodeURIComponent(query)}` +
    `&order_by=ORDER_BY_DESC&pagination.limit=50`;
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`tx query failed (HTTP ${resp.status})`);
  const json = await resp.json();
  const out: SponsorEvent[] = [];
  interface TxResp {
    txhash: string;
    timestamp: string;
    events?: { type: string; attributes?: { key: string; value: string }[] }[];
  }
  for (const tx of (json.tx_responses ?? []) as TxResp[]) {
    const attrs: Record<string, string> = {};
    for (const ev of tx.events ?? []) {
      if (ev.type !== "wasm") continue;
      for (const a of ev.attributes ?? []) attrs[a.key] = a.value;
    }
    if (attrs.action !== "sponsor") continue;
    out.push({
      txhash: tx.txhash,
      sponsor: attrs.sponsor ?? "",
      funds: attrs.funds ?? "",
      message: attrs.message ?? "",
      timestamp: tx.timestamp,
    });
  }
  return out;
}

/** "50000000000000000inj" -> "0.05 INJ" */
export function formatFunds(funds: string): string {
  const m = funds.match(/^(\d+)inj$/);
  if (!m) return funds;
  return formatInj(m[1], "inj");
}

// ---- contribution badges (§3 L1) ----

export interface BadgeInfo {
  id: number;
  repo_owner: string;
  repo_name: string;
  recipient: string;
  reason: string;
  awarded_at: number;
}

export async function badgesByRecipient(cfg: AppConfig, recipient: string): Promise<BadgeInfo[]> {
  const out = await smartQuery<{ badges: BadgeInfo[] }>(cfg, {
    badges_by_recipient: { recipient, limit: 100 },
  });
  return out.badges;
}

export async function badgesByRepo(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<BadgeInfo[]> {
  const out = await smartQuery<{ badges: BadgeInfo[] }>(cfg, {
    badges_by_repo: { owner, repo, limit: 100 },
  });
  return out.badges;
}
