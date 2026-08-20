import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { toInjectiveAddress } from "./address";
import { clearQueryCache, injBalanceOf, loadConfig } from "./chain";
import { ensureWalletChain, walletChainId, type Eip1193 } from "./transport";
import { formatWalletError, providerErrorCode } from "./errors";
import {
  resolveEvmProvider,
  prepareEvmProvider,
  subscribeWalletProviders,
  SUPPORTED_WALLETS,
  type ResolvedEvmProvider,
} from "./wallet";
import {
  WALLETCONNECT_ID,
  WALLETCONNECT_LABEL,
  WalletConnectPairingCancelledError,
  cancelWalletConnectPairing,
  connectWalletConnect as startWalletConnect,
  disconnectWalletConnect,
  restoreWalletConnect,
  walletConnectConfigured,
} from "./walletconnect";

export interface Connected {
  kind: "evm";
  id: string;
  label: string;
  address: string;
  ethAddress: string;
  chainId: number | null;
  writable: boolean;
}

interface WalletState {
  connected: Connected | null;
  /** The provider selected for the current in-memory session. */
  provider: Eip1193 | null;
  address: string;
  balance: string;
  connecting: boolean;
  error: string;
  connect: (walletId: string) => Promise<boolean>;
  connectWalletConnect: () => Promise<boolean>;
  walletConnectUri: string;
  walletConnectPairing: boolean;
  walletConnectConfigured: boolean;
  cancelWalletConnect: () => void;
  disconnect: () => void;
  refreshBalance: () => Promise<void>;
  walletModalOpen: boolean;
  openWalletModal: () => void;
  closeWalletModal: () => void;
}

interface WalletConnectResolution {
  walletId: typeof WALLETCONNECT_ID;
  provider: Eip1193;
  identity: { source: "walletconnect" };
}

type SessionResolution = ResolvedEvmProvider | WalletConnectResolution;

interface WalletSession {
  resolution: SessionResolution;
  connected: Connected;
}

interface WalletConnectAttempt {
  cancelled: boolean;
  provider: Eip1193 | null;
}

const Ctx = createContext<WalletState | null>(null);
const LS_PROVIDER = "igit-wallet-provider";

function validEthAddress(value: unknown): value is string {
  return typeof value === "string" && /^0x[0-9a-fA-F]{40}$/.test(value);
}

function firstAccount(value: unknown): string | undefined {
  if (!Array.isArray(value)) return undefined;
  const account = value.find(validEthAddress);
  return account;
}

function chainNumber(value: bigint | null): number | null {
  if (value == null || value > BigInt(Number.MAX_SAFE_INTEGER)) return null;
  return Number(value);
}

function formatWalletConnectError(cause: unknown): string {
  const code = providerErrorCode(cause);
  if (code !== undefined) return formatWalletError(cause);
  const message = formatWalletError(cause).toLowerCase();
  if (message.includes("wrong evm chain") || message.includes("switch the wallet")) {
    return "Switch the wallet to Injective EVM testnet and try again.";
  }
  return "WalletConnect pairing failed. The wallet may not support Injective EVM testnet (chain 1439).";
}

function listen(
  provider: Eip1193,
  event: string,
  listener: (...args: unknown[]) => void,
): () => void {
  if (provider.on) {
    provider.on(event, listener);
    return () => {
      provider.off?.(event, listener);
      provider.removeListener?.(event, listener);
      provider.removeEventListener?.(event, listener);
    };
  }
  if (provider.addEventListener) {
    provider.addEventListener(event, listener);
    return () => provider.removeEventListener?.(event, listener);
  }
  return () => undefined;
}

