import type { Eip1193 } from "./transport";

export type { Eip1193 } from "./transport";

export interface SupportedWallet {
  id: string;
  label: string;
  icon: string;
  installUrl: string;
}

export const SUPPORTED_WALLETS: SupportedWallet[] = [
  { id: "metamask", label: "MetaMask", icon: "token-branded:metamask", installUrl: "https://metamask.io/download/" },
  { id: "rabby", label: "Rabby", icon: "token-branded:rabby", installUrl: "https://rabby.io/" },
  { id: "okxevm", label: "OKX Wallet (EVM)", icon: "token-branded:okx", installUrl: "https://www.okx.com/web3" },
  { id: "bitget", label: "Bitget Wallet", icon: "token-branded:bitget", installUrl: "https://web3.bitget.com/" },
  { id: "trust", label: "Trust Wallet", icon: "token-branded:trust", installUrl: "https://trustwallet.com/" },
  { id: "coinbase", label: "Coinbase Wallet", icon: "token-branded:coinbase", installUrl: "https://www.coinbase.com/wallet" },
  { id: "gate", label: "Gate Wallet", icon: "token-branded:gate-io", installUrl: "https://chromewebstore.google.com/detail/gate-wallet/cpmkedoipcpimgecpmgpldfpohjplkpp" },
  { id: "brave", label: "Brave Wallet", icon: "thesvg-color:brave", installUrl: "https://brave.com/wallet/" },
  { id: "keplr", label: "Keplr (EVM)", icon: "token:keplr", installUrl: "https://www.keplr.app/download" },
  { id: "compass", label: "Compass (Leap EVM)", icon: "CP", installUrl: "https://chrome.google.com/webstore/detail/compass-wallet/anokgmphncpekkhclmingpimjmcooifb" },
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
  isGateWallet?: boolean;
}

interface WalletWindow {
  ethereum?: ProviderRecord;
  keplr?: { ethereum?: ProviderRecord };
  compassEvm?: ProviderRecord;
  rabby?: ProviderRecord;
  okxwallet?: ProviderRecord;
  bitkeep?: { ethereum?: ProviderRecord };
  trustwallet?: ProviderRecord;
  coinbaseWalletExtension?: ProviderRecord;
  gatewallet?: ProviderRecord;
}

export interface Eip6963ProviderInfo {
  uuid: string;
  name: string;
  icon: string;
  rdns: string;
}

export type EvmProviderSource = "eip6963" | "legacy";

export interface EvmProviderIdentity {
  source: EvmProviderSource;
  uuid?: string;
  rdns?: string;
}

export interface ResolvedEvmProvider {
  walletId: string;
  provider: Eip1193;
  identity: EvmProviderIdentity;
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
  gate: ["io.gate.wallet"],
  brave: ["com.brave.wallet"],
  // Compass may announce via EIP-6963; its exact RDNS is intentionally not
  // guessed here until an extension announcement is reviewed.
};

const announcedProviders = new Map<string, Eip6963ProviderDetail>();
const providerSubscribers = new Set<() => void>();
let discoveryWindow: Window | null = null;

function isProviderDetail(value: unknown): value is Eip6963ProviderDetail {
  if (!value || typeof value !== "object") return false;
  const detail = value as Partial<Eip6963ProviderDetail>;
  return typeof detail.info?.uuid === "string"
    && detail.info.uuid.length > 0
    && typeof detail.info.rdns === "string"
    && detail.info.rdns.length > 0
    && typeof detail.provider?.request === "function";
}

function announceProvider(event: Event): void {
  const detail = (event as CustomEvent<unknown>).detail;
  if (!isProviderDetail(detail)) return;
  const previous = announcedProviders.get(detail.info.uuid);
  if (previous?.provider === detail.provider && previous.info.rdns.toLowerCase() === detail.info.rdns.toLowerCase()) return;
  announcedProviders.set(detail.info.uuid, detail);
  providerSubscribers.forEach((subscriber) => subscriber());
}

