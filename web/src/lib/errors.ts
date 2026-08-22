// Canonical moved to ./chain/errors.ts — this file remains for compatibility
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
  const walletMessage = walletErrorMessage(providerErrorCode(error));
  if (walletMessage) return walletMessage;
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

function numericErrorCode(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isInteger(value)) return value;
  if (typeof value === "string" && /^-?\d+$/.test(value.trim())) {
    const parsed = Number(value.trim());
    return Number.isSafeInteger(parsed) ? parsed : undefined;
  }
  return undefined;
}

/** Normalize wallet/RPC error codes across provider-specific nesting and types. */
export function providerErrorCode(error: unknown, depth = 0, seen = new Set<object>()): number | undefined {
  if (depth > 6 || error == null) return undefined;
  const direct = numericErrorCode(error);
  if (direct !== undefined) return direct;
  if (typeof error !== "object") return undefined;
  if (seen.has(error)) return undefined;
  seen.add(error);
  const record = error as Record<string, unknown>;
  const code = numericErrorCode(record.code);
  if (code !== undefined) return code;
  for (const key of ["error", "originalError", "cause", "data", "details"]) {
    const nested = providerErrorCode(record[key], depth + 1, seen);
    if (nested !== undefined) return nested;
  }
  return undefined;
}

function walletErrorMessage(code: number | undefined): string | undefined {
  switch (code) {
    case 4001: return "Wallet request was rejected. Approve it in your wallet to continue.";
    case 4100: return "This wallet is not authorized for this site. Reconnect it in the wallet extension.";
    case 4200: return "This wallet does not support the requested EVM method.";
    case 4902: return "Injective EVM testnet is not configured in this wallet.";
    case -32002: return "A wallet request is already pending. Complete or cancel it in the wallet.";
    default: return undefined;
  }
}

/** Convert common EIP-1193 errors into retryable, user-actionable copy. */
export function formatWalletError(error: unknown): string {
  return walletErrorMessage(providerErrorCode(error)) ?? formatError(error);
}

export function formatResourceError(error: unknown, resource: "owner" | "repository"): string {
  const message = formatError(error).toLowerCase();
  const label = resource === "owner" ? "owner" : "repository";
  if (message.includes("failed to fetch") || message.includes("network error")) {
    return "Network unavailable. Check your connection and try again.";
  }
  if (error instanceof SuiteConfigurationError || message.includes("suitedirectory is not configured")) {
    return "Repository data is not configured. Open Settings and add a verified SuiteDirectory.";
  }
  if (
    error instanceof SuiteVerificationError ||
    message.includes("suitedirectory") ||
    message.includes("suite rpc") ||
    message.includes("suite module") ||
    message.includes("bootstrap coordinator")
  ) {
    return "Repository data could not be verified. Check the SuiteDirectory in Settings and try again.";
  }
  if (
    message.includes("not found") ||
    message.includes("locatornotfound") ||
    message.includes("invalid username") ||
    message.includes("invalidusername") ||
    message.includes("usernamenotfound") ||
    message.includes("invalid injective address")
  ) {
    return `Could not find this ${label}. Check the ${resource === "owner" ? "address or username" : "owner and repository name"}.`;
  }
  return `Unable to load this ${label}. Please try again.`;
}
