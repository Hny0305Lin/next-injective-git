// Chain access layer: wasm smart queries against the repo-registry
// contract through the public LCD REST endpoint. Mirrors cli/internal/chain.

import { fromBech32, toBech32, toHex } from "@cosmjs/encoding";
import type { Eip1193 } from "./wallet";

export type NetworkProfileId = "injective-testnet";
export type ContractVersion = "auto" | "v1" | "v2";

export interface NetworkProfile {
  id: NetworkProfileId;
  label: string;
  chainId: string;
  evmChainId: number;
  lcd: string;
  evmRpc: string;
  contract: string;
  evmContract: string;
  evmBadgeModule: string;
  evmEconomicModule: string;
  evmModerationModule: string;
  ipfsGateway: string;
  explorer: string;
  evmExplorer: string;
}

// Profile-owned endpoints keep RPC, chain ID, and contract details out of the
// user-facing settings. The V2 address is empty until the profile deploys it;
// auto mode consequently uses the V1 read path today.
export const NETWORK_PROFILES: Record<NetworkProfileId, NetworkProfile> = {
  "injective-testnet": {
    id: "injective-testnet",
    label: "Injective Testnet",
    chainId: "injective-888",
    evmChainId: 1439,
    lcd: "https://k8s.testnet.lcd.injective.network",
    evmRpc: "https://k8s.testnet.json-rpc.injective.network/",
    contract: "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh",
    evmContract: "",
    evmBadgeModule: "",
    evmEconomicModule: "",
    evmModerationModule: "",
    ipfsGateway: "https://igit-hk.haohanyh.ovh",
    explorer: "https://testnet.explorer.injective.network",
    evmExplorer: "https://testnet-injective.cloud.blockscout.com",
  },
};

export interface AppConfig {
  profile: NetworkProfileId;
  contractVersion: ContractVersion;
  lcd: string;
  contract: string;
  ipfsGateway: string;
  evmRpc: string;
  evmContract: string;
  evmBadgeModule: string;
  evmEconomicModule: string;
  evmModerationModule: string;
  evmChainId: number;
}

const DEFAULT_PROFILE: NetworkProfileId = "injective-testnet";
const DEFAULT_CONTRACT_VERSION: ContractVersion = "auto";

export function configForProfile(
  profile: NetworkProfileId = DEFAULT_PROFILE,
  contractVersion: ContractVersion = DEFAULT_CONTRACT_VERSION,
): AppConfig {
  const selected = NETWORK_PROFILES[profile] ?? NETWORK_PROFILES[DEFAULT_PROFILE];
  return {
    profile: selected.id,
    contractVersion,
    lcd: selected.lcd,
    contract: selected.contract,
    ipfsGateway: selected.ipfsGateway,
    evmRpc: selected.evmRpc,
    evmContract: selected.evmContract,
    evmBadgeModule: selected.evmBadgeModule,
    evmEconomicModule: selected.evmEconomicModule,
    evmModerationModule: selected.evmModerationModule,
    evmChainId: selected.evmChainId,
  };
}

const LS_KEY = "igit-web-config";

export function loadConfig(): AppConfig {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) {
      const saved = JSON.parse(raw) as Partial<AppConfig>;
      const profile = saved.profile && saved.profile in NETWORK_PROFILES
        ? saved.profile
        : DEFAULT_PROFILE;
      const contractVersion = saved.contractVersion === "v1" || saved.contractVersion === "v2"
        ? saved.contractVersion
        : DEFAULT_CONTRACT_VERSION;
      return configForProfile(profile, contractVersion);
    }
  } catch {
    /* fall through to defaults */
  }
  return configForProfile();
}

export function saveConfig(cfg: AppConfig) {
  localStorage.setItem(
    LS_KEY,
    JSON.stringify({ profile: cfg.profile, contractVersion: cfg.contractVersion }),
  );
}

