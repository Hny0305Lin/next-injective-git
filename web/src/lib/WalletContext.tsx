import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { connectKeplr, injBalance, type Wallet } from "../lib/wallet";

interface WalletState {
  wallet: Wallet | null;
  address: string;
  balance: string; // base units, "" while unknown
  connecting: boolean;
  error: string;
  connect: () => Promise<void>;
  disconnect: () => void;
  refreshBalance: () => Promise<void>;
}

const Ctx = createContext<WalletState | null>(null);

export function WalletProvider({ children }: { children: React.ReactNode }) {
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [balance, setBalance] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");

  const refreshBalance = useCallback(async () => {
    if (!wallet) return;
    try {
      setBalance(await injBalance(wallet));
    } catch {
      /* leave stale balance */
    }
  }, [wallet]);

  const connect = useCallback(async () => {
    setConnecting(true);
    setError("");
    try {
      const w = await connectKeplr();
      setWallet(w);
      try {
        localStorage.setItem("igit-wallet-autoconnect", "1");
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
      localStorage.removeItem("igit-wallet-autoconnect");
    } catch {
      /* ignore */
    }
  }, []);

  // reconnect on load if the user connected before (Keplr stays authorized)
  useEffect(() => {
    if (localStorage.getItem("igit-wallet-autoconnect") === "1" && !wallet) {
      connect();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void refreshBalance();
  }, [refreshBalance]);

  // Keplr fires this when the user switches account in the extension
  useEffect(() => {
    const onChange = () => {
      if (localStorage.getItem("igit-wallet-autoconnect") === "1") connect();
    };
    window.addEventListener("keplr_keystorechange", onChange);
    return () => window.removeEventListener("keplr_keystorechange", onChange);
  }, [connect]);

  return (
    <Ctx.Provider
      value={{
        wallet,
        address: wallet?.address ?? "",
        balance,
        connecting,
        error,
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
