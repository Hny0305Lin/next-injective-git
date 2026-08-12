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

function walletWindow(): WalletWindow {
  return window as unknown as WalletWindow;
}

function injectedByFlag(flag: keyof ProviderRecord): Eip1193 | undefined {
  const ethereum = walletWindow().ethereum;
  if (!ethereum) return undefined;
  const providers = Array.isArray(ethereum.providers) ? ethereum.providers : [ethereum];
  return providers.find((provider) => provider[flag] === true);
}

export function getEvmProvider(id: string): Eip1193 | undefined {
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
