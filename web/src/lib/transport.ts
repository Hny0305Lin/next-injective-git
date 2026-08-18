import {
  createPublicClient,
  createWalletClient,
  custom,
  decodeErrorResult,
  decodeFunctionResult,
  encodeFunctionData,
  getAddress,
  isAddress,
  keccak256,
  type Abi,
  type Address,
  type Hex,
} from "viem";
import {
  MODULE_ABIS,
  MODULE_IDS,
  MODULE_KEYS,
  SUITE_VERSION,
  boundModuleAbi,
  directoryAbi,
  type SuiteModuleKey,
} from "./abis";
import {
  EVMReceiptUnconfirmedError,
  EVMTransactionRevertedError,
  SuiteConfigurationError,
  SuiteVerificationError,
  WalletTransactionBusyError,
  findRpcData,
  formatError,
} from "./errors";
import { networkProfile, type AppConfig } from "./profile";

export interface Eip1193 {
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
  on?(event: string, listener: (...args: unknown[]) => void): void;
  removeListener?(event: string, listener: (...args: unknown[]) => void): void;
}

interface JsonRpcResponse<T> {
  result?: T;
  error?: unknown;
}

export interface SuiteBinding {
  directory: Address;
  coordinator: Address;
  snapshotRoot: Hex;
  blockTag: Hex;
  modules: Record<SuiteModuleKey, Address>;
  codeHashes: Record<SuiteModuleKey, Hex>;
}

const VERIFY_TTL_MS = 30_000;
const verificationCache = new Map<string, { value: SuiteBinding; timestamp: number }>();
const verificationPending = new Map<string, Promise<SuiteBinding>>();
const transactionLocks = new WeakSet<object>();

const RECEIPT_ATTEMPTS = 120;
const RECEIPT_INTERVAL_MS = 1_000;
const RECEIPT_INTERNAL_ERROR_LIMIT = 3;
const MIN_INJECTIVE_GAS_PRICE = 160_000_000n;
const GAS_HEADROOM = 10_000n;

function cacheKey(cfg: AppConfig): string {
  return `${cfg.evmChainId}:${cfg.evmRpc}:${cfg.suiteDirectory.toLowerCase()}`;
}

function quantity(value: unknown, label: string): bigint {
  if (typeof value !== "string" || !/^0x(?:0|[1-9a-fA-F][0-9a-fA-F]*)$/.test(value)) {
    throw new Error(`EVM RPC returned an invalid ${label}`);
  }
  return BigInt(value);
}

function requireDirectory(cfg: AppConfig): Address {
  if (!isAddress(cfg.suiteDirectory)) throw new SuiteConfigurationError();
  return getAddress(cfg.suiteDirectory);
}

export async function rpcRequest<T>(cfg: AppConfig, method: string, params: unknown[] = []): Promise<T> {
  if (!cfg.evmRpc) throw new SuiteConfigurationError("EVM JSON-RPC is not configured for this profile");
  const response = await fetch(cfg.evmRpc, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: Date.now(), method, params }),
  });
  const body = await response.text();
  let json: JsonRpcResponse<T>;
  try {
    json = JSON.parse(body) as JsonRpcResponse<T>;
  } catch {
    throw new Error(response.ok ? `EVM ${method} returned malformed JSON` : `EVM RPC failed (HTTP ${response.status})`);
  }
  if (json.error) throw rpcContractError(json.error);
  if (!response.ok) throw new Error(`EVM RPC failed (HTTP ${response.status})`);
  if (json.result === undefined) throw new Error(`EVM ${method} returned no result`);
  return json.result;
}

function rpcContractError(error: unknown): Error {
  const data = findRpcData(error);
  if (data) {
    const abis: Abi[] = [directoryAbi, boundModuleAbi, ...Object.values(MODULE_ABIS)];
    for (const abi of abis) {
      try {
        const decoded = decodeErrorResult({ abi, data });
        const args = decoded.args ? `(${Array.from(decoded.args).map(String).join(", ")})` : "";
        return new Error(`${decoded.errorName}${args}`);
      } catch {
        // Try the next module ABI.
      }
    }
  }
  return new Error(formatError(error));
}