export function WalletProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = useState<Connected | null>(null);
  const [balance, setBalance] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");
  const [walletModalOpen, setWalletModalOpen] = useState(false);
  const [walletConnectUri, setWalletConnectUri] = useState("");
  const [walletConnectPairing, setWalletConnectPairing] = useState(false);
  const sessionRef = useRef<WalletSession | null>(null);
  const connectingRef = useRef(false);
  const walletConnectAttemptRef = useRef<WalletConnectAttempt | null>(null);

  const clearSession = useCallback((removeStored = true) => {
    sessionRef.current = null;
    setConnected(null);
    setBalance("");
    setError("");
    clearQueryCache();
    if (removeStored && typeof localStorage !== "undefined") localStorage.removeItem(LS_PROVIDER);
  }, []);

  const refreshBalance = useCallback(async () => {
    const current = sessionRef.current?.connected;
    if (!current) return;
    try {
      // Native balance reads intentionally use the configured public RPC, not the wallet adapter.
      setBalance(await injBalanceOf(loadConfig(), current.ethAddress));
    } catch {
      // Preserve the last confirmed balance while the RPC is unavailable.
    }
  }, []);

  const adoptAccount = useCallback(async (
    resolution: SessionResolution,
    ethAddress: string,
    persist = false,
  ): Promise<boolean> => {
    const current = sessionRef.current;
    if (current && (current.resolution.provider !== resolution.provider || current.resolution.walletId !== resolution.walletId)) return false;
    const definition = SUPPORTED_WALLETS.find((wallet) => wallet.id === resolution.walletId);
    const label = resolution.walletId === WALLETCONNECT_ID ? WALLETCONNECT_LABEL : definition?.label;
    if (!label || !validEthAddress(ethAddress)) return false;
    const cfg = loadConfig();
    let activeChain: bigint | null = null;
    try {
      activeChain = await walletChainId(resolution.provider);
    } catch {
      // A locked or temporarily unavailable extension can still expose its account.
    }
    if (sessionRef.current && (sessionRef.current.resolution.provider !== resolution.provider
      || sessionRef.current.resolution.walletId !== resolution.walletId)) return false;
    const next: Connected = {
      kind: "evm",
      id: resolution.walletId,
      label,
      address: toInjectiveAddress(ethAddress),
      ethAddress,
      chainId: chainNumber(activeChain),
      writable: activeChain === BigInt(cfg.evmChainId),
    };
    sessionRef.current = { resolution, connected: next };
    setConnected(next);
    if (!next.writable) setError("Switch the wallet to Injective EVM testnet before writing.");
    else setError("");
    if (persist && typeof localStorage !== "undefined") localStorage.setItem(LS_PROVIDER, resolution.walletId);
    return true;
  }, []);

  const walletConnectResolution = useCallback((provider: Eip1193): WalletConnectResolution => ({
    walletId: WALLETCONNECT_ID,
    provider,
    identity: { source: "walletconnect" },
  }), []);

  const cancelWalletConnect = useCallback(() => {
    const attempt = walletConnectAttemptRef.current;
    if (attempt) {
      attempt.cancelled = true;
      cancelWalletConnectPairing();
      if (attempt.provider) void disconnectWalletConnect(attempt.provider);
    }
    setWalletConnectUri("");
  }, []);

  const connect = useCallback(async (walletId: string) => {
    if (connectingRef.current) return false;
    connectingRef.current = true;
    setConnecting(true);
    setError("");
    clearQueryCache();
    const previous = sessionRef.current;
    try {
      const definition = SUPPORTED_WALLETS.find((wallet) => wallet.id === walletId);
      if (!definition) throw new Error(`unsupported wallet ${walletId}`);
      const resolution = resolveEvmProvider(walletId);
      if (!resolution) throw new Error(`${definition.label} not detected; install or enable it`);
      const provider = resolution.provider;
      const cfg = loadConfig();
      // Keplr documents an EVM-provider permission step; no Cosmos signer API is used.
      await prepareEvmProvider(resolution);
      const accounts = await provider.request({ method: "eth_requestAccounts" });
      const ethAddress = firstAccount(accounts);
      if (!ethAddress) throw new Error("EVM wallet returned no valid account");
      await ensureWalletChain(provider, cfg);
      const stillSelected = resolveEvmProvider(walletId, { pinned: resolution });
      if (!stillSelected) throw new Error("The selected wallet provider changed; choose the wallet again");
      const activeChain = await walletChainId(provider);
      if (activeChain !== BigInt(cfg.evmChainId)) {
        throw new Error(`wallet is connected to the wrong EVM chain: ${activeChain.toString()}`);
      }
      const next: Connected = {
        kind: "evm",
        id: walletId,
        label: definition.label,
        address: toInjectiveAddress(ethAddress),
        ethAddress,
        chainId: chainNumber(activeChain),
        writable: true,
      };
      sessionRef.current = { resolution, connected: next };
      setConnected(next);
      if (typeof localStorage !== "undefined") localStorage.setItem(LS_PROVIDER, walletId);
      return true;
    } catch (cause) {
      // A failed attempt to switch wallets must not silently discard a prior valid session.
      if (!previous) clearSession(false);
      setError(formatWalletError(cause));
      return false;
    } finally {
      connectingRef.current = false;
      setConnecting(false);
    }
  }, [clearSession]);

  const connectWalletConnect = useCallback(async () => {
    if (connectingRef.current) return false;
    connectingRef.current = true;
    const attempt: WalletConnectAttempt = { cancelled: false, provider: null };
    walletConnectAttemptRef.current = attempt;
    setConnecting(true);
    setWalletConnectPairing(true);
    setWalletConnectUri("");
    setError("");
    clearQueryCache();
    const previous = sessionRef.current;
    try {
      const provider = await startWalletConnect(setWalletConnectUri);
      attempt.provider = provider;
      if (attempt.cancelled) throw new WalletConnectPairingCancelledError();
      const ethAddress = firstAccount(provider.accounts)
        ?? firstAccount(await provider.request({ method: "eth_accounts" }));
      if (attempt.cancelled) throw new WalletConnectPairingCancelledError();
      if (!ethAddress) throw new Error("WalletConnect returned no valid account");
      const cfg = loadConfig();
      await ensureWalletChain(provider, cfg);
      if (attempt.cancelled) throw new WalletConnectPairingCancelledError();
      const activeChain = await walletChainId(provider);
      if (attempt.cancelled) throw new WalletConnectPairingCancelledError();
      if (activeChain !== BigInt(cfg.evmChainId)) {
        throw new Error(`wallet is connected to the wrong EVM chain: ${activeChain.toString()}`);
      }
      const adopted = await adoptAccount(walletConnectResolution(provider), ethAddress, true);
      if (attempt.cancelled) {
        if (sessionRef.current?.resolution.provider === provider) clearSession(true);
        throw new WalletConnectPairingCancelledError();
      }
      if (!adopted) throw new Error("WalletConnect session changed; choose the wallet again");
      return true;
    } catch (cause) {
      if (attempt.cancelled && attempt.provider) await disconnectWalletConnect(attempt.provider);
      if (!previous) clearSession(false);
      setError(cause instanceof WalletConnectPairingCancelledError ? "" : formatWalletConnectError(cause));
      return false;
    } finally {
      if (walletConnectAttemptRef.current === attempt) walletConnectAttemptRef.current = null;
      setWalletConnectPairing(false);
      setWalletConnectUri("");
      connectingRef.current = false;
      setConnecting(false);
    }
  }, [adoptAccount, clearSession, walletConnectResolution]);

  const restoreWalletConnectConnection = useCallback(async () => {
    try {
      const provider = await restoreWalletConnect();
      if (!provider) {
        if (typeof localStorage !== "undefined") localStorage.removeItem(LS_PROVIDER);
        return;
      }
      const ethAddress = firstAccount(provider.accounts)
        ?? firstAccount(await provider.request({ method: "eth_accounts" }));
      if (!ethAddress) {
        if (typeof localStorage !== "undefined") localStorage.removeItem(LS_PROVIDER);
        return;
      }
      await adoptAccount(walletConnectResolution(provider), ethAddress, false);
    } catch {
      // Silent restore must never open a QR prompt or show a blocking error.
    }
  }, [adoptAccount, walletConnectResolution]);

  const restoreConnection = useCallback(async (
    walletId: string,
    pinned?: ResolvedEvmProvider,
  ) => {
    const definition = SUPPORTED_WALLETS.find((wallet) => wallet.id === walletId);
    const resolution = pinned
      ? resolveEvmProvider(walletId, { pinned })
      : resolveEvmProvider(walletId, { requireUnique: true });
    if (!definition || !resolution) return;
    try {
      const accounts = await resolution.provider.request({ method: "eth_accounts" });
      const ethAddress = firstAccount(accounts);
      if (!ethAddress) {
        clearSession(true);
        return;
      }
      await adoptAccount(resolution, ethAddress, false);
    } catch {
      // Silent restore must never trigger permission or chain-switch prompts.
    }
  }, [adoptAccount, clearSession]);

  const disconnect = useCallback(() => {
    const current = sessionRef.current;
    if (current?.resolution.walletId === WALLETCONNECT_ID) {
      void disconnectWalletConnect(current.resolution.provider);
    }
    clearSession(true);
  }, [clearSession]);

  useEffect(() => {
    if (typeof localStorage === "undefined") return;
    const previous = localStorage.getItem(LS_PROVIDER);
    if (!previous) return;
    if (previous === WALLETCONNECT_ID) {
      void restoreWalletConnectConnection();
      return;
    }
    const isSupported = SUPPORTED_WALLETS.some((wallet) => wallet.id === previous);
    if (!isSupported) {
      localStorage.removeItem(LS_PROVIDER);
      return;
    }
    const onProvidersChanged = () => {
      const current = sessionRef.current;
      if (current) {
        if (current.resolution.walletId !== WALLETCONNECT_ID) {
          const stillPresent = resolveEvmProvider(current.resolution.walletId, { pinned: current.resolution as ResolvedEvmProvider });
          if (!stillPresent) clearSession(true);
        }
        return;
      }
      void restoreConnection(previous);
    };
    const unsubscribe = subscribeWalletProviders(onProvidersChanged);
    void restoreConnection(previous);
    return unsubscribe;
  }, [clearSession, restoreConnection, restoreWalletConnectConnection]);

  useEffect(() => {
    void refreshBalance();
  }, [connected, refreshBalance]);

  const syncChain = useCallback(async (session: WalletSession) => {
    if (sessionRef.current?.resolution.provider !== session.resolution.provider) return;
    const cfg = loadConfig();
    let activeChain: bigint | null = null;
    try {
      activeChain = await walletChainId(session.resolution.provider);
    } catch {
      // Keep the address visible but mark the session unwritable until the provider responds.
    }
    const current = sessionRef.current;
    if (!current || current.resolution.provider !== session.resolution.provider) return;
    const next = {
      ...current.connected,
      chainId: chainNumber(activeChain),
      writable: activeChain === BigInt(cfg.evmChainId),
    };
    sessionRef.current = { resolution: current.resolution, connected: next };
    setConnected(next);
    clearQueryCache();
    if (!next.writable) setError("Switch the wallet to Injective EVM testnet before writing.");
    else setError("");
  }, []);

  useEffect(() => {
    const session = sessionRef.current;
    if (!session || !connected) return;
    const provider = session.resolution.provider;
    const sameSession = () => sessionRef.current?.resolution.provider === provider
      && sessionRef.current.resolution.walletId === session.resolution.walletId;
    const onAccountsChanged = (value: unknown) => {
      if (!sameSession()) return;
      if (Array.isArray(value) && value.length === 0) {
        clearSession(true);
        return;
      }
      const account = firstAccount(value);
      if (account) void adoptAccount(session.resolution, account, false);
      else if (session.resolution.walletId === WALLETCONNECT_ID) void restoreWalletConnectConnection();
      else void restoreConnection(session.resolution.walletId, session.resolution as ResolvedEvmProvider);
    };
    const onChainChanged = () => {
      if (sameSession()) void syncChain(session);
    };
    const onDisconnect = () => {
      if (sameSession()) clearSession(true);
    };
    const cleanups = [
      listen(provider, "accountsChanged", onAccountsChanged),
      listen(provider, "chainChanged", onChainChanged),
      listen(provider, "disconnect", onDisconnect),
    ];
    return () => cleanups.forEach((cleanup) => cleanup());
  }, [adoptAccount, clearSession, connected, restoreConnection, restoreWalletConnectConnection, syncChain]);

  return (
    <Ctx.Provider value={{
      connected,
      provider: connected ? sessionRef.current?.resolution.provider ?? null : null,
      address: connected?.address ?? "",
      balance,
      connecting,
      error,
      connect,
      connectWalletConnect,
      walletConnectUri,
      walletConnectPairing,
      walletConnectConfigured: walletConnectConfigured(),
      cancelWalletConnect,
      disconnect,
      refreshBalance,
      walletModalOpen,
      openWalletModal: () => setWalletModalOpen(true),
      closeWalletModal: () => setWalletModalOpen(false),
    }}>
      {children}
    </Ctx.Provider>
  );
}

export function useWallet(): WalletState {
  const context = useContext(Ctx);
  if (!context) throw new Error("useWallet must be used inside WalletProvider");
  return context;
}
