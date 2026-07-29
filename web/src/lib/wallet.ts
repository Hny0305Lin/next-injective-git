// Keplr wallet integration for Injective testnet.
//
// We deliberately use audited CosmJS (@cosmjs/cosmwasm-stargate) with Keplr's
// native OfflineSigner instead of @injectivelabs/sdk-ts: that package suffered
// a supply-chain key-stealer compromise (v1.20.21, 2026-07), and CosmJS keeps
// our dependency surface small. Keplr ships built-in support for injective-888
// and handles the ethsecp256k1 pubkey type internally.
import { SigningCosmWasmClient } from "@cosmjs/cosmwasm-stargate";
import type { AppConfig } from "./chain";

export const CHAIN_ID = "injective-888";

// RPC (Tendermint) endpoint CosmJS needs for broadcasting. The LCD in
// AppConfig is REST-only (used for queries); txs go through RPC.
const RPC = "https://k8s.testnet.tm.injective.network:443";

interface KeplrWindow {
  keplr?: {
    enable(chainId: string): Promise<void>;
    experimentalSuggestChain(cfg: unknown): Promise<void>;
    getOfflineSignerAuto?(chainId: string): Promise<OfflineSignerLike>;
    getOfflineSigner(chainId: string): OfflineSignerLike;
  };
}
interface OfflineSignerLike {
  getAccounts(): Promise<{ address: string }[]>;
}

function keplr() {
  const k = (window as unknown as KeplrWindow).keplr;
  if (!k) throw new Error("Keplr extension not found — install it from keplr.app");
  return k;
}

// Chain params so Keplr can add injective-888 if the user doesn't have it.
function injectiveTestnetChainInfo() {
  return {
    chainId: CHAIN_ID,
    chainName: "Injective Testnet",
    rpc: RPC,
    rest: "https://k8s.testnet.lcd.injective.network",
    bip44: { coinType: 60 }, // Injective uses the Ethereum coin type
    bech32Config: {
      bech32PrefixAccAddr: "inj",
      bech32PrefixAccPub: "injpub",
      bech32PrefixValAddr: "injvaloper",
      bech32PrefixValPub: "injvaloperpub",
      bech32PrefixConsAddr: "injvalcons",
      bech32PrefixConsPub: "injvalconspub",
    },
    currencies: [{ coinDenom: "INJ", coinMinimalDenom: "inj", coinDecimals: 18 }],
    feeCurrencies: [
      {
        coinDenom: "INJ",
        coinMinimalDenom: "inj",
        coinDecimals: 18,
        gasPriceStep: { low: 500000000, average: 700000000, high: 900000000 },
      },
    ],
    stakeCurrency: { coinDenom: "INJ", coinMinimalDenom: "inj", coinDecimals: 18 },
    features: ["ibc-transfer", "cosmwasm", "eth-address-gen", "eth-key-sign"],
  };
}

export interface Wallet {
  address: string;
  client: SigningCosmWasmClient;
}

/** Prompt Keplr to connect and return a signing client for the contract. */
export async function connectKeplr(): Promise<Wallet> {
  const k = keplr();
  try {
    await k.enable(CHAIN_ID);
  } catch {
    // chain unknown to Keplr — suggest it, then enable
    await k.experimentalSuggestChain(injectiveTestnetChainInfo());
    await k.enable(CHAIN_ID);
  }
  const signer = k.getOfflineSignerAuto
    ? await k.getOfflineSignerAuto(CHAIN_ID)
    : k.getOfflineSigner(CHAIN_ID);
  const accounts = await signer.getAccounts();
  if (accounts.length === 0) throw new Error("no account in Keplr for injective-888");

  const client = await SigningCosmWasmClient.connectWithSigner(RPC, signer as never);
  return { address: accounts[0].address, client };
}

// A fixed fee avoids depending on CosmJS' GasPrice type (which clashes across
// nested @cosmjs/stargate copies). Sponsor is a bounded-cost tx: 800k gas at
// 700000000inj/gas = 5.6e14 inj (~0.00056 INJ) is a comfortable ceiling.
const SPONSOR_FEE = {
  amount: [{ denom: "inj", amount: "560000000000000" }],
  gas: "800000",
};

/** Sponsor a repository: attach INJ funds, split happens in the contract. */
export async function sponsorWithKeplr(
  wallet: Wallet,
  cfg: AppConfig,
  owner: string,
  repo: string,
  amountInj: string,
  message: string,
): Promise<string> {
  const micro = toBaseUnits(amountInj);
  if (micro === "0") throw new Error("amount must be positive");
  const msg = { sponsor: { owner, repo, ...(message ? { message } : {}) } };
  const res = await wallet.client.execute(
    wallet.address,
    cfg.contract,
    msg,
    SPONSOR_FEE,
    message || undefined,
    [{ denom: "inj", amount: micro }],
  );
  return res.transactionHash;
}

/** Query INJ balance (base units) via the signing client. */
export async function injBalance(wallet: Wallet): Promise<string> {
  const c = await wallet.client.getBalance(wallet.address, "inj");
  return c.amount;
}

/** "0.05" -> "50000000000000000" (18 decimals), decimal-string safe. */
export function toBaseUnits(amountInj: string): string {
  const [whole, frac = ""] = amountInj.trim().split(".");
  if (frac.length > 18) throw new Error("too many decimal places (max 18)");
  const padded = (whole || "0") + frac.padEnd(18, "0");
  const trimmed = padded.replace(/^0+/, "");
  return trimmed === "" ? "0" : trimmed;
}
