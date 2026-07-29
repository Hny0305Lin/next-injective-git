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

// Pull a human-readable message out of the many error shapes we hit:
// native Error, Injective SDK exceptions, MetaMask RPC errors ({code,message}),
// LCD broadcast responses ({raw_log}). Falls back to JSON so nothing shows as
// the useless "[object Object]".
export function formatError(e: unknown): string {
  if (e == null) return "unknown error";
  if (typeof e === "string") return e;
  if (e instanceof Error) return e.message;
  if (typeof e === "object") {
    const o = e as Record<string, unknown>;
    const nested = o.data ?? o.error; // MetaMask often nests under .data/.error
    for (const k of ["originalMessage", "message", "rawLog", "raw_log", "reason"]) {
      const v = o[k];
      if (typeof v === "string" && v) return v;
    }
    if (nested && nested !== e) {
      const inner = formatError(nested);
      if (inner && inner !== "[object Object]") return inner;
    }
    try {
      return JSON.stringify(e);
    } catch {
      return String(e);
    }
  }
  return String(e);
}

/** INJ balance (base units) of any address via the LCD bank module. */
export async function injBalanceOf(cfg: AppConfig, address: string): Promise<string> {
  const url = `${cfg.lcd.replace(/\/+$/, "")}/cosmos/bank/v1beta1/balances/${address}`;
  const resp = await fetch(url);
  if (!resp.ok) return "0";
  const json = await resp.json();
  const inj = (json.balances ?? []).find((c: { denom: string }) => c.denom === "inj");
  return inj?.amount ?? "0";
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

// ---- block explorer: contract-scoped tx feed + single tx detail ----

export interface ContractTx {
  txhash: string;
  height: string;
  timestamp: string;
  code: number;
  action: string; // wasm.action (create_repo / update_ref / sponsor / ...)
  sender: string;
  wasm: Record<string, string>; // remaining wasm attributes (repo, ref, sha, ...)
}

interface RawTxResp {
  txhash: string;
  height: string;
  timestamp: string;
  code: number;
  raw_log?: string;
  events?: { type: string; attributes?: { key: string; value: string }[] }[];
}

/** Every tx that touched the repo-registry contract, newest first. */
export async function contractActivity(cfg: AppConfig, limit = 50): Promise<ContractTx[]> {
  const query = `wasm._contract_address='${cfg.contract}'`;
  const url =
    `${cfg.lcd.replace(/\/+$/, "")}/cosmos/tx/v1beta1/txs?query=${encodeURIComponent(query)}` +
    `&order_by=ORDER_BY_DESC&pagination.limit=${limit}`;
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`tx query failed (HTTP ${resp.status})`);
  const json = await resp.json();
  const out: ContractTx[] = [];
  for (const tx of (json.tx_responses ?? []) as RawTxResp[]) {
    const wasm: Record<string, string> = {};
    let sender = "";
    for (const ev of tx.events ?? []) {
      if (ev.type === "wasm") {
        for (const a of ev.attributes ?? []) {
          if (a.key !== "_contract_address") wasm[a.key] = a.value;
        }
      } else if (ev.type === "message") {
        for (const a of ev.attributes ?? []) if (a.key === "sender") sender = a.value;
      }
    }
    out.push({
      txhash: tx.txhash,
      height: tx.height,
      timestamp: tx.timestamp,
      code: tx.code ?? 0,
      action: wasm.action ?? "",
      sender,
      wasm,
    });
  }
  return out;
}

export interface TxDetail {
  txhash: string;
  height: string;
  timestamp: string;
  code: number;
  rawLog: string;
  gasUsed: string;
  gasWanted: string;
  messages: { type: string; body: Record<string, unknown> }[];
  extensionOptions: string[];
  signMode: string;
  pubkeyType: string;
  events: { type: string; attributes: { key: string; value: string }[] }[];
}

/** Full detail of one tx by hash (null if not found / not indexed yet). */
export async function txByHash(cfg: AppConfig, hash: string): Promise<TxDetail | null> {
  const clean = hash.trim().replace(/^0x/, "").toUpperCase();
  const url = `${cfg.lcd.replace(/\/+$/, "")}/cosmos/tx/v1beta1/txs/${clean}`;
  const resp = await fetch(url);
  if (resp.status === 404) return null;
  if (!resp.ok) throw new Error(`tx lookup failed (HTTP ${resp.status})`);
  const json = await resp.json();
  const r = json.tx_response;
  const body = json.tx?.body ?? {};
  const auth = json.tx?.auth_info ?? {};
  const signer = (auth.signer_infos ?? [])[0] ?? {};
  return {
    txhash: r.txhash,
    height: r.height,
    timestamp: r.timestamp,
    code: r.code ?? 0,
    rawLog: r.raw_log ?? "",
    gasUsed: r.gas_used ?? "",
    gasWanted: r.gas_wanted ?? "",
    messages: (body.messages ?? []).map((m: Record<string, unknown>) => ({
      type: String(m["@type"] ?? ""),
      body: m,
    })),
    extensionOptions: (body.extension_options ?? []).map((e: Record<string, unknown>) =>
      String(e["@type"] ?? ""),
    ),
    signMode: String(signer.mode_info?.single?.mode ?? ""),
    pubkeyType: String(signer.public_key?.["@type"] ?? ""),
    events: (r.events ?? []).map((e: { type: string; attributes?: { key: string; value: string }[] }) => ({
      type: e.type,
      attributes: e.attributes ?? [],
    })),
  };
}
