import { zeroAddress, type Address, type Hex } from "viem";
import { toEvmAddress, toInjectiveAddress } from "./address";
import {
  clearQueryCache,
  listCollaborators,
  repoInfoById,
  resolveRepo,
  type ModerationStatus,
} from "./registry";
import { nativeBalance, readModule, verifySuite, writeModule, type Eip1193 } from "./transport";
import type { AppConfig } from "./profile";

const PAGE_SIZE = 64n;
const MAX_REVENUE_SPLIT_RECIPIENTS = 20;

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
  moderation_committee: string;
  treasury: string;
  platform_fee_bps: number;
  suite_version: number;
  snapshot_root: Hex;
}

export interface BadgeInfo {
  id: number;
  repo_owner: string;
  repo_name: string;
  recipient: string;
  reason: string;
  repo_id: Hex;
  awarded_by: string;
  awarded_at: number;
}

interface RawBadge {
  id: bigint;
  repoId: Hex;
  recipient: Address;
  awardedBy: Address;
  reason: string;
  awardedAt: bigint;
  exists: boolean;
}

export type EVMModerationReportID = string | number | bigint;
export type EVMModerationStatus = ModerationStatus;

export interface EVMModerationReport {
  id: string;
  repoId: Hex;
  reporter: string;
  state: "open" | "resolved" | "appealed" | "appeal_resolved";
  resolution: EVMModerationStatus;
  reasonHash: string;
  createdAt: number;
  updatedAt: number;
}

export interface GuardianConfig {
  configuredBy: string;
  threshold: number;
  guardians: string[];
}

export interface RecoveryProposal {
  proposedBy: string;
  newOwner: string;
  executeAfter: number;
  expiresAt: number;
  nonce: string;
  approvals: number;
}

export interface ReleaseArtifact {
  version: string;
  platform: string;
  sha256: Hex;
  registeredBy: string;
  registeredAt: number;
}

function safeNumber(value: bigint, label: string): number {
  const number = Number(value);
  if (!Number.isSafeInteger(number)) throw new Error(`${label} exceeds the JavaScript safe integer range`);
  return number;
}

function moderationStatus(value: number | bigint): EVMModerationStatus {
  const status = (["active", "frozen", "delisted"] as const)[Number(value)];
  if (!status) throw new Error(`invalid moderation status ${String(value)}`);
  return status;
}

function moderationStatusValue(value: EVMModerationStatus): number {
  const status = { active: 0, frozen: 1, delisted: 2 }[value];
  if (status === undefined) throw new Error(`invalid moderation status ${String(value)}`);
  return status;
}

function reportId(value: EVMModerationReportID): bigint {
  try {
    const id = BigInt(value);
    if (id <= 0n) throw new Error();
    return id;
  } catch {
    throw new Error(`invalid moderation report ID: ${String(value)}`);
  }
}

function parseInjAmount(amount: string): bigint {
  const match = /^(?:0|[1-9]\d*)(?:\.(\d{1,18}))?$/.exec(amount.trim());
  if (!match) throw new Error("INJ amount must be a positive decimal with at most 18 fractional digits");
  const [whole] = amount.trim().split(".");
  const value = BigInt(whole) * 10n ** 18n + BigInt((match[1] ?? "").padEnd(18, "0") || "0");
  if (value <= 0n) throw new Error("sponsorship amount must be greater than zero");
  return value;
}

export async function revenueSplits(cfg: AppConfig, owner: string, repo: string): Promise<SplitEntry[]> {
  const resolved = await resolveRepo(cfg, owner, repo);
  const values = await readModule(cfg, "economic", "revenueSplits", [resolved.repoId]) as Array<{
    recipient: Address; bps: number;
  }>;
  return values.map((value) => ({ address: toInjectiveAddress(value.recipient), bps: Number(value.bps) }));
}

export async function sponsorTotals(cfg: AppConfig, owner: string, repo: string): Promise<SponsorTotal[]> {
  const resolved = await resolveRepo(cfg, owner, repo);
  const binding = await verifySuite(cfg);
  const denoms = await readModule(cfg, "economic", "sponsorDenoms", [resolved.repoId], binding) as string[];
  const amounts = await Promise.all(denoms.map((denom) =>
    readModule(cfg, "economic", "sponsorTotal", [resolved.repoId, denom], binding) as Promise<bigint>,
  ));
  return denoms.map((denom, index) => ({ denom, amount: amounts[index].toString() }));
}

