import { decodeEventLog, type Abi, type Address, type Hex } from "viem";
import { activityAbis } from "./abis";
import { toInjectiveAddress } from "./address";
import { rpcRequest, verifySuite } from "./transport";
import type { AppConfig } from "./profile";

/** Number of recent EVM blocks sampled by the public activity view. */
export const EVM_ACTIVITY_BLOCK_WINDOW = 100_000;

/**
 * Injective JSON-RPC rejects `eth_getLogs` when `to - from` exceeds 10000
 * ("maximum [from, to] blocks distance: 10000"), so the activity window is
 * walked in chunks that stay inside the limit instead of one wide query.
 */
export const EVM_LOG_RANGE_LIMIT = 10_000;

/** Bounds the adaptive split when a provider enforces a stricter range. */
const LOG_RANGE_SPLIT_DEPTH = 6;

export interface ContractTx {
  txhash: string;
  height: string;
  timestamp: string;
  code: number;
  action: string;
  sender: string;
  attributes: Record<string, string>;
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

interface RpcLog {
  address: Address;
  topics: Hex[];
  data: Hex;
  blockNumber: Hex;
  transactionHash: Hex;
  logIndex: Hex;
  removed?: boolean;
}

interface RpcTransaction {
  hash: Hex;
  from: Address;
  to: Address | null;
  input: Hex;
  value: Hex;
  type: Hex;
  blockNumber: Hex | null;
}

interface RpcReceipt {
  status: Hex | null;
  gasUsed: Hex | null;
  from?: Address;
  logs: RpcLog[] | null;
}

interface RpcBlock {
  timestamp: Hex | null;
}

const EVENT_ACTIONS: Record<string, string> = {
  RepositoryCreated: "create_repo",
  RepositoryMetadataUpdated: "update_repo",
  RefUpdated: "update_ref",
  RefDeleted: "delete_ref",
  CollaboratorUpdated: "set_collaborator",
  OwnershipTransferStarted: "begin_transfer",
  OwnershipTransferCancelled: "cancel_transfer",
  OwnershipTransferred: "transfer",
  RepositoryStatusSet: "moderation_status",
  ReportSubmitted: "report",
  SponsorSettled: "sponsor",
  RevenueSplitsUpdated: "set_splits",
  BadgeAwarded: "award_badge",
};

function quantity(value: Hex): bigint {
  return BigInt(value);
}

function blockTag(value: bigint): Hex {
  return `0x${value.toString(16)}`;
}

function displayValue(value: unknown): string {
  if (typeof value === "bigint") return value.toString();
  if (typeof value === "string") {
    if (/^0x[0-9a-fA-F]{40}$/.test(value)) return toInjectiveAddress(value);
    return value;
  }
  if (typeof value === "boolean" || typeof value === "number") return String(value);
  if (Array.isArray(value)) return value.map(displayValue).join(",");
  return String(value ?? "");
}

function decodeLog(log: RpcLog): { action: string; attributes: Record<string, string> } {
  for (const abi of activityAbis as readonly Abi[]) {
    try {
      if (log.topics.length === 0) continue;
      const decoded = decodeEventLog({
        abi,
        topics: log.topics as [Hex, ...Hex[]],
        data: log.data,
        strict: false,
      });
      const attributes: Record<string, string> = {};
      if (decoded.args && typeof decoded.args === "object") {
        for (const [key, value] of Object.entries(decoded.args)) attributes[key] = displayValue(value);
      }
      const eventName = String(decoded.eventName ?? "ContractEvent");
      return { action: EVENT_ACTIONS[eventName] ?? eventName, attributes };
    } catch {
      // Event belongs to a different suite module.
    }
  }
  return { action: "contract_event", attributes: {} };
}

async function blockTimestamp(cfg: AppConfig, height: Hex, cache: Map<string, string>): Promise<string | null> {
  const hit = cache.get(height);
  if (hit) return hit;
  const block = await rpcRequest<RpcBlock | null>(cfg, "eth_getBlockByNumber", [height, false]);
  if (!block || !block.timestamp) return null;
  const timestamp = new Date(Number(quantity(block.timestamp)) * 1000).toISOString();
  cache.set(height, timestamp);
  return timestamp;
}

/** Recognizes provider range/result caps so the span can be split and retried. */
function isLogRangeError(error: unknown): boolean {
  const message = (error instanceof Error ? error.message : String(error ?? "")).toLowerCase();
  return (
    message.includes("blocks distance") ||
    message.includes("block range") ||
    message.includes("block distance") ||
    message.includes("range is too large") ||
    message.includes("too many blocks") ||
    message.includes("query returned more than") ||
    message.includes("more than 10000 results") ||
    message.includes("log response size exceeded") ||
    message.includes("query timeout exceeded")
  );
}

/**
 * Reads one bounded span, halving it when the provider still reports a range or
 * result cap. Returns logs in ascending block order.
 */
async function logsInSpan(
  cfg: AppConfig,
  addresses: readonly Address[],
  from: bigint,
  to: bigint,
  depth = 0,
): Promise<RpcLog[]> {
  try {
    return await rpcRequest<RpcLog[]>(cfg, "eth_getLogs", [{
      address: addresses,
      fromBlock: blockTag(from),
      toBlock: blockTag(to),
    }]);
  } catch (error) {
    if (from >= to || depth >= LOG_RANGE_SPLIT_DEPTH || !isLogRangeError(error)) throw error;
    const middle = from + (to - from) / 2n;
    const head = await logsInSpan(cfg, addresses, from, middle, depth + 1);
    const tail = await logsInSpan(cfg, addresses, middle + 1n, to, depth + 1);
    return [...head, ...tail];
  }
}

/**
 * Yields newest-first chunks of the activity window, each within the range
 * limit, so a caller can stop as soon as it has enough events.
 */
function* descendingSpans(from: bigint, to: bigint): Generator<{ from: bigint; to: bigint }> {
  const step = BigInt(EVM_LOG_RANGE_LIMIT);
  let end = to;
  while (end >= from) {
    // A span is inclusive on both ends, so `step - 1` keeps `to - from` at the cap.
    const start = end - step + 1n > from ? end - step + 1n : from;
    yield { from: start, to: end };
    if (start === 0n || start <= from) return;
    end = start - 1n;
  }
}

function orderLogs(logs: readonly RpcLog[]): RpcLog[] {
  const unique = new Map<string, RpcLog>();
  for (const log of logs) {
    if (!log.removed) unique.set(log.transactionHash.toLowerCase(), log);
  }
  return [...unique.values()].sort((left, right) => {
    const block = quantity(right.blockNumber) - quantity(left.blockNumber);
    return block === 0n ? Number(quantity(right.logIndex) - quantity(left.logIndex)) : block > 0n ? 1 : -1;
  });
}

export async function contractActivity(
  cfg: AppConfig,
  limit = 50,
  sender?: string,
): Promise<ContractTx[]> {
  const suite = await verifySuite(cfg);
  const latest = quantity(await rpcRequest<Hex>(cfg, "eth_blockNumber"));
  const window = BigInt(EVM_ACTIVITY_BLOCK_WINDOW);
  const from = latest > window ? latest - window : 0n;
  const addresses = Object.values(suite.modules);
  const timestampCache = new Map<string, string>();
  const targetSender = sender?.toLowerCase();
  const output: ContractTx[] = [];
  const seen = new Set<string>();
  for (const span of descendingSpans(from, latest)) {
    const logs = await logsInSpan(cfg, addresses, span.from, span.to);
    for (const log of orderLogs(logs)) {
      const hash = log.transactionHash.toLowerCase();
      if (seen.has(hash)) continue;
      seen.add(hash);
      const [transaction, receipt, timestamp] = await Promise.all([
        rpcRequest<RpcTransaction | null>(cfg, "eth_getTransactionByHash", [log.transactionHash]),
        rpcRequest<RpcReceipt | null>(cfg, "eth_getTransactionReceipt", [log.transactionHash]),
        blockTimestamp(cfg, log.blockNumber, timestampCache),
      ]);
      if (!transaction || !receipt || !transaction.from || !receipt.status || !receipt.logs || !timestamp) {
        continue;
      }
      const normalizedSender = toInjectiveAddress(transaction.from);
      if (targetSender && normalizedSender.toLowerCase() !== targetSender && transaction.from.toLowerCase() !== targetSender) {
        continue;
      }
      const primary = receipt.logs.map(decodeLog).find((event) => event.action !== "contract_event") ?? decodeLog(log);
      output.push({
        txhash: transaction.hash,
        height: quantity(log.blockNumber).toString(),
        timestamp,
        code: quantity(receipt.status) === 1n ? 0 : 1,
        action: primary.action,
        sender: normalizedSender,
        attributes: primary.attributes,
      });
      if (output.length >= limit) return output;
    }
  }
  return output;
}

export async function txByHash(cfg: AppConfig, hash: string): Promise<TxDetail | null> {
  await verifySuite(cfg);
  const clean = hash.trim();
  if (!/^0x[0-9a-fA-F]{64}$/.test(clean)) throw new Error("invalid EVM transaction hash");
  const transaction = await rpcRequest<RpcTransaction | null>(cfg, "eth_getTransactionByHash", [clean]);
  if (!transaction || !transaction.blockNumber || !transaction.from) return null;
  const [receipt, block] = await Promise.all([
    rpcRequest<RpcReceipt | null>(cfg, "eth_getTransactionReceipt", [clean]),
    rpcRequest<RpcBlock | null>(cfg, "eth_getBlockByNumber", [transaction.blockNumber, false]),
  ]);
  if (!receipt || !receipt.status || !receipt.logs || !block || !block.timestamp) return null;
  return {
    txhash: transaction.hash,
    height: quantity(transaction.blockNumber).toString(),
    timestamp: new Date(Number(quantity(block.timestamp)) * 1000).toISOString(),
    code: quantity(receipt.status) === 1n ? 0 : 1,
    rawLog: "",
    gasUsed: receipt.gasUsed ? quantity(receipt.gasUsed).toString() : "0",
    gasWanted: "",
    messages: [{
      type: `EVM type ${quantity(transaction.type)}`,
      body: {
        from: toInjectiveAddress(transaction.from),
        to: transaction.to,
        value: quantity(transaction.value).toString(),
        input: transaction.input,
      },
    }],
    extensionOptions: [],
    signMode: quantity(transaction.type) === 0n ? "legacy" : `type ${quantity(transaction.type)}`,
    pubkeyType: "EVM",
    events: receipt.logs.map((log) => {
      const event = decodeLog(log);
      return {
        type: event.action,
        attributes: Object.entries(event.attributes).map(([key, value]) => ({ key, value })),
      };
    }),
  };
}