export function networkProfile(cfg: AppConfig): NetworkProfile {
  return NETWORK_PROFILES[cfg.profile] ?? NETWORK_PROFILES[DEFAULT_PROFILE];
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

// ---- query cache (dedup + TTL for smart queries) ----

interface CacheEntry<T> {
  data: T;
  ts: number;
}

const queryCache = new Map<string, CacheEntry<unknown>>();
const CACHE_TTL_MS = 30_000; // 30 seconds

function cacheKey(cfg: AppConfig, query: unknown): string {
  // Stable key: profile/backend endpoints + serialized query.
  return `${cfg.profile}\x00${cfg.contractVersion}\x00${cfg.lcd}\x00${cfg.contract}\x00${btoa(JSON.stringify(query))}`;
}

export interface ResolvedRepo {
  repoId: string | null;
  backend: "evm" | "cosmwasm";
  requested: { owner: string; name: string };
  canonical: { owner: string; name: string };
  isCanonical: boolean;
  info: RepoInfo;
}

async function smartQuery<T>(cfg: AppConfig, query: unknown): Promise<T> {
  const key = cacheKey(cfg, query);
  const hit = queryCache.get(key);
  if (hit && Date.now() - hit.ts < CACHE_TTL_MS) {
    return hit.data as T;
  }
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
  const data = json.data as T;
  queryCache.set(key, { data, ts: Date.now() });
  return data;
}

/** Clear all cached queries (call on wallet connect/disconnect if needed). */
export function clearQueryCache(): void {
  queryCache.clear();
}

const EVM_SELECTORS = {
  resolveRepo: "2c1ce86c",
  listReposPage: "502a5e93",
  listRefsPage: "084871ef",
  listCollaboratorsPage: "1758c17d",
  resolveRef: "a9104618",
  updateRepoInfo: "1a532166",
  beginOwnershipTransfer: "27abb3e4",
  cancelOwnershipTransfer: "99d02f6d",
  rejectOwnershipTransfer: "e704076c",
  expireOwnershipTransfer: "5cfae861",
  acceptOwnership: "1b4f6c46",
  pendingOwnershipTransfer: "01a17250",
  getRepoById: "3334edd1",
  awardBadge: "3ad71059",
  listBadgesByRecipientPage: "4a7d9d07",
  listBadgesByRepoPage: "44d938a8",
  revenueSplits: "d223931b",
  sponsorTotal: "60d02143",
  sponsor: "5b480bac",
  setRevenueSplits: "8c15cea8",
  submitModerationReport: "b97680d7",
  setModerationStatus: "b2f78354",
  resolveModerationReport: "feae5d48",
  appealModerationReport: "f31f6183",
  resolveModerationAppeal: "9f443ccc",
  getModerationReport: "4e7f9b19",
} as const;

const EVM_QUERY_PAGE_SIZE = 64;

const EVM_RECEIPT_POLL_ATTEMPTS = 120;
const EVM_RECEIPT_POLL_INTERVAL_MS = 1_000;

const EVM_REPO_NOT_FOUND_SELECTOR = "f6ca4ec1";
const EVM_LOCATOR_NOT_FOUND_SELECTOR = "6de3a11d";
const EVM_REPO_MOVED_SELECTOR = "58629d7f";
const EVM_REPO_NOT_ACTIVE_SELECTOR = "3dc0967d";
const EVM_INVALID_RECIPIENT_SELECTOR = "17858bbe";
const EVM_INVALID_REASON_LENGTH_SELECTOR = "fc91889b";
const EVM_BADGE_NOT_FOUND_SELECTOR = "dc29602f";
const EVM_INVALID_PAGE_SIZE_SELECTOR = "dc78128f";
const EVM_INVALID_CURSOR_SELECTOR = "2165c8f6";
const EVM_INVALID_REGISTRY_SELECTOR = "11a1e697";
const EVM_UNAUTHORIZED_SELECTOR = "8e4a23d6";
const EVM_TIMESTAMP_OVERFLOW_SELECTOR = "4f9022c0";
const EVM_FROZEN_REPO_SELECTOR = "c86a3cd7";
const EVM_NO_FUNDS_SELECTOR = "43f9e110";
const EVM_INVALID_MESSAGE_LENGTH_SELECTOR = "c2fdac98";
const EVM_SPLIT_LENGTH_MISMATCH_SELECTOR = "2db4fb29";
const EVM_TOO_MANY_SPLIT_RECIPIENTS_SELECTOR = "d981d6ae";
const EVM_INVALID_SPLIT_RECIPIENT_SELECTOR = "5f9a5aa6";
const EVM_DUPLICATE_SPLIT_RECIPIENT_SELECTOR = "cbb7c145";
const EVM_INVALID_SPLIT_BPS_SELECTOR = "179711f6";
const EVM_SPLIT_TOTAL_TOO_HIGH_SELECTOR = "36d34374";
const EVM_PLATFORM_FEE_TOO_HIGH_SELECTOR = "dafcfd9b";
const EVM_PAYOUT_FAILED_SELECTOR = "e3e92735";
const EVM_REENTRANT_SETTLEMENT_SELECTOR = "03358734";
const EVM_INVALID_ADMIN_SELECTOR = "b5eba9f0";
const EVM_INVALID_TREASURY_SELECTOR = "14bcf5c8";
const EVM_INVALID_MODERATION_STATUS_SELECTOR = "774f7f12";
const EVM_INVALID_REASON_HASH_LENGTH_SELECTOR = "c29d5298";
const EVM_REPORT_NOT_FOUND_SELECTOR = "6fa835ee";
const EVM_REPORT_STATE_SELECTOR = "c7c36b2e";
const EVM_APPEAL_UNAUTHORIZED_SELECTOR = "faefce2d";

export class EVMRepoNotFoundError extends Error {
  constructor(public readonly repoId: string) {
    super(`repository not found (repo ID ${repoId})`);
    this.name = "EVMRepoNotFoundError";
  }
}

export class EVMLocatorNotFoundError extends Error {
  constructor(
    public readonly owner: string,
    public readonly repo: string,
  ) {
    super(`repository locator not found: igit://${owner}/${repo}`);
    this.name = "EVMLocatorNotFoundError";
  }
}

export class EVMRepoMovedError extends Error {
  public readonly canonicalUrl: string;

  constructor(
    public readonly repoId: string,
    public readonly currentOwner: string,
    public readonly repo: string,
  ) {
    const canonicalUrl = `igit://${currentOwner}/${repo}`;
    super(`repository moved to ${canonicalUrl}`);
    this.name = "EVMRepoMovedError";
    this.canonicalUrl = canonicalUrl;
  }
}

function stripHex(value: string): string {
  return value.replace(/^0x/i, "");
}

function evmOwner(owner: string): string {
  const value = owner.trim();
  if (/^0x[0-9a-fA-F]{40}$/.test(value)) return stripHex(value).toLowerCase();
  const decoded = fromBech32(value);
  if (decoded.prefix !== "inj" || decoded.data.length !== 20) {
    throw new Error(`invalid Injective owner address: ${owner}`);
  }
  return toHex(decoded.data).padStart(40, "0").toLowerCase();
}

function abiWord(value: string): string {
  return value.padStart(64, "0");
}

function abiDynamicString(value: string): string {
  const bytes = new TextEncoder().encode(value);
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
  return abiWord(bytes.length.toString(16)) + hex.padEnd(Math.ceil(hex.length / 64) * 64, "0");
}

function abiBool(value: boolean): string {
  return abiWord(value ? "1" : "0");
}

function abiBytes32(value: string): string {
  const raw = stripHex(value);
  if (!/^[0-9a-fA-F]{64}$/.test(raw)) {
    throw new Error(`invalid EVM repo ID: ${value}`);
  }
  return raw.toLowerCase();
}

function abiUint(value: number): string {
  if (!Number.isSafeInteger(value) || value < 0) {
    throw new Error(`invalid ABI unsigned integer: ${value}`);
  }
  return abiWord(value.toString(16));
}

function encodeV2Call(selector: string, owner: string, strings: string[]): string {
  const headLength = 32 + strings.length * 32;
  let tail = "";
  const head = [abiWord(evmOwner(owner))];
  for (const value of strings) {
    head.push(abiWord((headLength + tail.length / 2).toString(16)));
    tail += abiDynamicString(value);
  }
  return `0x${selector}${head.join("")}${tail}`;
}

function encodeV2RepoIDPageCall(selector: string, repoId: string, cursor: number, limit: number): string {
  const rawRepoId = stripHex(repoId);
  if (!/^[0-9a-fA-F]{64}$/.test(rawRepoId)) {
    throw new Error(`invalid EVM repo ID: ${repoId}`);
  }
  return `0x${selector}${rawRepoId.toLowerCase()}${abiUint(cursor)}${abiUint(limit)}`;
}

function encodeV2OwnerPageCall(selector: string, owner: string, cursor: number, limit: number): string {
  return `0x${selector}${abiWord(evmOwner(owner))}${abiUint(cursor)}${abiUint(limit)}`;
}

function encodeUpdateRepoInfoCall(
  repo: string,
  description: string | undefined,
  defaultBranch: string | undefined,
): string {
  const repoValue = abiDynamicString(repo);
  const descriptionValue = abiDynamicString(description ?? "");
  const defaultBranchValue = abiDynamicString(defaultBranch ?? "");
  const headLength = 5 * 32;
  const descriptionOffset = headLength + repoValue.length / 2;
  const defaultBranchOffset = descriptionOffset + descriptionValue.length / 2;
  return `0x${EVM_SELECTORS.updateRepoInfo}${[
    abiWord(headLength.toString(16)),
    abiBool(description !== undefined),
    abiWord(descriptionOffset.toString(16)),
    abiBool(defaultBranch !== undefined),
    abiWord(defaultBranchOffset.toString(16)),
    repoValue,
    descriptionValue,
    defaultBranchValue,
  ].join("")}`;
}

function encodeOwnershipTransferCall(
  selector: string,
  repoId: string,
  newOwner?: string,
): string {
  const args = [abiBytes32(repoId)];
  if (newOwner !== undefined) args.push(abiWord(evmOwner(newOwner)));
  return `0x${selector}${args.join("")}`;
}

function encodeBadgeRecipientPageCall(recipient: string, cursor: number, limit: number): string {
  return `0x${EVM_SELECTORS.listBadgesByRecipientPage}${abiWord(evmOwner(recipient))}${abiUint(cursor)}${abiUint(limit)}`;
}

function encodeAwardBadgeCall(repoId: string, recipient: string, reason: string): string {
  const reasonTail = abiDynamicString(reason);
  return `0x${EVM_SELECTORS.awardBadge}${[
    abiBytes32(repoId),
    abiWord(evmOwner(recipient)),
    abiWord((3 * 32).toString(16)),
    reasonTail,
  ].join("")}`;
}

function encodeSponsorCall(repoId: string, message: string): string {
  return `0x${EVM_SELECTORS.sponsor}${[
    abiBytes32(repoId),
    abiWord((2 * 32).toString(16)),
    abiDynamicString(message),
  ].join("")}`;
}

function encodeAddressArray(values: string[]): string {
  return abiWord(values.length.toString(16)) + values
    .map((value) => abiWord(evmOwner(value)))
    .join("");
}

function encodeUint16Array(values: number[]): string {
  return abiWord(values.length.toString(16)) + values.map((value) => {
    if (!Number.isInteger(value) || value <= 0 || value > 10_000) {
      throw new Error(`invalid revenue split bps ${value}`);
    }
    return abiWord(value.toString(16));
  }).join("");
}

function encodeSetRevenueSplitsCall(repoId: string, splits: SplitEntry[]): string {
  if (splits.length > 20) throw new Error("revenue splits support at most 20 recipients");
  const normalized = splits.map((split) => ({ address: evmOwner(split.address), bps: split.bps }));
  const unique = new Set(normalized.map((split) => split.address));
  if (unique.size !== normalized.length) throw new Error("revenue split recipients must be unique");
  const total = normalized.reduce((sum, split) => sum + split.bps, 0);
  if (total > 10_000) throw new Error(`revenue split total ${total} bps exceeds 10000`);
  const recipients = encodeAddressArray(normalized.map((split) => `0x${split.address}`));
  const basisPoints = encodeUint16Array(normalized.map((split) => split.bps));
  const headBytes = 3 * 32;
  return `0x${EVM_SELECTORS.setRevenueSplits}${[
    abiBytes32(repoId),
    abiWord(headBytes.toString(16)),
    abiWord((headBytes + recipients.length / 2).toString(16)),
    recipients,
    basisPoints,
  ].join("")}`;
}

export type EVMModerationReportID = string | number | bigint;
export type EVMModerationStatus = RepoInfo["moderation_status"];

function abiModerationReportID(value: EVMModerationReportID): string {
  let id: bigint;
  if (typeof value === "bigint") {
    id = value;
  } else if (typeof value === "number") {
    if (!Number.isSafeInteger(value)) {
      throw new Error(`invalid EVM V2 moderation report ID: ${value}`);
    }
    id = BigInt(value);
  } else {
    if (!/^(?:0|[1-9][0-9]*)$/.test(value)) {
      throw new Error(`invalid EVM V2 moderation report ID: ${value}`);
    }
    id = BigInt(value);
  }
  if (id <= 0n || id >= 1n << 256n) {
    throw new Error(`invalid EVM V2 moderation report ID: ${value}`);
  }
  return abiWord(id.toString(16));
}

function abiModerationStatus(status: EVMModerationStatus): string {
  const encoded = ({ active: 0, frozen: 1, delisted: 2 } as const)[status];
  if (encoded === undefined) throw new Error(`invalid EVM V2 moderation status: ${status}`);
  return abiWord(encoded.toString(16));
}

function validateModerationReasonHash(reasonHash: string, optional: boolean): string {
  const length = new TextEncoder().encode(reasonHash).length;
  if ((!optional && length === 0) || length > 128) {
    const range = optional ? "at most 128" : "1 to 128";
    throw new Error(`moderation reason hash must contain ${range} UTF-8 bytes`);
  }
  // A reason hash is an opaque protocol value. Preserve it byte-for-byte.
  return reasonHash;
}

function encodeModerationRepoReasonCall(
  selector: string,
  repoId: string,
  reasonHash: string,
  optionalReason = false,
): string {
  const reason = abiDynamicString(validateModerationReasonHash(reasonHash, optionalReason));
  return `0x${selector}${[
    abiBytes32(repoId),
    abiWord((2 * 32).toString(16)),
    reason,
  ].join("")}`;
}

function encodeModerationRepoStatusCall(
  repoId: string,
  status: EVMModerationStatus,
  reasonHash: string,
): string {
  const reason = abiDynamicString(validateModerationReasonHash(reasonHash, true));
  return `0x${EVM_SELECTORS.setModerationStatus}${[
    abiBytes32(repoId),
    abiModerationStatus(status),
    abiWord((3 * 32).toString(16)),
    reason,
  ].join("")}`;
}

function encodeModerationReportReasonCall(
  selector: string,
  reportId: EVMModerationReportID,
  reasonHash: string,
): string {
  const reason = abiDynamicString(validateModerationReasonHash(reasonHash, false));
  return `0x${selector}${[
    abiModerationReportID(reportId),
    abiWord((2 * 32).toString(16)),
    reason,
  ].join("")}`;
}

function encodeModerationReportStatusCall(
  selector: string,
  reportId: EVMModerationReportID,
  status: EVMModerationStatus,
  reasonHash: string,
): string {
  const reason = abiDynamicString(validateModerationReasonHash(reasonHash, false));
  return `0x${selector}${[
    abiModerationReportID(reportId),
    abiModerationStatus(status),
    abiWord((3 * 32).toString(16)),
    reason,
  ].join("")}`;
}

function hexWord(data: string, offset: number): string {
  const value = stripHex(data);
  const start = offset * 2;
  const word = value.slice(start, start + 64);
  if (word.length !== 64) throw new Error("malformed EVM ABI response");
  return word;
}

function uintWord(data: string, offset: number): number {
  const value = Number.parseInt(hexWord(data, offset), 16);
  if (!Number.isSafeInteger(value)) throw new Error("EVM ABI integer exceeds JavaScript precision");
  return value;
}

function bigUintWord(data: string, offset: number): bigint {
  return BigInt(`0x${hexWord(data, offset)}`);
}

function dynamicOffset(data: string, wordOffset: number, base: number): number {
  const offset = uintWord(data, wordOffset);
  const absolute = base + offset;
  if (absolute < 0 || absolute > stripHex(data).length / 2) throw new Error("malformed EVM ABI offset");
  return absolute;
}

function decodeString(data: string, offset: number): string {
  const length = uintWord(data, offset);
  const raw = stripHex(data).slice((offset + 32) * 2, (offset + 32 + length) * 2);
  if (raw.length !== length * 2) throw new Error("malformed EVM ABI string");
  const bytes = new Uint8Array(raw.match(/../g)?.map((part) => Number.parseInt(part, 16)) ?? []);
  return new TextDecoder().decode(bytes);
}

function decodeAddress(data: string, offset: number): string {
  const raw = hexWord(data, offset).slice(24);
  const bytes = new Uint8Array(raw.match(/../g)?.map((part) => Number.parseInt(part, 16)) ?? []);
  return toBech32("inj", bytes);
}

function decodePendingOwnershipTransfer(data: string): PendingOwnershipTransfer | null {
  const raw = stripHex(data);
  if (raw.length < 128 * 2) throw new Error("malformed EVM ownership transfer response");
  const newOwnerWord = hexWord(data, 0);
  if (/^0+$/.test(newOwnerWord)) return null;
  return {
    newOwner: decodeAddress(data, 0),
    proposedAt: uintWord(data, 32),
    executeAfter: uintWord(data, 64),
    expiresAt: uintWord(data, 96),
  };
}

function decodeStringArray(data: string, offset: number): string[] {
  const count = uintWord(data, offset);
  const values: string[] = [];
  for (let i = 0; i < count; i += 1) {
    const item = dynamicOffset(data, offset + 32 + i * 32, offset + 32);
    values.push(decodeString(data, item));
  }
  return values;
}

function decodeAddressArray(data: string, offset: number): string[] {
  const count = uintWord(data, offset);
  const values: string[] = [];
  for (let i = 0; i < count; i += 1) values.push(decodeAddress(data, offset + 32 + i * 32));
  return values;
}

function decodeUintArray(data: string, offset: number): number[] {
  const count = uintWord(data, offset);
  const values: number[] = [];
  for (let i = 0; i < count; i += 1) values.push(uintWord(data, offset + 32 + i * 32));
  return values;
}

function decodeV2RevenueSplits(data: string): SplitEntry[] {
  const valuesBase = dynamicOffset(data, 0, 0);
  const count = uintWord(data, valuesBase);
  const values: SplitEntry[] = [];
  for (let i = 0; i < count; i += 1) {
    const tuple = valuesBase + 32 + i * 64;
    const bps = uintWord(data, tuple + 32);
    if (bps <= 0 || bps > 10_000) {
      throw new Error(`EVM economic module returned invalid split bps ${bps}`);
    }
    values.push({ address: decodeAddress(data, tuple), bps });
  }
  return values;
}

export interface EVMModerationReport {
  /** Report IDs are local to the configured V2 moderation module. */
  id: string;
  repoId: string;
  ownerAtReport: string;
  repoNameAtReport: string;
  reporter: string;
  reasonHash: string;
  state: "open" | "resolved" | "appealed" | "appeal_resolved";
  resolution: EVMModerationStatus;
  hasResolution: boolean;
  resolutionHash: string;
  appealHash: string;
  hasAppeal: boolean;
  createdAt: number;
  updatedAt: number;
  /** Module decisions do not yet enforce core registry ref writes. */
  enforcement: "advisory";
}

function decodeV2ModerationReport(data: string): EVMModerationReport {
  // getReport returns one dynamic tuple, so word zero points to its head.
  const base = dynamicOffset(data, 0, 0);
  const id = bigUintWord(data, base);
  if (id === 0n) throw new Error("EVM V2 moderation module returned a zero report ID");
  const state = uintWord(data, base + 192);
  if (state > 3) throw new Error(`EVM V2 moderation module returned invalid report state ${state}`);
  const resolution = uintWord(data, base + 224);
  if (resolution > 2) {
    throw new Error(`EVM V2 moderation module returned invalid resolution status ${resolution}`);
  }
  return {
    id: id.toString(),
    repoId: `0x${hexWord(data, base + 32)}`,
    ownerAtReport: decodeAddress(data, base + 64),
    repoNameAtReport: decodeString(data, dynamicOffset(data, base + 96, base)),
    reporter: decodeAddress(data, base + 128),
    reasonHash: decodeString(data, dynamicOffset(data, base + 160, base)),
    state: (["open", "resolved", "appealed", "appeal_resolved"] as const)[state],
    resolution: (["active", "frozen", "delisted"] as const)[resolution],
    hasResolution: decodeBoolWord(data, base + 256, "moderation report resolution flag"),
    resolutionHash: decodeString(data, dynamicOffset(data, base + 288, base)),
    appealHash: decodeString(data, dynamicOffset(data, base + 320, base)),
    hasAppeal: decodeBoolWord(data, base + 352, "moderation report appeal flag"),
    createdAt: uintWord(data, base + 384),
    updatedAt: uintWord(data, base + 416),
    enforcement: "advisory",
  };
}

function decodeBytes32Array(data: string, offset: number): string[] {
  const count = uintWord(data, offset);
  const values: string[] = [];
  for (let i = 0; i < count; i += 1) values.push(`0x${hexWord(data, offset + 32 + i * 32)}`);
  return values;
}

function decodeV2RepoAt(data: string, base: number): RepoInfo {
  const owner = decodeAddress(data, base);
  const name = decodeString(data, dynamicOffset(data, base + 32, base));
  const description = decodeString(data, dynamicOffset(data, base + 64, base));
  const defaultBranch = decodeString(data, dynamicOffset(data, base + 96, base));
  const moderationStatus = uintWord(data, base + 192);
  if (moderationStatus > 2) {
    throw new Error(`EVM contract returned invalid moderation status ${moderationStatus}`);
  }
  const exists = hexWord(data, base + 224).endsWith("1");
  if (!exists) throw new Error("EVM contract returned a non-existent repository");
  return {
    owner,
    name,
    description,
    default_branch: defaultBranch,
    created_at: uintWord(data, base + 128),
    updated_at: uintWord(data, base + 160),
    moderation_status: (["active", "frozen", "delisted"] as const)[moderationStatus],
    forked_from: null,
  };
}

function decodeV2ResolvedRepo(data: string, requestedOwner: string, requestedName: string): ResolvedRepo {
  const canonicalWord = hexWord(data, 32);
  if (!/^0{63}[01]$/.test(canonicalWord)) {
    throw new Error("EVM resolveRepo returned an invalid canonical flag");
  }
  const info = decodeV2RepoAt(data, dynamicOffset(data, 64, 0));
  return {
    repoId: `0x${hexWord(data, 0)}`,
    backend: "evm",
    requested: { owner: requestedOwner, name: requestedName },
    canonical: { owner: info.owner, name: info.name },
    isCanonical: canonicalWord.endsWith("1"),
    info,
  };
}

interface EVMRepoPage {
  nextCursor: number;
  hasMore: boolean;
  repositories: RepoInfo[];
}

function decodeV2RepoPage(data: string): EVMRepoPage {
  const nextCursor = uintWord(data, 0);
  const hasMore = decodeBoolWord(data, 32, "repository page");
  const repoIds = decodeBytes32Array(data, dynamicOffset(data, 64, 0));
  const repositoriesBase = dynamicOffset(data, 96, 0);
  const count = uintWord(data, repositoriesBase);
  if (count !== repoIds.length) {
    throw new Error(`EVM repository page returned ${repoIds.length} IDs and ${count} repositories`);
  }
  const repositories: RepoInfo[] = [];
  for (let i = 0; i < count; i += 1) {
    const base = dynamicOffset(data, repositoriesBase + 32 + i * 32, repositoriesBase + 32);
    repositories.push(decodeV2RepoAt(data, base));
  }
  return { nextCursor, hasMore, repositories };
}

function decodeV2Ref(data: string, base: number): RefInfo {
  const commitSha = decodeString(data, dynamicOffset(data, base, base));
  const packUris = decodeStringArray(data, dynamicOffset(data, base + 32, base));
  const exists = hexWord(data, base + 128).endsWith("1");
  if (!exists) throw new Error("EVM contract returned a non-existent ref");
  return {
    ref_name: "",
    commit_sha: commitSha,
    pack_uris: packUris,
    updated_at: uintWord(data, base + 64),
    updated_by: decodeAddress(data, base + 96),
  };
}

function decodeBoolWord(data: string, offset: number, label: string): boolean {
  const word = hexWord(data, offset);
  if (!/^0{63}[01]$/.test(word)) throw new Error(`EVM ${label} returned an invalid boolean`);
  return word.endsWith("1");
}

interface EVMRefPage {
  nextCursor: number;
  hasMore: boolean;
  refs: RefInfo[];
}

function decodeV2RefPage(data: string): EVMRefPage {
  const nextCursor = uintWord(data, 0);
  const hasMore = decodeBoolWord(data, 32, "ref page");
  const names = decodeStringArray(data, dynamicOffset(data, 64, 0));
  const valuesBase = dynamicOffset(data, 96, 0);
  const count = uintWord(data, valuesBase);
  if (count !== names.length) throw new Error("EVM ref page returned mismatched names and values");
  const refs: RefInfo[] = [];
  for (let i = 0; i < count; i += 1) {
    const base = dynamicOffset(data, valuesBase + 32 + i * 32, valuesBase + 32);
    refs.push({ ...decodeV2Ref(data, base), ref_name: names[i] });
  }
  return { nextCursor, hasMore, refs };
}

interface EVMCollaboratorPage {
  nextCursor: number;
  hasMore: boolean;
  collaborators: CollaboratorInfo[];
}

function decodeV2CollaboratorPage(data: string): EVMCollaboratorPage {
  const nextCursor = uintWord(data, 0);
  const hasMore = decodeBoolWord(data, 32, "collaborator page");
  const addresses = decodeAddressArray(data, dynamicOffset(data, 64, 0));
  const roles = decodeUintArray(data, dynamicOffset(data, 96, 0));
  if (addresses.length !== roles.length) {
    throw new Error(`EVM collaborator page returned ${addresses.length} addresses and ${roles.length} roles`);
  }
  return {
    nextCursor,
    hasMore,
    collaborators: addresses.map((address, index) => {
      const role = roles[index];
      if (role === 1) return { address, role: "maintainer" };
      if (role === 2) return { address, role: "reader" };
      throw new Error(`EVM collaborator page returned invalid role ${role}`);
    }),
  };
}

interface EVMBadgeRecord {
  id: number;
  repoId: string;
  recipient: string;
  reason: string;
  awardedBy: string;
  awardedAt: number;
}

interface EVMBadgePage {
  nextCursor: number;
  hasMore: boolean;
  badges: EVMBadgeRecord[];
}

function decodeV2BadgeAt(data: string, base: number): EVMBadgeRecord {
  const id = uintWord(data, base);
  if (id === 0) throw new Error("EVM badge module returned a zero badge ID");
  return {
    id,
    repoId: `0x${hexWord(data, base + 32)}`,
    recipient: decodeAddress(data, base + 64),
    reason: decodeString(data, dynamicOffset(data, base + 96, base)),
    awardedBy: decodeAddress(data, base + 128),
    awardedAt: uintWord(data, base + 160),
  };
}

function decodeV2BadgePage(data: string): EVMBadgePage {
  const nextCursor = uintWord(data, 0);
  const hasMore = decodeBoolWord(data, 32, "badge page");
  const valuesBase = dynamicOffset(data, 64, 0);
  const count = uintWord(data, valuesBase);
  const badges: EVMBadgeRecord[] = [];
  for (let i = 0; i < count; i += 1) {
    const base = dynamicOffset(data, valuesBase + 32 + i * 32, valuesBase + 32);
    badges.push(decodeV2BadgeAt(data, base));
  }
  return { nextCursor, hasMore, badges };
}

function decodeV2ResolveRef(data: string, refName: string): RefInfo {
  return { ...decodeV2Ref(data, dynamicOffset(data, 0, 0)), ref_name: refName };
}

function decodeEVMRevert(data: string): Error | undefined {
  const raw = stripHex(data).toLowerCase();
  if (raw.length < 8) return undefined;
  const selector = raw.slice(0, 8);
  const payload = raw.slice(8);
  if (selector === EVM_REPO_NOT_FOUND_SELECTOR) {
    return new EVMRepoNotFoundError(`0x${hexWord(payload, 0)}`);
  }
  if (selector === EVM_LOCATOR_NOT_FOUND_SELECTOR) {
    const owner = decodeAddress(payload, 0);
    const repo = decodeString(payload, dynamicOffset(payload, 32, 0));
    return new EVMLocatorNotFoundError(owner, repo);
  }
  if (selector === EVM_REPO_MOVED_SELECTOR) {
    const repoId = `0x${hexWord(payload, 0)}`;
    const currentOwner = decodeAddress(payload, 32);
    const repo = decodeString(payload, dynamicOffset(payload, 64, 0));
    return new EVMRepoMovedError(repoId, currentOwner, repo);
  }
  if (selector === EVM_REPO_NOT_ACTIVE_SELECTOR) {
    const repoId = `0x${hexWord(payload, 0)}`;
    const status = uintWord(payload, 32);
    const statusName = (["active", "frozen", "delisted"] as const)[status] ?? `unknown (${status})`;
    return new Error(`repository is not active: ${statusName} (repo ID ${repoId})`);
  }
  if (selector === EVM_INVALID_RECIPIENT_SELECTOR) {
    return new Error(`invalid badge recipient: ${decodeAddress(payload, 0)}`);
  }
  if (selector === EVM_INVALID_REASON_LENGTH_SELECTOR) {
    const length = bigUintWord(payload, 0);
    const maximum = bigUintWord(payload, 32);
    return new Error(`invalid badge reason length ${length}; maximum ${maximum} UTF-8 bytes`);
  }
  if (selector === EVM_BADGE_NOT_FOUND_SELECTOR) {
    return new Error(`badge not found: ${bigUintWord(payload, 0)}`);
  }
  if (selector === EVM_INVALID_PAGE_SIZE_SELECTOR) {
    const requested = bigUintWord(payload, 0);
    const maximum = bigUintWord(payload, 32);
    return new Error(`invalid EVM page size ${requested}; maximum ${maximum}`);
  }
  if (selector === EVM_INVALID_CURSOR_SELECTOR) {
    const cursor = bigUintWord(payload, 0);
    const total = bigUintWord(payload, 32);
    return new Error(`invalid EVM page cursor ${cursor}; total ${total}`);
  }
  if (selector === EVM_INVALID_REGISTRY_SELECTOR) {
    return new Error("EVM module registry configuration is invalid");
  }
  if (selector === EVM_UNAUTHORIZED_SELECTOR) {
    return new Error(`EVM contract caller is unauthorized: ${decodeAddress(payload, 0)}`);
  }
  if (selector === EVM_TIMESTAMP_OVERFLOW_SELECTOR) {
    return new Error(`EVM timestamp exceeds the contract range: ${bigUintWord(payload, 0)}`);
  }
  if (selector === EVM_FROZEN_REPO_SELECTOR) {
    return new Error(`repository is frozen (repo ID 0x${hexWord(payload, 0)})`);
  }
  if (selector === EVM_NO_FUNDS_SELECTOR) {
    return new Error("sponsorship amount must be greater than zero");
  }
  if (selector === EVM_INVALID_MESSAGE_LENGTH_SELECTOR) {
    return new Error(
      `invalid sponsor message length ${bigUintWord(payload, 0)}; ` +
      `maximum ${bigUintWord(payload, 32)} UTF-8 bytes`,
    );
  }
  if (selector === EVM_SPLIT_LENGTH_MISMATCH_SELECTOR) {
    return new Error(
      `revenue split length mismatch: ${bigUintWord(payload, 0)} recipients and ` +
      `${bigUintWord(payload, 32)} basis-point values`,
    );
  }
  if (selector === EVM_TOO_MANY_SPLIT_RECIPIENTS_SELECTOR) {
    return new Error(
      `too many revenue split recipients: ${bigUintWord(payload, 0)}; ` +
      `maximum ${bigUintWord(payload, 32)}`,
    );
  }
  if (selector === EVM_INVALID_SPLIT_RECIPIENT_SELECTOR) {
    return new Error(`invalid revenue split recipient: ${decodeAddress(payload, 0)}`);
  }
  if (selector === EVM_DUPLICATE_SPLIT_RECIPIENT_SELECTOR) {
    return new Error(`duplicate revenue split recipient: ${decodeAddress(payload, 0)}`);
  }
  if (selector === EVM_INVALID_SPLIT_BPS_SELECTOR) {
    return new Error(
      `invalid revenue split for ${decodeAddress(payload, 0)}: ${bigUintWord(payload, 32)} bps`,
    );
  }
  if (selector === EVM_SPLIT_TOTAL_TOO_HIGH_SELECTOR) {
    return new Error(
      `revenue split total ${bigUintWord(payload, 0)} bps exceeds ` +
      `${bigUintWord(payload, 32)} bps`,
    );
  }
  if (selector === EVM_PLATFORM_FEE_TOO_HIGH_SELECTOR) {
    return new Error(
      `platform fee ${bigUintWord(payload, 0)} bps exceeds ${bigUintWord(payload, 32)} bps`,
    );
  }
  if (selector === EVM_PAYOUT_FAILED_SELECTOR) {
    return new Error(
      `economic payout to ${decodeAddress(payload, 0)} failed for ` +
      `${bigUintWord(payload, 32)} base-unit INJ`,
    );
  }
  if (selector === EVM_REENTRANT_SETTLEMENT_SELECTOR) {
    return new Error("economic settlement re-entry was rejected");
  }
  if (selector === EVM_INVALID_ADMIN_SELECTOR) {
    return new Error("EVM economic module administrator is invalid");
  }
  if (selector === EVM_INVALID_TREASURY_SELECTOR) {
    return new Error("EVM economic module treasury is invalid");
  }
  if (selector === EVM_INVALID_MODERATION_STATUS_SELECTOR) {
    return new Error(`invalid EVM V2 moderation status: ${bigUintWord(payload, 0)}`);
  }
  if (selector === EVM_INVALID_REASON_HASH_LENGTH_SELECTOR) {
    return new Error(
      `invalid moderation reason hash length ${bigUintWord(payload, 0)}; ` +
      `maximum ${bigUintWord(payload, 32)} UTF-8 bytes`,
    );
  }
  if (selector === EVM_REPORT_NOT_FOUND_SELECTOR) {
    return new Error(`EVM V2 moderation report not found: ${bigUintWord(payload, 0)}`);
  }
  if (selector === EVM_REPORT_STATE_SELECTOR) {
    return new Error(
      `EVM V2 moderation report ${bigUintWord(payload, 0)} has state ` +
      `${bigUintWord(payload, 64)}, expected ${bigUintWord(payload, 32)}`,
    );
  }
  if (selector === EVM_APPEAL_UNAUTHORIZED_SELECTOR) {
    return new Error(
      `moderation appeal caller ${decodeAddress(payload, 0)} is not recorded owner ` +
      decodeAddress(payload, 32),
    );
  }
  return undefined;
}

async function evmCall(cfg: AppConfig, data: string, blockTag = "latest"): Promise<string> {
  if (!cfg.evmRpc || !/^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract)) {
    throw new Error("EVM V2 registry is not configured for this network profile");
  }
  const response = await fetch(cfg.evmRpc, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: Date.now(),
      method: "eth_call",
      params: [{ to: cfg.evmContract, data }, blockTag],
    }),
  });
  const body = await response.text();
  let json: { result?: string; error?: unknown } = {};
  try {
    json = JSON.parse(body) as { result?: string; error?: unknown };
  } catch {
    if (!response.ok) throw new Error(`EVM RPC failed (HTTP ${response.status})`);
    throw new Error("EVM eth_call returned malformed JSON");
  }
  if (json.error) {
    const revertData = findEVMRevertData(json.error);
    const decoded = revertData ? decodeEVMRevert(revertData) : undefined;
    if (decoded) throw decoded;
    throw new Error(formatError(json.error));
  }
  if (!response.ok) throw new Error(`EVM RPC failed (HTTP ${response.status})`);
  if (typeof json.result !== "string") {
    throw new Error("EVM eth_call returned no result");
  }
  return json.result;
}

