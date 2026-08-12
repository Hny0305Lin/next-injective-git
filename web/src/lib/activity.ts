import { decodeEventLog, type Abi, type Address, type Hex } from "viem";
import { activityAbis } from "./abis";
import { toInjectiveAddress } from "./address";
import { rpcRequest, verifySuite } from "./transport";
import type { AppConfig } from "./profile";

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
  status: Hex;
  gasUsed: Hex;
  logs: RpcLog[];
}

interface RpcBlock {
  timestamp: Hex;
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

async function blockTimestamp(cfg: AppConfig, height: Hex, cache: Map<string, string>): Promise<string> {
  const hit = cache.get(height);
  if (hit) return hit;
  const block = await rpcRequest<RpcBlock>(cfg, "eth_getBlockByNumber", [height, false]);
  const timestamp = new Date(Number(quantity(block.timestamp)) * 1000).toISOString();
  cache.set(height, timestamp);
  return timestamp;
}

export async function contractActivity(
  cfg: AppConfig,
  limit = 50,
  sender?: string,
): Promise<ContractTx[]> {
  const suite = await verifySuite(cfg);
  const latest = quantity(await rpcRequest<Hex>(cfg, "eth_blockNumber"));
  const from = latest > 100_000n ? latest - 100_000n : 0n;
  const logs = await rpcRequest<RpcLog[]>(cfg, "eth_getLogs", [{
    address: Object.values(suite.modules),
    fromBlock: blockTag(from),
    toBlock: blockTag(latest),
  }]);
  const unique = new Map<string, RpcLog>();
  for (const log of logs) {
    if (!log.removed) unique.set(log.transactionHash.toLowerCase(), log);
  }
  const ordered = [...unique.values()].sort((left, right) => {
    const block = quantity(right.blockNumber) - quantity(left.blockNumber);
    return block === 0n ? Number(quantity(right.logIndex) - quantity(left.logIndex)) : block > 0n ? 1 : -1;
  });
  const timestampCache = new Map<string, string>();
  const targetSender = sender?.toLowerCase();
  const output: ContractTx[] = [];
  for (const log of ordered) {
    const [transaction, receipt, timestamp] = await Promise.all([
      rpcRequest<RpcTransaction>(cfg, "eth_getTransactionByHash", [log.transactionHash]),
      rpcRequest<RpcReceipt>(cfg, "eth_getTransactionReceipt", [log.transactionHash]),
      blockTimestamp(cfg, log.blockNumber, timestampCache),
    ]);
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
    if (output.length >= limit) break;
  }
  return output;
}

export async function txByHash(cfg: AppConfig, hash: string): Promise<TxDetail | null> {
  await verifySuite(cfg);
  const clean = hash.trim();
  if (!/^0x[0-9a-fA-F]{64}$/.test(clean)) throw new Error("invalid EVM transaction hash");
  const transaction = await rpcRequest<RpcTransaction | null>(cfg, "eth_getTransactionByHash", [clean]);
  if (!transaction || !transaction.blockNumber) return null;
  const [receipt, block] = await Promise.all([
    rpcRequest<RpcReceipt>(cfg, "eth_getTransactionReceipt", [clean]),
    rpcRequest<RpcBlock>(cfg, "eth_getBlockByNumber", [transaction.blockNumber, false]),
  ]);
  return {
    txhash: transaction.hash,
    height: quantity(transaction.blockNumber).toString(),
    timestamp: new Date(Number(quantity(block.timestamp)) * 1000).toISOString(),
    code: quantity(receipt.status) === 1n ? 0 : 1,
    rawLog: "",
    gasUsed: quantity(receipt.gasUsed).toString(),
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
