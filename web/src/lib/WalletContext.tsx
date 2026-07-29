import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { connectWallet, type Wallet } from "../lib/wallet";
import { injBalanceOf, loadConfig, formatError } from "../lib/chain";

// A connected wallet is either a Cosmos wallet (signs via CosmJS) or an EVM
// wallet (MetaMask, signs via EIP-712). Both expose an inj1 address.
export type Connected =
  | { kind: "cosmos"; id: string; label: string; address: string; cosmos: Wallet }
  | { kind: "evm"; id: string; label: string; address: string; ethAddress: string };

interface WalletState {
  connected: Connected | null;
  address: string; // inj1 address of the connected wallet, "" if none
  balance: string; // base units, "" while unknown
  connecting: boolean;
  error: string;
  connect: (walletId: string) => Promise<void>;
  disconnect: () => void;
  refreshBalance: () => Promise<void>;
}

const Ctx = createContext<WalletState | null>(null);
const LS_PROVIDER = "igit-wallet-provider";

export function WalletProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = useState<Connected | null>(null);
  const [balance, setBalance] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");

  const refreshBalance = useCallback(async () => {
    if (!connected) return;
    try {
      setBalance(await injBalanceOf(loadConfig(), connected.address));
    } catch {
      /* leave stale balance */
    }
  }, [connected]);

  const connect = useCallback(async (walletId: string) => {
    setConnecting(true);
    setError("");
    try {
      if (walletId === "metamask") {
        // dynamic import keeps the heavy Injective SDK out of the main bundle
        const mm = await import("../lib/metamask");
        const { ethAddress, injectiveAddress } = await mm.connectMetaMask();
        setConnected({
          kind: "evm",
          id: "metamask",
          label: "MetaMask",
          address: injectiveAddress,
          ethAddress,
        });
      } else {
        const w = await connectWallet(walletId);
        setConnected({
          kind: "cosmos",
          id: walletId,
          label: w.providerLabel,
          address: w.address,
          cosmos: w,
        });
      }
      try {
        localStorage.setItem(LS_PROVIDER, walletId);
      } catch {
        /* ignore */
      }
    } catch (e) {
      console.error("[wallet] connect failed:", e);
      setError(formatError(e));
    } finally {
      setConnecting(false);
    }
  }, []);

  const disconnect = useCallback(() => {
    setConnected(null);
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
    if (prev && !connected) connect(prev);
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
        connected,
        address: connected?.address ?? "",
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