async function evmCallTo(
  cfg: AppConfig,
  to: string,
  data: string,
  blockTag = "latest",
  label = "EVM V2 module",
): Promise<string> {
  if (!cfg.evmRpc || !/^0x[0-9a-fA-F]{40}$/.test(to)) {
    throw new Error(`${label} is not configured for this network profile`);
  }
  const response = await fetch(cfg.evmRpc, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: Date.now(),
      method: "eth_call",
      params: [{ to, data }, blockTag],
    }),
  });
  const body = await response.text();
  let json: { result?: string; error?: unknown } = {};
  try {
    json = JSON.parse(body) as { result?: string; error?: unknown };
  } catch {
    if (!response.ok) throw new Error(`EVM RPC failed (HTTP ${response.status})`);
    throw new Error("EVM eth_call returned malformed JSON");
  }
  if (json.error) {
    const revertData = findEVMRevertData(json.error);
    const decoded = revertData ? decodeEVMRevert(revertData) : undefined;
    if (decoded) throw decoded;
    throw new Error(formatError(json.error));
  }
  if (!response.ok) throw new Error(`EVM RPC failed (HTTP ${response.status})`);
  if (typeof json.result !== "string") throw new Error("EVM eth_call returned no result");
  return json.result;
}

async function evmSnapshotBlockTag(cfg: AppConfig): Promise<string> {
  if (!cfg.evmRpc || !/^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract)) {
    throw new Error("EVM V2 registry is not configured for this network profile");
  }
  const response = await fetch(cfg.evmRpc, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: Date.now(),
      method: "eth_blockNumber",
      params: [],
    }),
  });
  const body = await response.text();
  let json: { result?: unknown; error?: unknown } = {};
  try {
    json = JSON.parse(body) as { result?: unknown; error?: unknown };
  } catch {
    if (!response.ok) throw new Error(`EVM RPC failed (HTTP ${response.status})`);
    throw new Error("EVM eth_blockNumber returned malformed JSON");
  }
  if (json.error) throw new Error(formatError(json.error));
  if (!response.ok) throw new Error(`EVM RPC failed (HTTP ${response.status})`);
  if (typeof json.result !== "string" || !/^0x(?:0|[1-9a-fA-F][0-9a-fA-F]*)$/.test(json.result)) {
    throw new Error("EVM eth_blockNumber returned an invalid block quantity");
  }
  return `0x${BigInt(json.result).toString(16)}`;
}