function startProviderDiscovery(): void {
  if (typeof window === "undefined") return;
  if (discoveryWindow === window) return;
  if (discoveryWindow) {
    discoveryWindow.removeEventListener("eip6963:announceProvider", announceProvider as EventListener);
  }
  announcedProviders.clear();
  discoveryWindow = window;
  discoveryWindow.addEventListener("eip6963:announceProvider", announceProvider as EventListener);
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

function walletWindow(): WalletWindow | undefined {
  if (typeof window === "undefined") return undefined;
  return window as unknown as WalletWindow;
}

function usableProvider(value: unknown): value is Eip1193 {
  return Boolean(value && typeof (value as Eip1193).request === "function");
}

function injectedByFlag(
  flag: keyof ProviderRecord,
  excludedFlags: readonly (keyof ProviderRecord)[] = [],
): Eip1193 | undefined {
  const ethereum = walletWindow()?.ethereum;
  if (!ethereum) return undefined;
  const providers = Array.isArray(ethereum.providers) ? ethereum.providers : [ethereum];
  const matches = providers.filter((provider) => provider[flag] === true
    && excludedFlags.every((excluded) => provider[excluded] !== true)
    && usableProvider(provider));
  // A shared vendor flag is not enough to choose between two extensions.
  return matches.length === 1 ? matches[0] : undefined;
}

function legacyProvider(id: string): Eip1193 | undefined {
  const wallet = walletWindow();
  if (!wallet) return undefined;
  switch (id) {
    case "metamask": return injectedByFlag("isMetaMask", ["isGateWallet"]);
    case "rabby": return (usableProvider(wallet.rabby) ? wallet.rabby : undefined) ?? injectedByFlag("isRabby");
    case "okxevm": return usableProvider(wallet.okxwallet) ? wallet.okxwallet : undefined;
    case "bitget": return (usableProvider(wallet.bitkeep?.ethereum) ? wallet.bitkeep.ethereum : undefined) ?? injectedByFlag("isBitKeep");
    case "trust": return (usableProvider(wallet.trustwallet) ? wallet.trustwallet : undefined) ?? injectedByFlag("isTrust") ?? injectedByFlag("isTrustWallet");
    case "coinbase": return (usableProvider(wallet.coinbaseWalletExtension) ? wallet.coinbaseWalletExtension : undefined) ?? injectedByFlag("isCoinbaseWallet");
    case "gate": return (usableProvider(wallet.gatewallet) ? wallet.gatewallet : undefined) ?? injectedByFlag("isGateWallet");
    case "brave": return injectedByFlag("isBraveWallet");
    case "keplr": return usableProvider(wallet.keplr?.ethereum) ? wallet.keplr.ethereum : undefined;
    case "compass": return usableProvider(wallet.compassEvm) ? wallet.compassEvm : undefined;
    default: return undefined;
  }
}

function acceptedRdns(id: string): readonly string[] {
  return EIP6963_RDNS[id] ?? [];
}

function announcedForWallet(id: string): Eip6963ProviderDetail[] {
  const accepted = acceptedRdns(id);
  return Array.from(announcedProviders.values())
    .filter(({ info }) => accepted.includes(info.rdns.toLowerCase()))
    .sort((left, right) => left.info.uuid.localeCompare(right.info.uuid));
}

export interface ResolveEvmProviderOptions {
  /** Require exactly one EIP-6963 match when restoring a persisted session. */
  requireUnique?: boolean;
  /** Keep an existing in-memory provider pinned to its UUID and object. */
  pinned?: ResolvedEvmProvider;
}

export function resolveEvmProvider(
  id: string,
  options: ResolveEvmProviderOptions = {},
): ResolvedEvmProvider | undefined {
  startProviderDiscovery();
  const matches = announcedForWallet(id);
  const accepted = acceptedRdns(id);

  if (options.pinned) {
    const pinned = options.pinned;
    if (pinned.walletId !== id) return undefined;
    if (pinned.identity.source === "eip6963") {
      if (!pinned.identity.uuid) return undefined;
      const record = announcedProviders.get(pinned.identity.uuid);
      if (!record || record.provider !== pinned.provider) return undefined;
      if (!accepted.includes(record.info.rdns.toLowerCase())) return undefined;
      return {
        walletId: id,
        provider: record.provider,
        identity: { source: "eip6963", uuid: record.info.uuid, rdns: record.info.rdns.toLowerCase() },
      };
    }
    const current = legacyProvider(id);
    return current === pinned.provider ? pinned : undefined;
  }

  if (matches.length > 0) {
    if (options.requireUnique && matches.length !== 1) return undefined;
    const selected = matches[0];
    return {
      walletId: id,
      provider: selected.provider,
      identity: { source: "eip6963", uuid: selected.info.uuid, rdns: selected.info.rdns.toLowerCase() },
    };
  }

  const provider = legacyProvider(id);
  if (!provider) return undefined;
  return { walletId: id, provider, identity: { source: "legacy" } };
}

export async function prepareEvmProvider(resolution: ResolvedEvmProvider): Promise<void> {
  if (resolution.walletId !== "keplr") return;
  const enable = (resolution.provider as Eip1193 & { enable?: () => Promise<void> }).enable;
  if (enable) await enable.call(resolution.provider);
}

/** Compatibility helper for read-only detection and older callers. */
export function getEvmProvider(id: string): Eip1193 | undefined {
  return resolveEvmProvider(id)?.provider;
}

export function isWalletInstalled(id: string): boolean {
  return resolveEvmProvider(id) !== undefined;
}

/** Test/HMR helper; it does not revoke permissions in a wallet extension. */
export function clearDiscoveredWalletProviders(): void {
  announcedProviders.clear();
  providerSubscribers.forEach((subscriber) => subscriber());
}
