// Keplr wallet integration for Injective testnet.
//
// We deliberately use audited CosmJS + cosmjs-types with Keplr's native
// signing instead of @injectivelabs/sdk-ts: that package suffered a
// supply-chain key-stealer compromise (v1.20.21, 2026-07). Injective is not a
// vanilla Cosmos chain, so two things need custom handling that plain CosmJS
// gets wrong:
//   1. accounts use /injective.types.v1beta1.EthAccount (not BaseAccount)
//   2. pubkeys are /injective.crypto.v1beta1.ethsecp256k1.PubKey, and the
//      inj1 address is keccak-derived from it — CosmJS hardcodes cosmos
//      secp256k1, which would make on-chain signature verification fail.
// We therefore build the SignDoc ourselves with the correct pubkey type and
// let Keplr sign the bytes; the client is only used to broadcast.
import { SigningCosmWasmClient } from "@cosmjs/cosmwasm-stargate";
import { fromBase64, toUtf8 } from "@cosmjs/encoding";
import { BaseAccount } from "cosmjs-types/cosmos/auth/v1beta1/auth";
import { PubKey as Secp256k1PubKey } from "cosmjs-types/cosmos/crypto/secp256k1/keys";
import { SignMode } from "cosmjs-types/cosmos/tx/signing/v1beta1/signing";
import { AuthInfo, TxBody, TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import { MsgExecuteContract } from "cosmjs-types/cosmwasm/wasm/v1/tx";
import { Any } from "cosmjs-types/google/protobuf/any";
import { BinaryReader } from "cosmjs-types/binary";
import type { AppConfig } from "./chain";

export const CHAIN_ID = "injective-888";

const ETH_PUBKEY_TYPE = "/injective.crypto.v1beta1.ethsecp256k1.PubKey";

// RPC (Tendermint) endpoint CosmJS needs for broadcasting. The LCD in
// AppConfig is REST-only (used for queries); txs go through RPC.
const RPC = "https://k8s.testnet.tm.injective.network:443";

interface KeplrKey {
  bech32Address: string;
  pubKey: Uint8Array;
}
interface KeplrSignDoc {
  bodyBytes: Uint8Array;
  authInfoBytes: Uint8Array;
  chainId: string;
  accountNumber: bigint;
}
interface KeplrDirectResponse {
  signed: KeplrSignDoc;
  signature: { signature: string };
}
// Cosmos wallets (Keplr, Leap, OKX, Cosmostation, ...) all expose the same
// keplr-compatible injection API, so one code path serves them all.
interface KeplrLike {
  enable(chainId: string): Promise<void>;
  experimentalSuggestChain(cfg: unknown): Promise<void>;
  getOfflineSignerAuto?(chainId: string): Promise<OfflineSignerLike>;
  getOfflineSigner(chainId: string): OfflineSignerLike;
  getKey(chainId: string): Promise<KeplrKey>;
  signDirect(chainId: string, signer: string, signDoc: KeplrSignDoc): Promise<KeplrDirectResponse>;
}
interface OfflineSignerLike {
  getAccounts(): Promise<{ address: string }[]>;
}

// Where each wallet injects its keplr-compatible object on window.
const PROVIDERS: { id: string; label: string; get: () => KeplrLike | undefined }[] = [
  { id: "keplr", label: "Keplr", get: () => win().keplr },
  { id: "leap", label: "Leap", get: () => win().leap },
  { id: "okx", label: "OKX Wallet", get: () => win().okxwallet?.keplr },
  { id: "cosmostation", label: "Cosmostation", get: () => win().cosmostation?.providers?.keplr },
];

interface WalletWindow {
  keplr?: KeplrLike;
  leap?: KeplrLike;
  okxwallet?: { keplr?: KeplrLike };
  cosmostation?: { providers?: { keplr?: KeplrLike } };
}
function win(): WalletWindow {
  return window as unknown as WalletWindow;
}

export interface WalletOption {
  id: string;
  label: string;
}

// Every wallet the app supports, for the picker UI (installed or not).
// kind "cosmos" wallets sign via CosmJS; "evm" (MetaMask) via EIP-712.
export interface SupportedWallet {
  id: string;
  label: string;
  kind: "cosmos" | "evm";
  icon: string; // emoji stand-in (no image assets needed)
  installUrl: string;
}

export const SUPPORTED_WALLETS: SupportedWallet[] = [
  { id: "keplr", label: "Keplr", kind: "cosmos", icon: "\u{1F311}", installUrl: "https://www.keplr.app/download" },
  { id: "leap", label: "Leap", kind: "cosmos", icon: "\u{1F998}", installUrl: "https://www.leapwallet.io/download" },
  { id: "okx", label: "OKX Wallet", kind: "cosmos", icon: "\u2B21", installUrl: "https://www.okx.com/web3" },
  { id: "cosmostation", label: "Cosmostation", kind: "cosmos", icon: "\u2728", installUrl: "https://www.cosmostation.io/wallet" },
  // EVM wallets — all sign through the same Injective EIP-712 path proven with
  // MetaMask. They must be on the Injective testnet inEVM network (chainId
  // 1439); the sponsor flow prompts a switch/add automatically.
  { id: "metamask", label: "MetaMask", kind: "evm", icon: "\u{1F98A}", installUrl: "https://metamask.io/download/" },
  { id: "rabby", label: "Rabby", kind: "evm", icon: "\u{1F430}", installUrl: "https://rabby.io/" },
  { id: "okxevm", label: "OKX Wallet (EVM)", kind: "evm", icon: "\u2B22", installUrl: "https://www.okx.com/web3" },
  { id: "bitget", label: "Bitget Wallet", kind: "evm", icon: "\u{1F7E6}", installUrl: "https://web3.bitget.com/" },
  { id: "trust", label: "Trust Wallet", kind: "evm", icon: "\u{1F6E1}\uFE0F", installUrl: "https://trustwallet.com/" },
  { id: "coinbase", label: "Coinbase Wallet", kind: "evm", icon: "\u{1F535}", installUrl: "https://www.coinbase.com/wallet" },
  { id: "brave", label: "Brave Wallet", kind: "evm", icon: "\u{1F981}", installUrl: "https://brave.com/wallet/" },
];

// A minimal EIP-1193 provider (what every EVM wallet injects).
export interface Eip1193 {
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
}

// Some browsers expose several injected providers under window.ethereum.providers;
// pick the one whose brand flag matches.
function injectedByFlag(flag: string): Eip1193 | undefined {
  const eth = (window as unknown as { ethereum?: { providers?: Record<string, unknown>[] } & Record<string, unknown> }).ethereum;
  if (!eth) return undefined;
  const list = Array.isArray(eth.providers) ? eth.providers : undefined;
  if (list) {
    const hit = list.find((p) => p[flag]);
    if (hit) return hit as unknown as Eip1193;
  }
  return eth[flag] ? (eth as unknown as Eip1193) : undefined;
}

/** Resolve the EIP-1193 provider for an EVM wallet id (undefined if absent). */
export function getEvmProvider(id: string): Eip1193 | undefined {
  const w = window as unknown as Record<string, { ethereum?: Eip1193 } & Eip1193>;
  switch (id) {
    case "metamask":
      return injectedByFlag("isMetaMask");
    case "rabby":
      return (w.rabby as Eip1193) ?? injectedByFlag("isRabby");
    case "okxevm":
      return w.okxwallet as unknown as Eip1193;
    case "bitget":
      return (w.bitkeep?.ethereum as Eip1193) ?? injectedByFlag("isBitKeep");
    case "trust":
      return (w.trustwallet as Eip1193) ?? injectedByFlag("isTrust") ?? injectedByFlag("isTrustWallet");
    case "coinbase":
      return (w.coinbaseWalletExtension as Eip1193) ?? injectedByFlag("isCoinbaseWallet");
    case "brave":
      return injectedByFlag("isBraveWallet");
    default:
      return undefined;
  }
}

const EVM_IDS = new Set(SUPPORTED_WALLETS.filter((w) => w.kind === "evm").map((w) => w.id));

/** Whether a supported wallet's extension is currently injected. */
export function isWalletInstalled(id: string): boolean {
  if (EVM_IDS.has(id)) {
    return !!getEvmProvider(id);
  }
  const p = PROVIDERS.find((x) => x.id === id);
  const w = p?.get();
  return !!w && typeof w.getKey === "function" && typeof w.signDirect === "function";
}

/** Wallets whose extension is actually installed and exposes the needed API. */
export function availableWallets(): WalletOption[] {
  return PROVIDERS.filter((p) => {
    const w = p.get();
    return !!w && typeof w.getKey === "function" && typeof w.signDirect === "function";
  }).map(({ id, label }) => ({ id, label }));
}

function providerObject(id: string): KeplrLike {
  const provider = PROVIDERS.find((p) => p.id === id);
  const w = provider?.get();
  if (!w) {
    throw new Error(`${provider?.label ?? id} not found — install its browser extension`);
  }
  return w;
}

// CosmJS only understands the standard cosmos BaseAccount. Injective wraps it
// in a custom /injective.types.v1beta1.EthAccount (field 1 = base_account),
// which makes the default parser throw "Unsupported type". Unwrap it so the
// signing client can read account_number / sequence.
interface ParsedAccount {
  address: string;
  pubkey: null; // the offline signer supplies the pubkey; not needed from chain
  accountNumber: number;
  sequence: number;
}

function injectiveAccountParser(input: { typeUrl: string; value: Uint8Array }): ParsedAccount {
  let baseBytes: Uint8Array;
  if (input.typeUrl === "/injective.types.v1beta1.EthAccount") {
    baseBytes = extractBaseAccountBytes(input.value);
  } else if (input.typeUrl === "/cosmos.auth.v1beta1.BaseAccount") {
    baseBytes = input.value;
  } else {
    throw new Error(`unsupported account type: ${input.typeUrl}`);
  }
  const base = BaseAccount.decode(baseBytes);
  return {
    address: base.address,
    pubkey: null,
    accountNumber: Number(base.accountNumber),
    sequence: Number(base.sequence),
  };
}

// Read field 1 (base_account, wire type 2) out of an EthAccount protobuf.
function extractBaseAccountBytes(value: Uint8Array): Uint8Array {
  const reader = new BinaryReader(value);
  while (reader.pos < reader.len) {
    const tag = reader.uint32();
    if (tag >>> 3 === 1 && (tag & 7) === 2) {
      return reader.bytes();
    }
    reader.skipType(tag & 7);
  }
  throw new Error("EthAccount is missing base_account");
}

// Chain params so a wallet can add injective-888 if the user doesn't have it.
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
  providerId: string;
  providerLabel: string;
  provider: KeplrLike;
}