function evmChainIdHex(chainId: number): string {
  if (!Number.isSafeInteger(chainId) || chainId <= 0) {
    throw new Error(`invalid EVM chain ID: ${chainId}`);
  }
  return `0x${chainId.toString(16)}`;
}

function rpcErrorCode(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const code = (error as { code?: unknown }).code;
  return typeof code === "number" ? code : undefined;
}

async function switchEvmChain(provider: Eip1193, cfg: AppConfig): Promise<void> {
  const chainId = evmChainIdHex(cfg.evmChainId);
  try {
    await provider.request({
      method: "wallet_switchEthereumChain",
      params: [{ chainId }],
    });
  } catch (error) {
    if (rpcErrorCode(error) !== 4902) throw error;
    const profile = networkProfile(cfg);
    await provider.request({
      method: "wallet_addEthereumChain",
      params: [{
        chainId,
        chainName: profile.label,
        nativeCurrency: { name: "Injective", symbol: "INJ", decimals: 18 },
        rpcUrls: [cfg.evmRpc],
        blockExplorerUrls: [profile.evmExplorer],
      }],
    });
    await provider.request({
      method: "wallet_switchEthereumChain",
      params: [{ chainId }],
    });
  }
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForEvmReceipt(provider: Eip1193, txHash: string): Promise<void> {
  for (let attempt = 0; attempt < EVM_RECEIPT_POLL_ATTEMPTS; attempt += 1) {
    const value = await provider.request({
      method: "eth_getTransactionReceipt",
      params: [txHash],
    });
    if (value != null) {
      if (typeof value !== "object") {
        throw new Error("EVM wallet returned a malformed transaction receipt");
      }
      const status = (value as { status?: unknown }).status;
      if (typeof status !== "string" || !/^0x[0-9a-fA-F]+$/.test(status)) {
        throw new Error("EVM transaction receipt has no valid status");
      }
      const numericStatus = BigInt(status);
      if (numericStatus === 0n) {
        throw new Error(`EVM transaction ${txHash} reverted (receipt status ${status})`);
      }
      if (numericStatus !== 1n) {
        throw new Error(`EVM transaction ${txHash} returned unexpected receipt status ${status}`);
      }
      return;
    }
    if (attempt + 1 < EVM_RECEIPT_POLL_ATTEMPTS) {
      await delay(EVM_RECEIPT_POLL_INTERVAL_MS);
    }
  }
  throw new Error(`timed out waiting for EVM transaction receipt: ${txHash}`);
}

export interface RepoInfoPatch {
  // Undefined means "do not update"; an empty string is an explicit value.
  description?: string;
  defaultBranch?: string;
}

export interface PendingOwnershipTransfer {
  newOwner: string;
  proposedAt: number;
  executeAfter: number;
  expiresAt: number;
}

export interface OwnershipTransferCapabilities {
  canCancel: boolean;
  canAccept: boolean;
  canReject: boolean;
  canExpire: boolean;
}

/**
 * Return role-based transfer actions without consulting the browser clock.
 * Maturity and expiry are enforced against block.timestamp by the contract;
 * a local clock must never hide an otherwise valid transaction.
 */
export function ownershipTransferCapabilities(
  pending: PendingOwnershipTransfer | null,
  account: string,
  currentOwner: string,
): OwnershipTransferCapabilities {
  if (!pending) {
    return { canCancel: false, canAccept: false, canReject: false, canExpire: false };
  }
  const normalizedAccount = account.toLowerCase();
  const isOwner = normalizedAccount === currentOwner.toLowerCase();
  const isTarget = normalizedAccount === pending.newOwner.toLowerCase();
  return {
    canCancel: isOwner,
    canAccept: isTarget,
    canReject: isTarget,
    canExpire: normalizedAccount.length > 0,
  };
}

/** Read the pending ownership-transfer state for a stable V2 repository ID. */
export async function pendingOwnershipTransferWithEvm(
  cfg: AppConfig,
  repoId: string,
): Promise<PendingOwnershipTransfer | null> {
  const data = await evmCall(cfg, encodeOwnershipTransferCall(EVM_SELECTORS.pendingOwnershipTransfer, repoId));
  return decodePendingOwnershipTransfer(data);
}

async function sendOwnershipTransferActionWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  data: string,
): Promise<string> {
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract)) {
    throw new Error("EVM V2 registry is not configured for this network profile");
  }
  await switchEvmChain(provider, cfg);
  const accounts = await provider.request({ method: "eth_requestAccounts" });
  const from = Array.isArray(accounts) ? accounts[0] : undefined;
  if (typeof from !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(from)) {
    throw new Error("EVM wallet returned no valid account");
  }
  let txHash: unknown;
  try {
    txHash = await provider.request({
      method: "eth_sendTransaction",
      params: [{ from, to: cfg.evmContract, data }],
    });
  } catch (error) {
    const revertData = findEVMRevertData(error);
    const decoded = revertData ? decodeEVMRevert(revertData) : undefined;
    if (decoded) throw decoded;
    throw error;
  }
  if (typeof txHash !== "string" || !/^0x[0-9a-fA-F]{64}$/.test(txHash)) {
    throw new Error("EVM wallet returned an invalid transaction hash");
  }
  await waitForEvmReceipt(provider, txHash);
  clearQueryCache();
  return txHash;
}