export async function contractConfig(cfg: AppConfig): Promise<ContractConfig> {
  const binding = await verifySuite(cfg);
  const [admin, committee, treasury, fee] = await Promise.all([
    readModule(cfg, "economic", "admin", [], binding),
    readModule(cfg, "moderation", "committee", [], binding),
    readModule(cfg, "economic", "treasury", [], binding),
    readModule(cfg, "economic", "platformFeeBps", [], binding),
  ]);
  return {
    admin: toInjectiveAddress(admin as Address),
    moderation_committee: toInjectiveAddress(committee as Address),
    treasury: toInjectiveAddress(treasury as Address),
    platform_fee_bps: Number(fee),
    suite_version: 3,
    snapshot_root: binding.snapshotRoot,
  };
}

export async function sponsorWithEconomicModule(
  provider: Eip1193,
  cfg: AppConfig,
  repoId: Hex,
  amountInj: string,
  message: string,
): Promise<Hex> {
  const trimmed = message.trim();
  if (new TextEncoder().encode(trimmed).length > 256) {
    throw new Error("sponsor message must contain at most 256 UTF-8 bytes");
  }
  const hash = await writeModule(provider, cfg, "economic", "sponsor", [repoId, trimmed], parseInjAmount(amountInj));
  clearQueryCache();
  return hash;
}

export async function setRevenueSplitsWithEconomicModule(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, splits: SplitEntry[],
): Promise<Hex> {
  if (splits.length > MAX_REVENUE_SPLIT_RECIPIENTS) {
    throw new Error(`revenue splits support at most ${MAX_REVENUE_SPLIT_RECIPIENTS} recipients`);
  }
  const recipients = splits.map((split) => toEvmAddress(split.address));
  if (new Set(recipients.map((address) => address.toLowerCase())).size !== recipients.length) {
    throw new Error("revenue split recipients must be unique");
  }
  const bps = splits.map((split) => {
    if (!Number.isInteger(split.bps) || split.bps <= 0 || split.bps > 10_000) {
      throw new Error(`invalid revenue split bps ${split.bps}`);
    }
    return split.bps;
  });
  if (bps.reduce((sum, value) => sum + value, 0) > 10_000) {
    throw new Error("revenue split total exceeds 10000 bps");
  }
  const hash = await writeModule(provider, cfg, "economic", "setRevenueSplits", [repoId, recipients, bps]);
  clearQueryCache();
  return hash;
}

async function drainBadges(
  cfg: AppConfig,
  functionName: "listBadgesByRecipientPage" | "listBadgesByRepositoryPage",
  key: Address | Hex,
): Promise<RawBadge[]> {
  const binding = await verifySuite(cfg);
  const output: RawBadge[] = [];
  let cursor = 0n;
  while (true) {
    const [page, next] = await readModule(
      cfg, "badge", functionName, [key, cursor, PAGE_SIZE], binding,
    ) as [RawBadge[], bigint];
    output.push(...page);
    if (next === cursor && page.length !== 0) throw new Error("badge page cursor did not advance");
    if (page.length < Number(PAGE_SIZE)) break;
    cursor = next;
  }
  return output;
}

async function mapBadges(cfg: AppConfig, badges: RawBadge[]): Promise<BadgeInfo[]> {
  const binding = await verifySuite(cfg);
  const repositories = new Map<string, Awaited<ReturnType<typeof repoInfoById>>>();
  await Promise.all([...new Set(badges.map((badge) => badge.repoId.toLowerCase()))].map(async (id) => {
    repositories.set(id, await repoInfoById(cfg, id as Hex, binding));
  }));
  return badges.map((badge) => {
    if (!badge.exists) throw new Error(`badge ${badge.id} does not exist`);
    const repository = repositories.get(badge.repoId.toLowerCase());
    if (!repository) throw new Error(`badge ${badge.id} repository metadata is missing`);
    return {
      id: safeNumber(badge.id, "badge ID"),
      repo_owner: repository.owner,
      repo_name: repository.name,
      recipient: toInjectiveAddress(badge.recipient),
      reason: badge.reason,
      repo_id: badge.repoId,
      awarded_by: toInjectiveAddress(badge.awardedBy),
      awarded_at: safeNumber(badge.awardedAt, "badge timestamp"),
    };
  });
}