/** Connect a specific Cosmos wallet and return a signing client. */
export async function connectWallet(providerId: string): Promise<Wallet> {
  const label = PROVIDERS.find((p) => p.id === providerId)?.label ?? providerId;
  const k = providerObject(providerId);
  try {
    await k.enable(CHAIN_ID);
  } catch {
    // chain unknown to the wallet — suggest it, then enable
    await k.experimentalSuggestChain(injectiveTestnetChainInfo());
    await k.enable(CHAIN_ID);
  }
  const signer = k.getOfflineSignerAuto
    ? await k.getOfflineSignerAuto(CHAIN_ID)
    : k.getOfflineSigner(CHAIN_ID);
  const accounts = await signer.getAccounts();
  if (accounts.length === 0) throw new Error(`no account in ${label} for injective-888`);

  const client = await SigningCosmWasmClient.connectWithSigner(RPC, signer as never, {
    accountParser: injectiveAccountParser as never,
  });
  return { address: accounts[0].address, client, providerId, providerLabel: label, provider: k };
}

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

  const k = wallet.provider;
  const key = await k.getKey(CHAIN_ID);
  const { accountNumber, sequence } = await fetchAccount(cfg, wallet.address);

  // message body: MsgExecuteContract carrying the JSON sponsor msg + funds
  const execJson = { sponsor: { owner, repo, ...(message ? { message } : {}) } };
  const msgValue = MsgExecuteContract.encode(
    MsgExecuteContract.fromPartial({
      sender: wallet.address,
      contract: cfg.contract,
      msg: toUtf8(JSON.stringify(execJson)),
      funds: [{ denom: "inj", amount: micro }],
    }),
  ).finish();
  const bodyBytes = TxBody.encode(
    TxBody.fromPartial({
      messages: [{ typeUrl: "/cosmwasm.wasm.v1.MsgExecuteContract", value: msgValue }],
      memo: message || "",
    }),
  ).finish();

  // authInfo with the *Injective* pubkey type (ethsecp256k1), not cosmos secp256k1
  const pubkey = Any.fromPartial({
    typeUrl: ETH_PUBKEY_TYPE,
    value: Secp256k1PubKey.encode({ key: key.pubKey }).finish(),
  });
  const authInfoBytes = AuthInfo.encode(
    AuthInfo.fromPartial({
      signerInfos: [
        {
          publicKey: pubkey,
          modeInfo: { single: { mode: SignMode.SIGN_MODE_DIRECT } },
          sequence: BigInt(sequence),
        },
      ],
      fee: {
        amount: [{ denom: "inj", amount: "560000000000000" }],
        gasLimit: BigInt(800000),
      },
    }),
  ).finish();

  const signDoc = { bodyBytes, authInfoBytes, chainId: CHAIN_ID, accountNumber: BigInt(accountNumber) };
  const { signed, signature } = await k.signDirect(CHAIN_ID, wallet.address, signDoc);
  const txBytes = TxRaw.encode(
    TxRaw.fromPartial({
      bodyBytes: signed.bodyBytes,
      authInfoBytes: signed.authInfoBytes,
      signatures: [fromBase64(signature.signature)],
    }),
  ).finish();

  const res = await wallet.client.broadcastTx(txBytes);
  if (res.code !== 0) {
    throw new Error(`tx failed on chain (code ${res.code}): ${res.rawLog}`);
  }
  return res.transactionHash;
}

// Fetch account_number + sequence from the LCD REST endpoint, which returns
// JSON and handles the EthAccount wrapper natively (no protobuf parsing).
async function fetchAccount(
  cfg: AppConfig,
  address: string,
): Promise<{ accountNumber: string; sequence: string }> {
  const url = `${cfg.lcd.replace(/\/+$/, "")}/cosmos/auth/v1beta1/accounts/${address}`;
  const resp = await fetch(url);
  if (!resp.ok) {
    throw new Error(
      `cannot read account ${address} (HTTP ${resp.status}). Fund it with test INJ first.`,
    );
  }
  const json = await resp.json();
  // EthAccount: { account: { base_account: { account_number, sequence } } }
  // BaseAccount: { account: { account_number, sequence } }
  const acc = json.account ?? {};
  const base = acc.base_account ?? acc;
  return {
    accountNumber: String(base.account_number ?? "0"),
    sequence: String(base.sequence ?? "0"),
  };
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