/** Start the delayed ownership transfer as the current repository owner. */
export function beginOwnershipTransferWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
  newOwner: string,
): Promise<string> {
  return sendOwnershipTransferActionWithEvm(
    provider,
    cfg,
    encodeOwnershipTransferCall(EVM_SELECTORS.beginOwnershipTransfer, repoId, newOwner),
  );
}

/** Cancel a pending transfer as the current repository owner. */
export function cancelOwnershipTransferWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
): Promise<string> {
  return sendOwnershipTransferActionWithEvm(
    provider,
    cfg,
    encodeOwnershipTransferCall(EVM_SELECTORS.cancelOwnershipTransfer, repoId),
  );
}

/** Reject a pending transfer as its proposed target owner. */
export function rejectOwnershipTransferWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
): Promise<string> {
  return sendOwnershipTransferActionWithEvm(
    provider,
    cfg,
    encodeOwnershipTransferCall(EVM_SELECTORS.rejectOwnershipTransfer, repoId),
  );
}

/** Permissionlessly clear a transfer after its acceptance window expires. */
export function expireOwnershipTransferWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
): Promise<string> {
  return sendOwnershipTransferActionWithEvm(
    provider,
    cfg,
    encodeOwnershipTransferCall(EVM_SELECTORS.expireOwnershipTransfer, repoId),
  );
}

