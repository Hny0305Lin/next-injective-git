import { createContext, useCallback, useContext, useEffect, useState } from "react";
import {
  availableWallets,
  connectWallet,
  injBalance,
  type Wallet,
  type WalletOption,
} from "../lib/wallet";

interface WalletState {
  wallet: Wallet | null;
  address: string;
  balance: string; // base units, "" while unknown
  connecting: boolean;
  error: string;
  available: WalletOption[];
  connect: (providerId: string) => Promise<void>;
  disconnect: () => void;
  refreshBalance: () => Promise<void>;
}

const Ctx = createContext<WalletState | null>(null);
const LS_PROVIDER = "igit-wallet-provider";

export function WalletProvider({ children }: { children: React.ReactNode }) {
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [balance, setBalance] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");
  const [available, setAvailable] = useState<WalletOption[]>([]);

  // wallets inject asynchronously; re-scan for a moment after load
  useEffect(() => {
    const scan = () => setAvailable(availableWallets());
    scan();
    const t = setInterval(scan, 500);
    const stop = setTimeout(() => clearInterval(t), 3000);
    return () => {
      clearInterval(t);
      clearTimeout(stop);
    };
  }, []);

  const refreshBalance = useCallback(async () => {
    if (!wallet) return;
    try {
      setBalance(await injBalance(wallet));
    } catch {
      /* leave stale balance */
    }
  }, [wallet]);

  const connect = useCallback(async (providerId: string) => {
    setConnecting(true);
    setError("");
    try {
      const w = await connectWallet(providerId);
      setWallet(w);
      try {
        localStorage.setItem(LS_PROVIDER, providerId);
      } catch {
        /* ignore */
      }
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setConnecting(false);
    }
  }, []);

  const disconnect = useCallback(() => {
    setWallet(null);
    setBalance("");
    try {
      localStorage.removeItem(LS_PROVIDER);
    } catch {
      /* ignore */
    }
  }, []);

  // reconnect on load with the previously used wallet (still authorized)
  useEffect(() => {
    const prev = localStorage.getItem(LS_PROVIDER);
    if (prev && !wallet) connect(prev);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void refreshBalance();
  }, [refreshBalance]);

  // wallets fire this when the user switches account in the extension
  useEffect(() => {
    const onChange = () => {
      const prev = localStorage.getItem(LS_PROVIDER);
      if (prev) connect(prev);
    };
    window.addEventListener("keplr_keystorechange", onChange);
    window.addEventListener("leap_keystorechange", onChange);
    return () => {
      window.removeEventListener("keplr_keystorechange", onChange);
      window.removeEventListener("leap_keystorechange", onChange);
    };
  }, [connect]);

  return (
    <Ctx.Provider
      value={{
        wallet,
        address: wallet?.address ?? "",
        balance,
        connecting,
        error,
        available,
        connect,
        disconnect,
        refreshBalance,
      }}
    >
      {children}
    </Ctx.Provider>
  );
}

export function useWallet(): WalletState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useWallet must be used inside WalletProvider");
  return ctx;
}