export async function badgesByRecipient(cfg: AppConfig, recipient: string): Promise<BadgeInfo[]> {
  return mapBadges(cfg, await drainBadges(cfg, "listBadgesByRecipientPage", toEvmAddress(recipient)));
}

export async function badgesByRepo(cfg: AppConfig, owner: string, repo: string): Promise<BadgeInfo[]> {
  const resolved = await resolveRepo(cfg, owner, repo);
  return mapBadges(cfg, await drainBadges(cfg, "listBadgesByRepositoryPage", resolved.repoId));
}

export async function awardBadgeWithEvm(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, recipient: string, reason: string,
): Promise<Hex> {
  const trimmed = reason.trim();
  const length = new TextEncoder().encode(trimmed).length;
  if (length === 0 || length > 256) throw new Error("badge reason must contain 1 to 256 UTF-8 bytes");
  const hash = await writeModule(
    provider, cfg, "badge", "awardBadge", [repoId, toEvmAddress(recipient), trimmed],
  );
  clearQueryCache();
  return hash;
}

export async function resolveUsername(cfg: AppConfig, name: string): Promise<string> {
  const record = await readModule(cfg, "username", "resolveUsername", [name]) as { owner: Address };
  return toInjectiveAddress(record.owner);
}

export async function addressUsername(cfg: AppConfig, address: string): Promise<string | null> {
  const name = await readModule(cfg, "username", "usernameOf", [toEvmAddress(address)]) as string;
  return name || null;
}

export async function resolveOwner(cfg: AppConfig, owner: string): Promise<string> {
  try {
    return toInjectiveAddress(toEvmAddress(owner));
  } catch {
    return resolveUsername(cfg, owner);
  }
}

export async function registerUsernameWithEvm(
  provider: Eip1193, cfg: AppConfig, name: string,
): Promise<Hex> {
  return writeModule(provider, cfg, "username", "registerUsername", [name]);
}

export async function claimOriginalUsernameWithEvm(
  provider: Eip1193, cfg: AppConfig, name: string,
): Promise<Hex> {
  return writeModule(provider, cfg, "username", "claimOriginalUsername", [name]);
}

export function releaseUsernameWithEvm(provider: Eip1193, cfg: AppConfig): Promise<Hex> {
  return writeModule(provider, cfg, "username", "releaseUsername");
}

export async function injBalanceOf(cfg: AppConfig, address: string): Promise<string> {
  return (await nativeBalance(cfg, toEvmAddress(address))).toString();
}

export async function moderationReportWithEvm(
  cfg: AppConfig, id: EVMModerationReportID,
): Promise<EVMModerationReport> {
  const raw = await readModule(cfg, "moderation", "getReport", [reportId(id)]) as {
    id: bigint; repoId: Hex; reporter: Address; status: number; resolution: number;
    reasonHash: string; createdAt: bigint; updatedAt: bigint; exists: boolean;
  };
  if (!raw.exists) throw new Error(`moderation report not found: ${String(id)}`);
  const state = (["open", "resolved", "appealed", "appeal_resolved"] as const)[Number(raw.status)];
  if (!state) throw new Error(`invalid moderation report state ${String(raw.status)}`);
  return {
    id: raw.id.toString(),
    repoId: raw.repoId,
    reporter: toInjectiveAddress(raw.reporter),
    state,
    resolution: moderationStatus(raw.resolution),
    reasonHash: raw.reasonHash,
    createdAt: safeNumber(raw.createdAt, "moderation report created timestamp"),
    updatedAt: safeNumber(raw.updatedAt, "moderation report updated timestamp"),
  };
}

export function submitModerationReportWithEvm(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, reasonHash: string,
): Promise<Hex> {
  return writeModule(provider, cfg, "moderation", "submitReport", [repoId, reasonHash]);
}

export function setModerationStatusWithEvm(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, status: EVMModerationStatus, reasonHash: string,
): Promise<Hex> {
  return writeModule(
    provider, cfg, "moderation", "setRepositoryStatus", [repoId, moderationStatusValue(status), reasonHash],
  );
}