/** Accept a matured transfer as its proposed target owner. */
export function acceptOwnershipWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
): Promise<string> {
  return sendOwnershipTransferActionWithEvm(
    provider,
    cfg,
    encodeOwnershipTransferCall(EVM_SELECTORS.acceptOwnership, repoId),
  );
}

/**
 * Patch repository metadata directly on the EVM V2 registry through an
 * EIP-1193 wallet. This path never falls back to or double-writes CosmWasm.
 */
export async function updateRepoInfoWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  repo: string,
  patch: RepoInfoPatch,
): Promise<string> {
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract)) {
    throw new Error("EVM V2 registry is not configured for this network profile");
  }

  await switchEvmChain(provider, cfg);
  const accounts = await provider.request({ method: "eth_requestAccounts" });
  const from = Array.isArray(accounts) ? accounts[0] : undefined;
  if (typeof from !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(from)) {
    throw new Error("EVM wallet returned no valid account");
  }

  const data = encodeUpdateRepoInfoCall(repo, patch.description, patch.defaultBranch);
  let txHash: unknown;
  try {
    txHash = await provider.request({
      method: "eth_sendTransaction",
      params: [{ from, to: cfg.evmContract, data }],
    });
  } catch (error) {
    const revertData = findEVMRevertData(error);
    const decoded = revertData ? decodeEVMRevert(revertData) : undefined;
    if (decoded) throw decoded;
    throw error;
  }
  if (typeof txHash !== "string" || !/^0x[0-9a-fA-F]{64}$/.test(txHash)) {
    throw new Error("EVM wallet returned an invalid transaction hash");
  }

  await waitForEvmReceipt(provider, txHash);
  clearQueryCache();
  return txHash;
}

function findEVMRevertData(value: unknown, depth = 0): string | undefined {
  if (depth > 5 || value == null) return undefined;
  if (typeof value === "string") {
    return /^0x[0-9a-fA-F]{8,}$/.test(value) ? value : undefined;
  }
  if (typeof value !== "object") return undefined;
  const record = value as Record<string, unknown>;
  for (const key of ["data", "result", "return", "originalError", "error"]) {
    const found = findEVMRevertData(record[key], depth + 1);
    if (found) return found;
  }
  return undefined;
}

function shouldFallbackToV1(cfg: AppConfig, error: unknown): boolean {
  // Before a profile has a reviewed V2 address, auto mode must remain usable
  // against the V1 testnet. Once V2 is configured, only an explicit
  // repository-not-found response may select the legacy read path; transport,
  // permission, and ABI errors must stay visible to the caller.
  if (!cfg.evmContract) return true;
  return error instanceof EVMLocatorNotFoundError;
}

async function readWithV1Fallback<T>(cfg: AppConfig, v2: () => Promise<T>, v1: () => Promise<T>): Promise<T> {
  if (cfg.contractVersion === "v1") return v1();
  try {
    return await v2();
  } catch (error) {
    if (cfg.contractVersion === "v2" || !shouldFallbackToV1(cfg, error)) throw error;
    return v1();
  }
}

export async function listRepos(
  cfg: AppConfig,
  owner: string,
  includeInactive = false,
): Promise<RepoInfo[]> {
  let repos: RepoInfo[];
  if (cfg.contractVersion === "v1" || (cfg.contractVersion === "auto" && !cfg.evmContract)) {
    repos = [];
    let startAfter: string | null = null;
    while (true) {
      const out: { repos: RepoInfo[] } = await smartQuery(cfg, {
        list_repos: { owner, start_after: startAfter, limit: 100 },
      });
      repos.push(...out.repos);
      if (out.repos.length < 100) break;
      const next = out.repos[out.repos.length - 1]?.name;
      if (!next || next === startAfter) {
        throw new Error(`CosmWasm repository page cursor did not advance from ${startAfter ?? "start"}`);
      }
      startAfter = next;
    }
  } else {
    repos = [];
    const blockTag = await evmSnapshotBlockTag(cfg);
    let cursor = 0;
    while (true) {
      const page = decodeV2RepoPage(await evmCall(
        cfg,
        encodeV2OwnerPageCall(EVM_SELECTORS.listReposPage, owner, cursor, EVM_QUERY_PAGE_SIZE),
        blockTag,
      ));
      repos.push(...page.repositories);
      if (!page.hasMore) break;
      if (page.nextCursor <= cursor) {
        throw new Error(`EVM repository page cursor did not advance: ${cursor}`);
      }
      cursor = page.nextCursor;
    }
  }
  return includeInactive ? repos : repos.filter((repo) => repo.moderation_status === "active");
}

