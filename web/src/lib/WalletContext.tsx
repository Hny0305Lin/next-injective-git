import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { toInjectiveAddress } from "./address";
import { clearQueryCache, ensureWalletChain, formatError, injBalanceOf, loadConfig } from "./chain";
import { getEvmProvider, subscribeWalletProviders, SUPPORTED_WALLETS } from "./wallet";

export interface Connected {
  kind: "evm";
  id: string;
  label: string;
  address: string;
  ethAddress: string;
}

interface WalletState {
  connected: Connected | null;
  address: string;
  balance: string;
  connecting: boolean;
  error: string;
  connect: (walletId: string) => Promise<boolean>;
  disconnect: () => void;
  refreshBalance: () => Promise<void>;
  walletModalOpen: boolean;
  openWalletModal: () => void;
  closeWalletModal: () => void;
}

const Ctx = createContext<WalletState | null>(null);
const LS_PROVIDER = "igit-wallet-provider";

export function WalletProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = useState<Connected | null>(null);
  const [balance, setBalance] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");
  const [walletModalOpen, setWalletModalOpen] = useState(false);
  const connectedRef = useRef<Connected | null>(null);
  connectedRef.current = connected;

  const refreshBalance = useCallback(async () => {
    const current = connectedRef.current;
    if (!current) return;
    try {
      setBalance(await injBalanceOf(loadConfig(), current.ethAddress));
    } catch {
      // Preserve the last confirmed balance while the RPC is unavailable.
    }
  }, []);

  const connect = useCallback(async (walletId: string) => {
    setConnecting(true);
    setError("");
    clearQueryCache();
    try {
      const definition = SUPPORTED_WALLETS.find((wallet) => wallet.id === walletId);
      if (!definition) throw new Error(`unsupported wallet ${walletId}`);
      const provider = getEvmProvider(walletId);
      if (!provider) throw new Error(`${definition.label} not detected; install or enable it`);
      const cfg = loadConfig();
      const accounts = await provider.request({ method: "eth_requestAccounts" });
      const ethAddress = Array.isArray(accounts) ? accounts[0] : undefined;
      if (typeof ethAddress !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(ethAddress)) {
        throw new Error("EVM wallet returned no valid account");
      }
      await ensureWalletChain(provider, cfg);
      const next: Connected = {
        kind: "evm",
        id: walletId,
        label: definition.label,
        address: toInjectiveAddress(ethAddress),
        ethAddress,
      };
      setConnected(next);
      connectedRef.current = next;
      localStorage.setItem(LS_PROVIDER, walletId);
      return true;
    } catch (cause) {
      setConnected(null);
      connectedRef.current = null;
      setBalance("");
      setError(formatError(cause));
      return false;
    } finally {
      setConnecting(false);
    }
  }, []);

  const restoreConnection = useCallback(async (walletId: string) => {
    const definition = SUPPORTED_WALLETS.find((wallet) => wallet.id === walletId);
    const provider = definition ? getEvmProvider(walletId) : undefined;
    if (!definition || !provider) return;
    try {
      const accounts = await provider.request({ method: "eth_accounts" });
      const ethAddress = Array.isArray(accounts) ? accounts[0] : undefined;
      if (typeof ethAddress !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(ethAddress)) {
        setConnected(null);
        connectedRef.current = null;
        setBalance("");
        localStorage.removeItem(LS_PROVIDER);
        return;
      }
      const next: Connected = {
        kind: "evm",
        id: walletId,
        label: definition.label,
        address: toInjectiveAddress(ethAddress),
        ethAddress,
      };
      setConnected(next);
      connectedRef.current = next;
      setError("");
    } catch {
      // Silent restore must never trigger a permission or chain-switch prompt.
    }
  }, []);

  const disconnect = useCallback(() => {
    setConnected(null);
    connectedRef.current = null;
    setBalance("");
    clearQueryCache();
    localStorage.removeItem(LS_PROVIDER);
  }, []);

  useEffect(() => {
    const previous = localStorage.getItem(LS_PROVIDER);
    if (!previous) return;
    const isSupported = SUPPORTED_WALLETS.some((wallet) => wallet.id === previous);
    if (!isSupported) {
      localStorage.removeItem(LS_PROVIDER);
      return;
    }
    const restore = () => void restoreConnection(previous);
    const unsubscribe = subscribeWalletProviders(restore);
    restore();
    return unsubscribe;
  }, [restoreConnection]);

  useEffect(() => {
    void refreshBalance();
  }, [connected, refreshBalance]);

  useEffect(() => {
    const current = connected;
    if (!current) return;
    const provider = getEvmProvider(current.id);
    if (!provider?.on || !provider.removeListener) return;
    const refreshAccount = () => {
      clearQueryCache();
      void restoreConnection(current.id);
    };
    const refreshChain = () => clearQueryCache();
    provider.on("accountsChanged", refreshAccount);
    provider.on("chainChanged", refreshChain);
    return () => {
      provider.removeListener?.("accountsChanged", refreshAccount);
      provider.removeListener?.("chainChanged", refreshChain);
    };
  }, [connected, restoreConnection]);

  return (
    <Ctx.Provider value={{
      connected,
      address: connected?.address ?? "",
      balance,
      connecting,
      error,
      connect,
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
