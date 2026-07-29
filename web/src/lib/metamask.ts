    // MetaMask (EVM) sponsorship for Injective via EIP-712.
//
// MetaMask cannot sign Cosmos SignDocs; Injective accepts EIP-712 typed-data
// signatures verified by its web3 ante handler. Building that typed data
// correctly is intricate, so we delegate it to @injectivelabs/sdk-ts
// (pinned 1.20.27 — the post-compromise clean release; 1.20.21 was the
// malicious version and is deprecated on npm). This module is loaded via
// dynamic import() so the heavy SDK stays out of the main bundle and off the
// critical path of the audited CosmJS wallet flow.
import {
  BaseAccount,
  ChainRestAuthApi,
  ChainRestTendermintApi,
  createTransaction,
  createTxRawEIP712,
  createWeb3Extension,
  getEip712TypedDataV2,
  getInjectiveAddress,
  hexToBase64,
  MsgExecuteContract,
  recoverTypedSignaturePubKey,
  SIGN_EIP712_V2,
  TxRestApi,
} from "@injectivelabs/sdk-ts";
import { ChainId, EvmChainId } from "@injectivelabs/ts-types";
import { BigNumberInBase, DEFAULT_BLOCK_TIMEOUT_HEIGHT } from "@injectivelabs/utils";
import type { AppConfig } from "./chain";

interface Eip712Window {
  ethereum?: {
    request(args: { method: string; params?: unknown[] }): Promise<unknown>;
  };
}

function metamask() {
  const eth = (window as unknown as Eip712Window).ethereum;
  if (!eth) throw new Error("MetaMask not found — install the MetaMask extension");
  return eth;
}

/** Is MetaMask (or an EVM injected provider) available? */
export function hasMetaMask(): boolean {
  return !!(window as unknown as Eip712Window).ethereum;
}

/** Connect MetaMask; returns the derived Injective (inj1) address. */
export async function connectMetaMask(): Promise<{ ethAddress: string; injectiveAddress: string }> {
  const eth = metamask();
  const accounts = (await eth.request({ method: "eth_requestAccounts" })) as string[];
  if (!accounts || accounts.length === 0) throw new Error("no MetaMask account");
  return { ethAddress: accounts[0], injectiveAddress: getInjectiveAddress(accounts[0]) };
}

/**
 * Sponsor a repo through MetaMask.
 * Flow: build MsgExecuteContract -> EIP-712 typed data -> eth_signTypedData_v4
 * -> recover pubkey from the signature -> assemble the web3-extension tx ->
 * broadcast via the REST endpoint.
 */
export async function sponsorWithMetaMask(
  cfg: AppConfig,
  owner: string,
  repo: string,
  amountInj: string,
  message: string,
): Promise<string> {
  const eth = metamask();
  const micro = new BigNumberInBase(amountInj).toWei().toFixed();
  const [ethAddress] = (await eth.request({ method: "eth_requestAccounts" })) as string[];
  const injectiveAddress = getInjectiveAddress(ethAddress);
  const rest = cfg.lcd.replace(/\/+$/, "");

  // account number + sequence
  const accountDetails = await new ChainRestAuthApi(rest).fetchAccount(injectiveAddress);
  const baseAccount = BaseAccount.fromRestApi(accountDetails);

  // timeout height (required by the web3 tx)
  const latestBlock = await new ChainRestTendermintApi(rest).fetchLatestBlock();
  const latestHeight = latestBlock.header.height;
  const timeoutHeight = new BigNumberInBase(latestHeight).plus(DEFAULT_BLOCK_TIMEOUT_HEIGHT);

  const msg = MsgExecuteContract.fromJSON({
    sender: injectiveAddress,
    contractAddress: cfg.contract,
    exec: {
      action: "sponsor",
      msg: { owner, repo, ...(message ? { message } : {}) },
    },
    funds: [{ denom: "inj", amount: micro }],
  });

  // The EIP-712 v2 "context" the chain reconstructs includes fee + memo, so the
  // signed typed data MUST carry the exact same fee + memo as the tx, or the
  // recovered signature won't match (signature verification failed).
  const fee = {
    amount: [{ denom: "inj", amount: "560000000000000" }],
    gas: "800000",
  };

  // MsgExecuteContract requires EIP-712 v2 (the chain rejects v1/amino for it).
  const eip712TypedData = getEip712TypedDataV2({
    msgs: [msg],
    tx: {
      accountNumber: baseAccount.accountNumber.toString(),
      sequence: baseAccount.sequence.toString(),
      chainId: ChainId.Testnet,
      timeoutHeight: timeoutHeight.toFixed(),
      memo: message,
    },
    fee,
    evmChainId: EvmChainId.TestnetEvm, // 1439 == Injective testnet inEVM chainId (MetaMask's active network)
  });

  // MetaMask signs the typed data
  const signature = (await eth.request({
    method: "eth_signTypedData_v4",
    params: [ethAddress, JSON.stringify(eip712TypedData)],
  })) as string;

  // recover the ethsecp256k1 pubkey from the signature (MetaMask won't expose it)
  const publicKeyHex = await recoverTypedSignaturePubKey(eip712TypedData as never, signature);
  const publicKeyBase64 = hexToBase64(publicKeyHex);

  const { txRaw } = createTransaction({
    message: msg,
    memo: message,
    signMode: SIGN_EIP712_V2, // must match the v2 typed data the user signed
    fee,
    pubKey: publicKeyBase64,
    sequence: baseAccount.sequence,
    accountNumber: baseAccount.accountNumber,
    chainId: ChainId.Testnet,
    timeoutHeight: timeoutHeight.toNumber(),
  });

  const web3Extension = createWeb3Extension({ evmChainId: EvmChainId.TestnetEvm });
  const txRawEip712 = createTxRawEIP712(txRaw, web3Extension);

  // attach the recovered signature and broadcast
  const sigBuff = Buffer.from(signature.replace(/^0x/, ""), "hex");
  txRawEip712.signatures = [sigBuff];

  const response = await new TxRestApi(rest).broadcast(txRawEip712);
  if (response.code !== 0) {
    throw new Error(`tx failed on chain (code ${response.code}): ${response.rawLog}`);
  }
  return response.txHash;
}