export async function repoInfo(cfg: AppConfig, owner: string, repo: string): Promise<RepoInfo> {
  return (await resolveRepo(cfg, owner, repo)).info;
}

export async function resolveRepo(cfg: AppConfig, owner: string, repo: string): Promise<ResolvedRepo> {
  return resolveRepoAtBlock(cfg, owner, repo, "latest");
}

async function resolveRepoAtBlock(
  cfg: AppConfig,
  owner: string,
  repo: string,
  blockTag: string,
): Promise<ResolvedRepo> {
  return readWithV1Fallback(
    cfg,
    async () => decodeV2ResolvedRepo(
      await evmCall(cfg, encodeV2Call(EVM_SELECTORS.resolveRepo, owner, [repo]), blockTag),
      owner,
      repo,
    ),
    async () => {
      const info = await smartQuery<RepoInfo>(cfg, { repo_info: { owner, repo } });
      return {
        repoId: null,
        backend: "cosmwasm",
        requested: { owner, name: repo },
        canonical: { owner: info.owner, name: info.name },
        isCanonical: true,
        info,
      };
    },
  );
}

async function resolveRepoForEnumeration(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<{ resolved: ResolvedRepo; blockTag: string }> {
  if (cfg.contractVersion === "v1" || !cfg.evmContract) {
    return { resolved: await resolveRepo(cfg, owner, repo), blockTag: "latest" };
  }
  const blockTag = await evmSnapshotBlockTag(cfg);
  return { resolved: await resolveRepoAtBlock(cfg, owner, repo, blockTag), blockTag };
}

export async function listRefs(cfg: AppConfig, owner: string, repo: string): Promise<RefInfo[]> {
  const { resolved, blockTag } = await resolveRepoForEnumeration(cfg, owner, repo);
  if (resolved.backend === "cosmwasm") {
    const out = await smartQuery<{ refs: RefInfo[] }>(cfg, {
      list_refs: { owner: resolved.canonical.owner, repo: resolved.canonical.name, limit: 100 },
    });
    return out.refs;
  }
  if (!resolved.repoId) throw new Error("EVM resolveRepo returned no stable repository ID");

  const refs: RefInfo[] = [];
  let cursor = 0;
  while (true) {
    const page = decodeV2RefPage(await evmCall(
      cfg,
      encodeV2RepoIDPageCall(EVM_SELECTORS.listRefsPage, resolved.repoId, cursor, EVM_QUERY_PAGE_SIZE),
      blockTag,
    ));
    refs.push(...page.refs);
    if (!page.hasMore) return refs;
    if (page.nextCursor <= cursor) {
      throw new Error(`EVM ref page cursor did not advance: ${cursor}`);
    }
    cursor = page.nextCursor;
  }
}

/** Resolve one V2 ref, falling back to the CosmWasm query during migration. */
export async function resolveRef(
  cfg: AppConfig,
  owner: string,
  repo: string,
  refName: string,
): Promise<RefInfo> {
  return readWithV1Fallback(
    cfg,
    async () => ({
      ...decodeV2ResolveRef(
        await evmCall(cfg, encodeV2Call(EVM_SELECTORS.resolveRef, owner, [repo, refName])),
        refName,
      ),
      ref_name: refName,
    }),
    async () => {
      const out = await smartQuery<{ ref_name: string; commit_sha: string; pack_uris: string[] }>(cfg, {
        resolve_ref: { owner, repo, ref_name: refName },
      });
      return {
        ref_name: out.ref_name,
        commit_sha: out.commit_sha,
        pack_uris: out.pack_uris,
        updated_at: 0,
        updated_by: owner,
      };
    },
  );
}

export async function listCollaborators(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<CollaboratorInfo[]> {
  const { resolved, blockTag } = await resolveRepoForEnumeration(cfg, owner, repo);
  if (resolved.backend === "cosmwasm") {
    const out = await smartQuery<{ collaborators: CollaboratorInfo[] }>(cfg, {
      list_collaborators: {
        owner: resolved.canonical.owner,
        repo: resolved.canonical.name,
        limit: 100,
      },
    });
    return out.collaborators;
  }
  if (!resolved.repoId) throw new Error("EVM resolveRepo returned no stable repository ID");

  const collaborators: CollaboratorInfo[] = [];
  let cursor = 0;
  while (true) {
    const page = decodeV2CollaboratorPage(await evmCall(
      cfg,
      encodeV2RepoIDPageCall(
        EVM_SELECTORS.listCollaboratorsPage,
        resolved.repoId,
        cursor,
        EVM_QUERY_PAGE_SIZE,
      ),
      blockTag,
    ));
    collaborators.push(...page.collaborators);
    if (!page.hasMore) return collaborators;
    if (page.nextCursor <= cursor) {
      throw new Error(`EVM collaborator page cursor did not advance: ${cursor}`);
    }
    cursor = page.nextCursor;
  }
}

export async function revenueSplits(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<SplitEntry[]> {
  const { resolved, blockTag } = await resolveRepoForEnumeration(cfg, owner, repo);
  if (resolved.backend === "cosmwasm") {
    const out = await smartQuery<{ splits: SplitEntry[] }>(cfg, {
      revenue_splits: {
        owner: resolved.canonical.owner,
        repo: resolved.canonical.name,
      },
    });
    return out.splits;
  }
  if (!resolved.repoId) throw new Error("EVM resolveRepo returned no stable repository ID");
  return decodeV2RevenueSplits(await evmCallTo(
    cfg,
    cfg.evmEconomicModule,
    `0x${EVM_SELECTORS.revenueSplits}${abiBytes32(resolved.repoId)}`,
    blockTag,
    "EVM V2 economic module",
  ));
}

export async function sponsorTotals(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<SponsorTotal[]> {
  const { resolved, blockTag } = await resolveRepoForEnumeration(cfg, owner, repo);
  if (resolved.backend === "cosmwasm") {
    const out = await smartQuery<{ totals: SponsorTotal[] }>(cfg, {
      sponsor_totals: {
        owner: resolved.canonical.owner,
        repo: resolved.canonical.name,
      },
    });
    return out.totals;
  }
  if (!resolved.repoId) throw new Error("EVM resolveRepo returned no stable repository ID");
  const result = await evmCallTo(
    cfg,
    cfg.evmEconomicModule,
    `0x${EVM_SELECTORS.sponsorTotal}${abiBytes32(resolved.repoId)}`,
    blockTag,
    "EVM V2 economic module",
  );
  const total = bigUintWord(result, 0);
  return total === 0n ? [] : [{ denom: "inj", amount: total.toString() }];
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

/** Convert low-level LCD/contract failures into concise page-level feedback. */
export function formatResourceError(e: unknown, resource: "owner" | "repository"): string {
  const message = formatError(e);
  const lower = message.toLowerCase();
  const label = resource === "owner" ? "owner" : "repository";

  if (lower.includes("failed to fetch") || lower.includes("networkerror") || lower.includes("network error")) {
    return "Network unavailable. Check your connection and try again.";
  }
  if (
    lower.includes("not found") ||
    lower.includes("unknown username") ||
    lower.includes("no such") ||
    /http\s*(?:400|404)\b/.test(lower)
  ) {
    return `Could not find this ${label}. Check the ${resource === "owner" ? "address or username" : "owner and repository name"}.`;
  }
  if (/http\s*\d{3}\b/.test(lower)) {
    return `Unable to load this ${label} right now. Please try again.`;
  }
  return `Unable to load this ${label}. Please try again.`;
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
  repo_id?: string;
  awarded_by?: string;
  awarded_at: number;
}

export async function badgesByRecipient(cfg: AppConfig, recipient: string): Promise<BadgeInfo[]> {
  const readV1 = async () => {
    const out = await smartQuery<{ badges: BadgeInfo[] }>(cfg, {
      badges_by_recipient: { recipient, limit: 100 },
    });
    return out.badges;
  };
  if (cfg.contractVersion === "v1" || !cfg.evmContract) return readV1();
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmBadgeModule)) {
    throw new Error("EVM V2 badge module is not configured for this network profile");
  }

  const blockTag = await evmSnapshotBlockTag(cfg);
  const records = await drainV2BadgePages(
    cfg,
    (cursor) => encodeBadgeRecipientPageCall(recipient, cursor, EVM_QUERY_PAGE_SIZE),
    blockTag,
  );
  const repositories = await canonicalBadgeRepositories(cfg, records, blockTag);
  const v2 = records.map((badge) => {
    const repository = repositories.get(badge.repoId.toLowerCase());
    if (!repository) throw new Error(`EVM badge ${badge.id} repository metadata is missing`);
    return badgeInfoFromV2(badge, repository);
  });

  // Recipient enumeration has no locator whose absence can select V1. Auto
  // mode therefore merges the legacy trophy wall only after all V2 reads
  // succeed. Explicit V2 remains EVM-only; write paths never call V1.
  if (cfg.contractVersion !== "auto") return v2;
  return [...v2, ...await readV1()];
}

export async function badgesByRepo(
  cfg: AppConfig,
  owner: string,
  repo: string,
): Promise<BadgeInfo[]> {
  const { resolved, blockTag } = await resolveRepoForEnumeration(cfg, owner, repo);
  if (resolved.backend === "cosmwasm") {
    const out = await smartQuery<{ badges: BadgeInfo[] }>(cfg, {
      badges_by_repo: {
        owner: resolved.canonical.owner,
        repo: resolved.canonical.name,
        limit: 100,
      },
    });
    return out.badges;
  }
  if (!resolved.repoId) throw new Error("EVM resolveRepo returned no stable repository ID");
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmBadgeModule)) {
    throw new Error("EVM V2 badge module is not configured for this network profile");
  }
  const records = await drainV2BadgePages(
    cfg,
    (cursor) => encodeV2RepoIDPageCall(
      EVM_SELECTORS.listBadgesByRepoPage,
      resolved.repoId as string,
      cursor,
      EVM_QUERY_PAGE_SIZE,
    ),
    blockTag,
  );
  return records.map((badge) => badgeInfoFromV2(badge, resolved.info));
}

async function drainV2BadgePages(
  cfg: AppConfig,
  calldata: (cursor: number) => string,
  blockTag: string,
): Promise<EVMBadgeRecord[]> {
  const badges: EVMBadgeRecord[] = [];
  let cursor = 0;
  while (true) {
    const page = decodeV2BadgePage(await evmCallTo(
      cfg,
      cfg.evmBadgeModule,
      calldata(cursor),
      blockTag,
      "EVM V2 badge module",
    ));
    badges.push(...page.badges);
    if (!page.hasMore) return badges;
    if (page.nextCursor <= cursor) {
      throw new Error(`EVM badge page cursor did not advance: ${cursor}`);
    }
    cursor = page.nextCursor;
  }
}

async function canonicalBadgeRepositories(
  cfg: AppConfig,
  badges: EVMBadgeRecord[],
  blockTag: string,
): Promise<Map<string, RepoInfo>> {
  const uniqueIds = [...new Set(badges.map((badge) => badge.repoId.toLowerCase()))];
  const entries = await Promise.all(uniqueIds.map(async (repoId) => {
    const result = await evmCall(
      cfg,
      `0x${EVM_SELECTORS.getRepoById}${abiBytes32(repoId)}`,
      blockTag,
    );
    return [repoId, decodeV2RepoAt(result, dynamicOffset(result, 0, 0))] as const;
  }));
  return new Map(entries);
}

function badgeInfoFromV2(badge: EVMBadgeRecord, repository: RepoInfo): BadgeInfo {
  return {
    id: badge.id,
    repo_id: badge.repoId,
    repo_owner: repository.owner,
    repo_name: repository.name,
    recipient: badge.recipient,
    reason: badge.reason,
    awarded_by: badge.awardedBy,
    awarded_at: badge.awardedAt,
  };
}

/** Award one V2 contribution badge. This transaction cannot fall back to V1. */
export async function awardBadgeWithEvm(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
  recipient: string,
  reason: string,
): Promise<string> {
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract)) {
    throw new Error("EVM V2 registry is not configured for this network profile");
  }
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmBadgeModule)) {
    throw new Error("EVM V2 badge module is not configured for this network profile");
  }
  const trimmedReason = reason.trim();
  const reasonLength = new TextEncoder().encode(trimmedReason).length;
  if (reasonLength === 0 || reasonLength > 256) {
    throw new Error("badge reason must contain 1 to 256 UTF-8 bytes");
  }

  await switchEvmChain(provider, cfg);
  const accounts = await provider.request({ method: "eth_requestAccounts" });
  const from = Array.isArray(accounts) ? accounts[0] : undefined;
  if (typeof from !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(from)) {
    throw new Error("EVM wallet returned no valid account");
  }
  let txHash: unknown;
  try {
    txHash = await provider.request({
      method: "eth_sendTransaction",
      params: [{
        from,
        to: cfg.evmBadgeModule,
        data: encodeAwardBadgeCall(repoId, recipient, trimmedReason),
      }],
    });
  } catch (error) {
    const revertData = findEVMRevertData(error);
    const decoded = revertData ? decodeEVMRevert(revertData) : undefined;
    if (decoded) throw decoded;
    throw error;
  }
  if (typeof txHash !== "string" || !/^0x[0-9a-fA-F]{64}$/.test(txHash)) {
    throw new Error("EVM wallet returned an invalid transaction hash");
  }
  await waitForEvmReceipt(provider, txHash);
  clearQueryCache();
  return txHash;
}