async function rawRead(
  cfg: AppConfig,
  address: Address,
  abi: Abi,
  functionName: string,
  args: readonly unknown[] = [],
  blockTag: Hex | "latest" = "latest",
): Promise<unknown> {
  const data = encodeFunctionData({ abi, functionName, args } as never);
  const result = await rpcRequest<Hex>(cfg, "eth_call", [{ to: address, data }, blockTag]);
  return decodeFunctionResult({ abi, functionName, data: result } as never);
}

async function verifySuiteNow(cfg: AppConfig): Promise<SuiteBinding> {
  const directory = requireDirectory(cfg);
  const [chainHex, blockTag] = await Promise.all([
    rpcRequest<Hex>(cfg, "eth_chainId"),
    rpcRequest<Hex>(cfg, "eth_blockNumber"),
  ]);
  if (quantity(chainHex, "chain ID") !== BigInt(cfg.evmChainId)) {
    throw new SuiteVerificationError(`Suite RPC chain ID ${quantity(chainHex, "chain ID")} does not match profile ${cfg.evmChainId}`);
  }
  const directoryCode = await rpcRequest<Hex>(cfg, "eth_getCode", [directory, blockTag]);
  if (directoryCode === "0x") throw new SuiteVerificationError(`SuiteDirectory has no code at ${directory}`);

  const [state, version, configuredChainId, snapshotRoot, coordinator, coordinatorHash, count] = await Promise.all([
    rawRead(cfg, directory, directoryAbi, "state", [], blockTag),
    rawRead(cfg, directory, directoryAbi, "suiteVersion", [], blockTag),
    rawRead(cfg, directory, directoryAbi, "configuredChainId", [], blockTag),
    rawRead(cfg, directory, directoryAbi, "snapshotRoot", [], blockTag),
    rawRead(cfg, directory, directoryAbi, "bootstrapCoordinator", [], blockTag),
    rawRead(cfg, directory, directoryAbi, "bootstrapCoordinatorCodeHash", [], blockTag),
    rawRead(cfg, directory, directoryAbi, "registeredModuleCount", [], blockTag),
  ]);
  if (Number(state) !== 1) throw new SuiteVerificationError("SuiteDirectory is not active");
  if (BigInt(version as bigint) !== SUITE_VERSION) {
    throw new SuiteVerificationError(`unsupported suite version ${String(version)}; expected ${SUITE_VERSION}`);
  }
  if (BigInt(configuredChainId as bigint) !== BigInt(cfg.evmChainId)) {
    throw new SuiteVerificationError("SuiteDirectory configured chain ID does not match the profile");
  }
  if (BigInt(count as bigint) !== BigInt(MODULE_KEYS.length)) {
    throw new SuiteVerificationError(`SuiteDirectory contains ${String(count)} modules; expected ${MODULE_KEYS.length}`);
  }
  if (!isAddress(coordinator as string) || (coordinator as string) === "0x0000000000000000000000000000000000000000") {
    throw new SuiteVerificationError("SuiteDirectory bootstrap coordinator is invalid");
  }
  const normalizedCoordinator = getAddress(coordinator as string);
  const coordinatorCode = await rpcRequest<Hex>(cfg, "eth_getCode", [normalizedCoordinator, blockTag]);
  if (coordinatorCode === "0x" || keccak256(coordinatorCode) !== String(coordinatorHash).toLowerCase()) {
    throw new SuiteVerificationError("bootstrap coordinator code hash mismatch");
  }

  const moduleEntries = await Promise.all(MODULE_KEYS.map(async (key) => {
    const id = MODULE_IDS[key];
    const [address, expectedHash, verified] = await Promise.all([
      rawRead(cfg, directory, directoryAbi, "moduleAddress", [id], blockTag),
      rawRead(cfg, directory, directoryAbi, "moduleCodeHash", [id], blockTag),
      rawRead(cfg, directory, directoryAbi, "verifyModule", [id], blockTag),
    ]);
    if (!isAddress(address as string) || (address as string) === "0x0000000000000000000000000000000000000000") {
      throw new SuiteVerificationError(`suite module ${key} has an invalid address`);
    }
    if (verified !== true) throw new SuiteVerificationError(`SuiteDirectory rejected module ${key}`);
    const moduleAddress = getAddress(address as string);
    const code = await rpcRequest<Hex>(cfg, "eth_getCode", [moduleAddress, blockTag]);
    if (code === "0x" || keccak256(code) !== String(expectedHash).toLowerCase()) {
      throw new SuiteVerificationError(`suite module ${key} code hash mismatch`);
    }
    const [boundDirectory, boundCoordinator, boundId] = await Promise.all([
      rawRead(cfg, moduleAddress, boundModuleAbi, "suiteDirectory", [], blockTag),
      rawRead(cfg, moduleAddress, boundModuleAbi, "bootstrapCoordinator", [], blockTag),
      rawRead(cfg, moduleAddress, boundModuleAbi, "moduleId", [], blockTag),
    ]);
    if (String(boundDirectory).toLowerCase() !== directory.toLowerCase()) {
      throw new SuiteVerificationError(`suite module ${key} directory binding mismatch`);
    }
    if (String(boundCoordinator).toLowerCase() !== normalizedCoordinator.toLowerCase()) {
      throw new SuiteVerificationError(`suite module ${key} coordinator binding mismatch`);
    }
    if (String(boundId).toLowerCase() !== id.toLowerCase()) {
      throw new SuiteVerificationError(`suite module ${key} ID binding mismatch`);
    }
    return [key, moduleAddress, String(expectedHash).toLowerCase() as Hex] as const;
  }));

  return {
    directory,
    coordinator: normalizedCoordinator,
    snapshotRoot: snapshotRoot as Hex,
    blockTag,
    modules: Object.fromEntries(moduleEntries.map(([key, address]) => [key, address])) as Record<SuiteModuleKey, Address>,
    codeHashes: Object.fromEntries(moduleEntries.map(([key, , hash]) => [key, hash])) as Record<SuiteModuleKey, Hex>,
  };
}