export function resolveModerationReportWithEvm(
  provider: Eip1193, cfg: AppConfig, id: EVMModerationReportID,
  status: EVMModerationStatus, reasonHash: string,
): Promise<Hex> {
  return writeModule(
    provider, cfg, "moderation", "resolveReport", [reportId(id), moderationStatusValue(status), reasonHash],
  );
}

export function appealModerationReportWithEvm(
  provider: Eip1193, cfg: AppConfig, id: EVMModerationReportID, reasonHash: string,
): Promise<Hex> {
  return writeModule(provider, cfg, "moderation", "appealReport", [reportId(id), reasonHash]);
}

export function resolveModerationAppealWithEvm(
  provider: Eip1193, cfg: AppConfig, id: EVMModerationReportID,
  status: EVMModerationStatus, reasonHash: string,
): Promise<Hex> {
  return writeModule(
    provider, cfg, "moderation", "resolveAppeal", [reportId(id), moderationStatusValue(status), reasonHash],
  );
}

export async function guardianConfig(cfg: AppConfig, repoId: Hex): Promise<GuardianConfig> {
  const value = await readModule(cfg, "recovery", "guardianConfig", [repoId]) as {
    configuredBy: Address; threshold: number; guardians: Address[];
  };
  return {
    configuredBy: value.configuredBy === zeroAddress ? "" : toInjectiveAddress(value.configuredBy),
    threshold: Number(value.threshold),
    guardians: value.guardians.map(toInjectiveAddress),
  };
}

export async function recoveryProposal(cfg: AppConfig, repoId: Hex): Promise<RecoveryProposal | null> {
  const value = await readModule(cfg, "recovery", "recoveryProposal", [repoId]) as {
    proposedBy: Address; newOwner: Address; executeAfter: bigint; expiresAt: bigint; nonce: bigint; approvals: number;
  };
  if (value.proposedBy === zeroAddress) return null;
  return {
    proposedBy: toInjectiveAddress(value.proposedBy),
    newOwner: toInjectiveAddress(value.newOwner),
    executeAfter: safeNumber(value.executeAfter, "recovery execute timestamp"),
    expiresAt: safeNumber(value.expiresAt, "recovery expiry timestamp"),
    nonce: value.nonce.toString(),
    approvals: Number(value.approvals),
  };
}

export function setGuardiansWithEvm(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, guardians: string[], threshold: number,
): Promise<Hex> {
  return writeModule(provider, cfg, "recovery", "setGuardians", [repoId, guardians.map(toEvmAddress), threshold]);
}

export function proposeRecoveryWithEvm(
  provider: Eip1193, cfg: AppConfig, repoId: Hex, newOwner: string,
): Promise<Hex> {
  return writeModule(provider, cfg, "recovery", "proposeRecovery", [repoId, toEvmAddress(newOwner)]);
}

export function approveRecoveryWithEvm(provider: Eip1193, cfg: AppConfig, repoId: Hex): Promise<Hex> {
  return writeModule(provider, cfg, "recovery", "approveRecovery", [repoId]);
}

export function cancelRecoveryWithEvm(provider: Eip1193, cfg: AppConfig, repoId: Hex): Promise<Hex> {
  return writeModule(provider, cfg, "recovery", "cancelRecovery", [repoId]);
}

export function executeRecoveryWithEvm(provider: Eip1193, cfg: AppConfig, repoId: Hex): Promise<Hex> {
  return writeModule(provider, cfg, "recovery", "executeRecovery", [repoId]);
}

export async function releaseArtifact(
  cfg: AppConfig, version: string, platform: string,
): Promise<ReleaseArtifact> {
  const value = await readModule(cfg, "release", "getArtifact", [version, platform]) as {
    version: string; platform: string; sha256: Hex; registeredBy: Address; registeredAt: bigint; exists: boolean;
  };
  if (!value.exists) throw new Error(`release artifact not found: ${version}/${platform}`);
  return {
    version: value.version,
    platform: value.platform,
    sha256: value.sha256,
    registeredBy: toInjectiveAddress(value.registeredBy),
    registeredAt: safeNumber(value.registeredAt, "release artifact timestamp"),
  };
}

export function registerReleaseArtifactWithEvm(
  provider: Eip1193, cfg: AppConfig, version: string, platform: string, digest: Hex,
): Promise<Hex> {
  return writeModule(provider, cfg, "release", "registerArtifact", [version, platform, digest]);
}

export { listCollaborators };