function parseInjAmountToWei(amount: string): bigint {
  const value = amount.trim();
  const match = /^(?:0|[1-9]\d*)(?:\.(\d{1,18}))?$/.exec(value);
  if (!match) {
    throw new Error("INJ amount must be a positive decimal with at most 18 fractional digits");
  }
  const [whole] = value.split(".");
  const fraction = (match[1] ?? "").padEnd(18, "0");
  const wei = BigInt(whole) * 10n ** 18n + BigInt(fraction || "0");
  if (wei <= 0n) throw new Error("sponsorship amount must be greater than zero");
  return wei;
}

/** Sponsor one V2 repository through its economic module. This never writes V1. */
export async function sponsorWithEconomicModule(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
  amountInj: string,
  message: string,
): Promise<string> {
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract)) {
    throw new Error("EVM V2 registry is not configured for this network profile");
  }
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmEconomicModule)) {
    throw new Error("EVM V2 economic module is not configured for this network profile");
  }
  abiBytes32(repoId);
  const trimmedMessage = message.trim();
  const messageLength = new TextEncoder().encode(trimmedMessage).length;
  if (messageLength > 256) {
    throw new Error("sponsor message must contain at most 256 UTF-8 bytes");
  }
  const value = parseInjAmountToWei(amountInj);

  await switchEvmChain(provider, cfg);
  const accounts = await provider.request({ method: "eth_requestAccounts" });
  const from = Array.isArray(accounts) ? accounts[0] : undefined;
  if (typeof from !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(from)) {
    throw new Error("EVM wallet returned no valid account");
  }
  let txHash: unknown;
  try {
    txHash = await provider.request({
      method: "eth_sendTransaction",
      params: [{
        from,
        to: cfg.evmEconomicModule,
        data: encodeSponsorCall(repoId, trimmedMessage),
        value: `0x${value.toString(16)}`,
      }],
    });
  } catch (error) {
    const revertData = findEVMRevertData(error);
    const decoded = revertData ? decodeEVMRevert(revertData) : undefined;
    if (decoded) throw decoded;
    throw error;
  }
  if (typeof txHash !== "string" || !/^0x[0-9a-fA-F]{64}$/.test(txHash)) {
    throw new Error("EVM wallet returned an invalid transaction hash");
  }
  await waitForEvmReceipt(provider, txHash);
  clearQueryCache();
  return txHash;
}

/** Replace one V2 repository's revenue splits. This transaction cannot fall back to V1. */
export async function setRevenueSplitsWithEconomicModule(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: string,
  splits: SplitEntry[],
): Promise<string> {
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmContract)) {
    throw new Error("EVM V2 registry is not configured for this network profile");
  }
  if (!/^0x[0-9a-fA-F]{40}$/.test(cfg.evmEconomicModule)) {
    throw new Error("EVM V2 economic module is not configured for this network profile");
  }
  const data = encodeSetRevenueSplitsCall(repoId, splits);

  await switchEvmChain(provider, cfg);
  const accounts = await provider.request({ method: "eth_requestAccounts" });
  const from = Array.isArray(accounts) ? accounts[0] : undefined;
  if (typeof from !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(from)) {
    throw new Error("EVM wallet returned no valid account");
  }
  let txHash: unknown;
  try {
    txHash = await provider.request({
      method: "eth_sendTransaction",
      params: [{
        from,
        to: cfg.evmEconomicModule,
        data,
      }],
    });
  } catch (error) {
    const revertData = findEVMRevertData(error);
    const decoded = revertData ? decodeEVMRevert(revertData) : undefined;
    if (decoded) throw decoded;
    throw error;
  }
  if (typeof txHash !== "string" || !/^0x[0-9a-fA-F]{64}$/.test(txHash)) {
    throw new Error("EVM wallet returned an invalid transaction hash");
  }
  await waitForEvmReceipt(provider, txHash);
  clearQueryCache();
  return txHash;
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

/** Every tx that touched the repo-registry contract, newest first.
 *  Pass `sender` to scope the feed to a single address (personal view). */
export async function contractActivity(
  cfg: AppConfig,
  limit = 50,
  sender?: string,
): Promise<ContractTx[]> {
  const conds = [`wasm._contract_address='${cfg.contract}'`];
  if (sender) conds.push(`message.sender='${sender}'`);
  const query = conds.join(" AND ");
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
