import type { Eip1193 } from "./transport";

export type { Eip1193 } from "./transport";

export interface SupportedWallet {
  id: string;
  label: string;
  icon: string;
  installUrl: string;
}

export const SUPPORTED_WALLETS: SupportedWallet[] = [
  { id: "metamask", label: "MetaMask", icon: "MM", installUrl: "https://metamask.io/download/" },
  { id: "rabby", label: "Rabby", icon: "RB", installUrl: "https://rabby.io/" },
  { id: "okxevm", label: "OKX Wallet", icon: "OK", installUrl: "https://www.okx.com/web3" },
  { id: "bitget", label: "Bitget Wallet", icon: "BG", installUrl: "https://web3.bitget.com/" },
  { id: "trust", label: "Trust Wallet", icon: "TW", installUrl: "https://trustwallet.com/" },
  { id: "coinbase", label: "Coinbase Wallet", icon: "CB", installUrl: "https://www.coinbase.com/wallet" },
  { id: "brave", label: "Brave Wallet", icon: "BW", installUrl: "https://brave.com/wallet/" },
];

interface ProviderRecord extends Eip1193 {
  providers?: ProviderRecord[];
  isMetaMask?: boolean;
  isRabby?: boolean;
  isBitKeep?: boolean;
  isTrust?: boolean;
  isTrustWallet?: boolean;
  isCoinbaseWallet?: boolean;
  isBraveWallet?: boolean;
}

interface WalletWindow {
  ethereum?: ProviderRecord;
  rabby?: ProviderRecord;
  okxwallet?: ProviderRecord;
  bitkeep?: { ethereum?: ProviderRecord };
  trustwallet?: ProviderRecord;
  coinbaseWalletExtension?: ProviderRecord;
}

interface Eip6963ProviderInfo {
  uuid: string;
  name: string;
  icon: string;
  rdns: string;
}

interface Eip6963ProviderDetail {
  info: Eip6963ProviderInfo;
  provider: ProviderRecord;
}

const EIP6963_RDNS: Record<string, readonly string[]> = {
  metamask: ["io.metamask"],
  rabby: ["io.rabby"],
  okxevm: ["com.okex.wallet"],
  bitget: ["com.bitget.web3", "com.bitkeep.wallet"],
  trust: ["com.trustwallet.app"],
  coinbase: ["com.coinbase.wallet"],
  brave: ["com.brave.wallet"],
};

const announcedProviders = new Map<string, Eip6963ProviderDetail>();
const providerSubscribers = new Set<() => void>();
let discoveryStarted = false;

function isProviderDetail(value: unknown): value is Eip6963ProviderDetail {
  if (!value || typeof value !== "object") return false;
  const detail = value as Partial<Eip6963ProviderDetail>;
  return typeof detail.info?.uuid === "string"
    && typeof detail.info.rdns === "string"
    && typeof detail.provider?.request === "function";
}

function announceProvider(event: Event): void {
  const detail = (event as CustomEvent<unknown>).detail;
  if (!isProviderDetail(detail)) return;
  const previous = announcedProviders.get(detail.info.uuid);
  if (previous?.provider === detail.provider && previous.info.rdns === detail.info.rdns) return;
  announcedProviders.set(detail.info.uuid, detail);
  providerSubscribers.forEach((subscriber) => subscriber());
}

function startProviderDiscovery(): void {
  if (discoveryStarted || typeof window === "undefined") return;
  discoveryStarted = true;
  window.addEventListener("eip6963:announceProvider", announceProvider as EventListener);
}

export function requestWalletProviders(): void {
  if (typeof window === "undefined") return;
  startProviderDiscovery();
  window.dispatchEvent(new Event("eip6963:requestProvider"));
}

export function subscribeWalletProviders(subscriber: () => void): () => void {
  startProviderDiscovery();
  providerSubscribers.add(subscriber);
  requestWalletProviders();
  return () => providerSubscribers.delete(subscriber);
}

function walletWindow(): WalletWindow {
  return window as unknown as WalletWindow;
}

function injectedByFlag(flag: keyof ProviderRecord): Eip1193 | undefined {
  const ethereum = walletWindow().ethereum;
  if (!ethereum) return undefined;
  const providers = Array.isArray(ethereum.providers) ? ethereum.providers : [ethereum];
  return providers.find((provider) => provider[flag] === true);
}

function announcedByRdns(id: string): Eip1193 | undefined {
  const accepted = EIP6963_RDNS[id];
  if (!accepted) return undefined;
  return Array.from(announcedProviders.values()).find(({ info }) => (
    accepted.includes(info.rdns.toLowerCase())
  ))?.provider;
}

export function getEvmProvider(id: string): Eip1193 | undefined {
  startProviderDiscovery();
  const announced = announcedByRdns(id);
  if (announced) return announced;
  const wallet = walletWindow();
  switch (id) {
    case "metamask": return injectedByFlag("isMetaMask");
    case "rabby": return wallet.rabby ?? injectedByFlag("isRabby");
    case "okxevm": return wallet.okxwallet;
    case "bitget": return wallet.bitkeep?.ethereum ?? injectedByFlag("isBitKeep");
    case "trust": return wallet.trustwallet ?? injectedByFlag("isTrust") ?? injectedByFlag("isTrustWallet");
    case "coinbase": return wallet.coinbaseWalletExtension ?? injectedByFlag("isCoinbaseWallet");
    case "brave": return injectedByFlag("isBraveWallet");
    default: return undefined;
  }
}

export function isWalletInstalled(id: string): boolean {
  return getEvmProvider(id) !== undefined;
}