export async function verifySuite(cfg: AppConfig, force = false): Promise<SuiteBinding> {
  requireDirectory(cfg);
  const key = cacheKey(cfg);
  const cached = verificationCache.get(key);
  if (!force && cached && Date.now() - cached.timestamp < VERIFY_TTL_MS) return cached.value;
  const pending = verificationPending.get(key);
  if (!force && pending) return pending;
  const promise = verifySuiteNow(cfg).then((value) => {
    verificationCache.set(key, { value, timestamp: Date.now() });
    return value;
  }).finally(() => verificationPending.delete(key));
  verificationPending.set(key, promise);
  return promise;
}

export function clearSuiteCache(): void {
  verificationCache.clear();
  verificationPending.clear();
}

export async function readModule(
  cfg: AppConfig,
  module: SuiteModuleKey,
  functionName: string,
  args: readonly unknown[] = [],
  binding?: SuiteBinding,
): Promise<unknown> {
  const suite = binding ?? await verifySuite(cfg);
  return rawRead(cfg, suite.modules[module], MODULE_ABIS[module], functionName, args, suite.blockTag);
}

function rpcCode(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const value = (error as { code?: unknown }).code;
  return typeof value === "number" ? value : undefined;
}

export async function ensureWalletChain(provider: Eip1193, cfg: AppConfig): Promise<void> {
  const chainId = `0x${cfg.evmChainId.toString(16)}`;
  try {
    await provider.request({ method: "wallet_switchEthereumChain", params: [{ chainId }] });
  } catch (error) {
    if (rpcCode(error) !== 4902) throw error;
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
    await provider.request({ method: "wallet_switchEthereumChain", params: [{ chainId }] });
  }
  const active = await provider.request({ method: "eth_chainId" });
  if (quantity(active, "wallet chain ID") !== BigInt(cfg.evmChainId)) {
    throw new Error(`wallet is connected to the wrong EVM chain: ${String(active)}`);
  }
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function waitForReceipt(provider: Eip1193, txHash: Hex): Promise<void> {
  let internalErrors = 0;
  for (let attempt = 0; attempt < RECEIPT_ATTEMPTS; attempt += 1) {
    try {
      const value = await provider.request({ method: "eth_getTransactionReceipt", params: [txHash] });
      internalErrors = 0;
      if (value != null) {
        if (typeof value !== "object") throw new EVMReceiptUnconfirmedError(txHash);
        const status = (value as { status?: unknown }).status;
        if (quantity(status, "transaction receipt status") === 0n) throw new EVMTransactionRevertedError(txHash);
        if (quantity(status, "transaction receipt status") !== 1n) throw new EVMReceiptUnconfirmedError(txHash);
        return;
      }
    } catch (error) {
      if (error instanceof EVMTransactionRevertedError || error instanceof EVMReceiptUnconfirmedError) throw error;
      const message = formatError(error).toLowerCase();
      if (rpcCode(error) !== -32603 && !message.includes("internal error")) {
        throw new EVMReceiptUnconfirmedError(txHash, { cause: error });
      }
      internalErrors += 1;
      if (internalErrors >= RECEIPT_INTERNAL_ERROR_LIMIT) {
        throw new EVMReceiptUnconfirmedError(txHash, { cause: error });
      }
    }
    if (attempt + 1 < RECEIPT_ATTEMPTS) await delay(RECEIPT_INTERVAL_MS);
  }
  throw new EVMReceiptUnconfirmedError(txHash);
}

export async function writeModule(
  provider: Eip1193,
  cfg: AppConfig,
  module: SuiteModuleKey,
  functionName: string,
  args: readonly unknown[] = [],
  value?: bigint,
): Promise<Hex> {
  const identity = provider as object;
  if (transactionLocks.has(identity)) throw new WalletTransactionBusyError();
  transactionLocks.add(identity);
  try {
    const suite = await verifySuite(cfg, true);
    const accounts = await provider.request({ method: "eth_requestAccounts" });
    const from = Array.isArray(accounts) ? accounts[0] : undefined;
    if (typeof from !== "string" || !isAddress(from)) throw new Error("EVM wallet returned no valid account");
    await ensureWalletChain(provider, cfg);
    const data = encodeFunctionData({ abi: MODULE_ABIS[module], functionName, args } as never);
    const nodeGasPrice = quantity(await provider.request({ method: "eth_gasPrice" }), "gas price");
    const gasPrice = nodeGasPrice < MIN_INJECTIVE_GAS_PRICE ? MIN_INJECTIVE_GAS_PRICE : nodeGasPrice;
    const account = getAddress(from);
    const publicClient = createPublicClient({ transport: custom(provider) });
    const walletClient = createWalletClient({ transport: custom(provider) });
    const base = {
      account,
      to: suite.modules[module],
      data,
      type: "legacy" as const,
      gasPrice,
      ...(value === undefined ? {} : { value }),
    };
    const estimate = await publicClient.estimateGas(base);
    const gas = (estimate * 14n) / 10n + GAS_HEADROOM;
    let txHash: Hex;
    try {
      txHash = await walletClient.sendTransaction({ ...base, gas, chain: null });
    } catch (error) {
      throw rpcContractError(error);
    }
    if (!/^0x[0-9a-fA-F]{64}$/.test(txHash)) {
      throw new Error("EVM wallet returned an invalid transaction hash");
    }
    await waitForReceipt(provider, txHash);
    clearSuiteCache();
    return txHash;
  } finally {
    transactionLocks.delete(identity);
  }
}

export async function nativeBalance(cfg: AppConfig, address: Address): Promise<bigint> {
  return quantity(await rpcRequest<Hex>(cfg, "eth_getBalance", [address, "latest"]), "balance");
}
