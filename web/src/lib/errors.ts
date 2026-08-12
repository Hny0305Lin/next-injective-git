export class SuiteConfigurationError extends Error {
  constructor(message = "EVM SuiteDirectory is not configured for this network profile") {
    super(message);
    this.name = "SuiteConfigurationError";
  }
}

export class SuiteVerificationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SuiteVerificationError";
  }
}

export class WalletTransactionBusyError extends Error {
  constructor() {
    super("another transaction is already in progress for this wallet");
    this.name = "WalletTransactionBusyError";
  }
}

export class EVMReceiptUnconfirmedError extends Error {
  constructor(public readonly txHash: string, options?: ErrorOptions) {
    super(`transaction broadcast but confirmation is uncertain: ${txHash}`, options);
    this.name = "EVMReceiptUnconfirmedError";
  }
}

export class EVMTransactionRevertedError extends Error {
  constructor(public readonly txHash: string) {
    super(`EVM transaction reverted: ${txHash}`);
    this.name = "EVMTransactionRevertedError";
  }
}

export function findRpcData(value: unknown, depth = 0): `0x${string}` | undefined {
  if (depth > 6 || value == null) return undefined;
  if (typeof value === "string") {
    return /^0x[0-9a-fA-F]{8,}$/.test(value) ? value as `0x${string}` : undefined;
  }
  if (typeof value !== "object") return undefined;
  const record = value as Record<string, unknown>;
  for (const key of ["data", "result", "return", "originalError", "error", "cause"]) {
    const found = findRpcData(record[key], depth + 1);
    if (found) return found;
  }
  return undefined;
}

export function formatError(error: unknown): string {
  if (error == null) return "unknown error";
  if (typeof error === "string") return error;
  if (error instanceof Error) return error.message;
  if (typeof error === "object") {
    const record = error as Record<string, unknown>;
    for (const key of ["shortMessage", "message", "originalMessage", "reason"]) {
      if (typeof record[key] === "string" && record[key]) return record[key];
    }
    const nested = record.error ?? record.data;
    if (nested && nested !== error) return formatError(nested);
    try {
      return JSON.stringify(error);
    } catch {
      return String(error);
    }
  }
  return String(error);
}

export function formatResourceError(error: unknown, resource: "owner" | "repository"): string {
  const message = formatError(error).toLowerCase();
  const label = resource === "owner" ? "owner" : "repository";
  if (message.includes("failed to fetch") || message.includes("network error")) {
    return "Network unavailable. Check your connection and try again.";
  }
  if (message.includes("not found") || message.includes("locatornotfound")) {
    return `Could not find this ${label}. Check the ${resource === "owner" ? "address or username" : "owner and repository name"}.`;
  }
  return `Unable to load this ${label}. Please try again.`;
}
