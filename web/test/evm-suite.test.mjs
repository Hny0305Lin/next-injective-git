import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  decodeFunctionData,
  encodeFunctionResult,
  keccak256,
} from "viem";

import {
  MODULE_ABIS,
  MODULE_IDS,
  MODULE_KEYS,
  boundModuleAbi,
  directoryAbi,
} from "../src/lib/abis.ts";
import { configForProfile, isLocalDeploymentHost, loadConfig, saveConfig } from "../src/lib/profile.ts";
import {
  EVMReceiptUnconfirmedError,
  SuiteConfigurationError,
  SuiteVerificationError,
  formatWalletError,
  providerErrorCode,
} from "../src/lib/errors.ts";
import {
  clearSuiteCache,
  ensureWalletChain,
  nativeBalance,
  verifySuite,
  walletChainId,
} from "../src/lib/transport.ts";
import {
  cancelOwnershipTransferWithEvm,
  rejectOwnershipTransferWithEvm,
  resolveRepo,
  updateRepoInfoWithEvm,
} from "../src/lib/registry.ts";
import { setRevenueSplitsWithEconomicModule, sponsorWithEconomicModule } from "../src/lib/modules.ts";
import {
  EVM_ACTIVITY_BLOCK_WINDOW,
  EVM_LOG_RANGE_LIMIT,
  contractActivity,
} from "../src/lib/activity.ts";
import {
  clearDiscoveredWalletProviders,
  getEvmProvider,
  isWalletInstalled,
  prepareEvmProvider,
  requestWalletProviders,
  resolveEvmProvider,
  subscribeWalletProviders,
  SUPPORTED_WALLETS,
} from "../src/lib/wallet.ts";

const DIRECTORY = "0x0000000000000000000000000000000000000001";
const COORDINATOR = "0x0000000000000000000000000000000000000002";
const SNAPSHOT_ROOT = `0x${"ab".repeat(32)}`;
const BLOCK_TAG = "0x100";
const REPO_ID = `0x${"cd".repeat(32)}`;
const TX_HASH = `0x${"ef".repeat(32)}`;
const OWNER = "0x1111111111111111111111111111111111111111";

const MODULE_ADDRESSES = Object.fromEntries(MODULE_KEYS.map((key, index) => [
  key,
  `0x${(index + 3).toString(16).padStart(40, "0")}`,
]));
const ADDRESS_TO_MODULE = new Map(Object.entries(MODULE_ADDRESSES).map(([key, address]) => [
  address.toLowerCase(), key,
]));
const COORDINATOR_CODE = "0x6001600055";
const MODULE_CODES = Object.fromEntries(MODULE_KEYS.map((key, index) => [
  key,
  `0x60${(index + 2).toString(16).padStart(2, "0")}600055`,
]));

function json(result) {
  return new Response(JSON.stringify({ jsonrpc: "2.0", id: 1, result }), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function functionResult(abi, functionName, result) {
  return encodeFunctionResult({ abi, functionName, result });
}

function suiteFixture(options = {}) {
  const requests = [];
  const latestBlock = options.latestBlock ?? BLOCK_TAG;
  const moduleCalls = options.moduleCalls ?? (() => undefined);
  const moduleCodeHashes = Object.fromEntries(MODULE_KEYS.map((key) => [key, keccak256(MODULE_CODES[key])]));
  if (options.badCodeHashFor) moduleCodeHashes[options.badCodeHashFor] = `0x${"99".repeat(32)}`;

  function directoryCall(data) {
    const decoded = decodeFunctionData({ abi: directoryAbi, data });
    switch (decoded.functionName) {
      case "state": return functionResult(directoryAbi, decoded.functionName, 1);
      case "suiteVersion": return functionResult(directoryAbi, decoded.functionName, 3n);
      case "configuredChainId": return functionResult(directoryAbi, decoded.functionName, 1439n);
      case "snapshotRoot": return functionResult(directoryAbi, decoded.functionName, SNAPSHOT_ROOT);
      case "bootstrapCoordinator": return functionResult(directoryAbi, decoded.functionName, COORDINATOR);
      case "bootstrapCoordinatorCodeHash": {
        return functionResult(directoryAbi, decoded.functionName, keccak256(COORDINATOR_CODE));
      }
      case "registeredModuleCount": return functionResult(directoryAbi, decoded.functionName, 7n);
      case "moduleAddress": {
        const key = MODULE_KEYS.find((candidate) => MODULE_IDS[candidate] === decoded.args[0]);
        assert.ok(key);
        return functionResult(directoryAbi, decoded.functionName, MODULE_ADDRESSES[key]);
      }
      case "moduleCodeHash": {
        const key = MODULE_KEYS.find((candidate) => MODULE_IDS[candidate] === decoded.args[0]);
        assert.ok(key);
        return functionResult(directoryAbi, decoded.functionName, moduleCodeHashes[key]);
      }
      case "verifyModule": return functionResult(directoryAbi, decoded.functionName, true);
      default: throw new Error(`unexpected Directory function ${decoded.functionName}`);
    }
  }

  function moduleCall(module, data) {
    try {
      const decoded = decodeFunctionData({ abi: boundModuleAbi, data });
      switch (decoded.functionName) {
        case "suiteDirectory": {
          const value = options.badDirectoryBindingFor === module ? COORDINATOR : DIRECTORY;
          return functionResult(boundModuleAbi, decoded.functionName, value);
        }
        case "bootstrapCoordinator": return functionResult(boundModuleAbi, decoded.functionName, COORDINATOR);
        case "moduleId": return functionResult(boundModuleAbi, decoded.functionName, MODULE_IDS[module]);
        default: throw new Error(`unexpected bound function ${decoded.functionName}`);
      }
    } catch (error) {
      if (error instanceof assert.AssertionError) throw error;
    }
    const abi = MODULE_ABIS[module];
    const decoded = decodeFunctionData({ abi, data });
    const value = moduleCalls(module, decoded);
    if (value === undefined) throw new Error(`unexpected ${module}.${decoded.functionName}`);
    return functionResult(abi, decoded.functionName, value);
  }

  return {
    requests,
    async fetch(_url, init) {
      const request = JSON.parse(init.body);
      requests.push(request);
      switch (request.method) {
        case "eth_chainId": return json("0x59f");
        case "eth_blockNumber": return json(latestBlock);
        case "eth_getLogs": {
          assert.ok(options.getLogs, "fixture received an unexpected eth_getLogs");
          return options.getLogs(request.params[0], json);
        }
        case "eth_getTransactionByHash": {
          assert.ok(options.transaction, "fixture received an unexpected eth_getTransactionByHash");
          return json(options.transaction(request.params[0]));
        }
        case "eth_getTransactionReceipt": {
          assert.ok(options.receipt, "fixture received an unexpected eth_getTransactionReceipt");
          return json(options.receipt(request.params[0]));
        }
        case "eth_getBlockByNumber": {
          assert.ok(options.block, "fixture received an unexpected eth_getBlockByNumber");
          return json(options.block(request.params[0]));
        }
        case "eth_getCode": {
          const address = request.params[0].toLowerCase();
          if (address === DIRECTORY.toLowerCase()) return json("0x60006000");
          if (address === COORDINATOR.toLowerCase()) return json(COORDINATOR_CODE);
          const module = ADDRESS_TO_MODULE.get(address);
          assert.ok(module, `unexpected code address ${address}`);
          return json(MODULE_CODES[module]);
        }
        case "eth_call": {
          const [{ to, data }, tag] = request.params;
          assert.equal(tag, latestBlock);
          if (to.toLowerCase() === DIRECTORY.toLowerCase()) return json(directoryCall(data));
          const module = ADDRESS_TO_MODULE.get(to.toLowerCase());
          assert.ok(module, `unexpected call address ${to}`);
          return json(moduleCall(module, data));
        }
        default: throw new Error(`unexpected JSON-RPC method ${request.method}`);
      }
    },
  };
}

function configured() {
  return { ...configForProfile(), suiteDirectory: DIRECTORY };
}

async function withFetch(fetch, run) {
  const previous = globalThis.fetch;
  globalThis.fetch = fetch;
  clearSuiteCache();
  try {
    return await run();
  } finally {
    clearSuiteCache();
    globalThis.fetch = previous;
  }
}

test("built-in profile has one empty SuiteDirectory and ignores legacy stored fields", () => {
  const previous = globalThis.localStorage;
  const values = new Map([["igit-web-config", JSON.stringify({
    profile: "injective-testnet",
    contractVersion: "v1",
    contract: "inj1legacy",
    evmContract: OWNER,
  })]]);
  globalThis.localStorage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  };
  try {
    const cfg = loadConfig();
    assert.equal(cfg.suiteDirectory, "");
    assert.deepEqual(Object.keys(cfg).sort(), ["evmChainId", "evmRpc", "ipfsGateway", "profile", "suiteDirectory"]);
    saveConfig(cfg);
    assert.deepEqual(JSON.parse(values.get("igit-web-config")), { profile: "injective-testnet" });
  } finally {
    globalThis.localStorage = previous;
  }
});

test("an explicit local SuiteDirectory override round-trips without restoring legacy module fields", () => {
  const previous = globalThis.localStorage;
  const values = new Map([["igit-web-config", JSON.stringify({
    profile: "injective-testnet",
    suiteDirectory: DIRECTORY,
    contractVersion: "v1",
    evmContract: OWNER,
  })]]);
  globalThis.localStorage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  };
  try {
    const cfg = loadConfig();
    assert.equal(cfg.suiteDirectory, DIRECTORY);
    assert.deepEqual(Object.keys(cfg).sort(), ["evmChainId", "evmRpc", "ipfsGateway", "profile", "suiteDirectory"]);
    saveConfig(cfg);
    assert.deepEqual(JSON.parse(values.get("igit-web-config")), {
      profile: "injective-testnet",
      suiteDirectory: DIRECTORY,
    });
  } finally {
    globalThis.localStorage = previous;
  }
});

test("SuiteDirectory editing is limited to local deployment origins", () => {
  for (const host of [
    "localhost",
    "igit.localhost",
    "127.0.0.1",
    "127.1.2.3",
    "[::1]",
    "dev.local",
    "10.0.0.4",
    "192.168.1.20",
    "172.16.0.9",
    "172.31.255.255",
    "",
  ]) {
    assert.equal(isLocalDeploymentHost(host), true, `expected ${host || "(empty)"} to be local`);
  }
  for (const host of [
    "www.igit.xyz",
    "igit.xyz",
    "igit-web.vercel.app",
    "172.32.0.1",
    "172.15.0.1",
    "11.0.0.1",
    "192.169.1.1",
    "127.0.0.256",
    "localhost.attacker.example",
    "8.8.8.8",
  ]) {
    assert.equal(isLocalDeploymentHost(host), false, `expected ${host} to be public`);
  }
});

test("public Settings keeps the Directory copy-only and both write actions disabled", async () => {
  const source = await readFile(new URL("../src/features/settings/Settings.tsx", import.meta.url), "utf8");
  assert.match(source, /useState\(isLocalDeployment\)/);
  assert.match(source, /readOnly=\{!suiteEditable\}/);
  assert.equal(source.match(/(?<!aria-)disabled=\{saving \|\| !suiteEditable\}/g).length, 2);
  assert.equal(source.match(/aria-disabled=\{saving \|\| !suiteEditable\}/g).length, 2);
  for (const guarded of ["const persist = async () => {\n    if (!suiteEditable) return;", "const restoreDefaults = () => {\n    if (!suiteEditable) return;"]) {
    assert.ok(source.includes(guarded), `missing guard: ${guarded}`);
  }
});

test("wallet chain setup adds Injective testnet when the wallet does not know it", async () => {
  const requests = [];
  let switchAttempts = 0;
  const wallet = {
    async request(request) {
      requests.push(request);
      if (request.method === "wallet_switchEthereumChain") {
        switchAttempts += 1;
        if (switchAttempts === 1) throw Object.assign(new Error("unknown chain"), { code: 4902 });
        return null;
      }
      if (request.method === "wallet_addEthereumChain") return null;
      if (request.method === "eth_chainId") return "0x59f";
      throw new Error(`unexpected wallet method ${request.method}`);
    },
  };
  await ensureWalletChain(wallet, configForProfile());
  assert.deepEqual(requests.map((request) => request.method), [
    "wallet_switchEthereumChain",
    "wallet_addEthereumChain",
    "wallet_switchEthereumChain",
    "eth_chainId",
  ]);
  assert.equal(requests[1].params[0].chainId, "0x59f");
  assert.equal(requests[1].params[0].nativeCurrency.symbol, "INJ");
});

test("native INJ balance does not depend on SuiteDirectory readiness", async () => {
  const requests = [];
  await withFetch(async (_url, init) => {
    const request = JSON.parse(init.body);
    requests.push(request);
    assert.equal(request.method, "eth_getBalance");
    assert.deepEqual(request.params, [OWNER, "latest"]);
    return json("0x123");
  }, async () => {
    assert.equal(await nativeBalance(configForProfile(), OWNER), 0x123n);
  });
  assert.equal(requests.length, 1);
});

test("unconfigured Directory fails closed before any network or wallet request", async () => {
  let fetched = false;
  await withFetch(async () => {
    fetched = true;
    throw new Error("network must not be reached");
  }, async () => {
    await assert.rejects(verifySuite(configForProfile()), SuiteConfigurationError);
    await assert.rejects(
      sponsorWithEconomicModule({ request: async () => { throw new Error("wallet must not be reached"); } },
        configForProfile(), REPO_ID, "0.1", ""),
      SuiteConfigurationError,
    );
  });
  assert.equal(fetched, false);
});

test("Suite verification binds chain, active version, code hashes, and all seven modules at one block", async () => {
  const fixture = suiteFixture();
  await withFetch(fixture.fetch, async () => {
    const suite = await verifySuite(configured(), true);
    assert.equal(suite.directory, DIRECTORY);
    assert.equal(suite.coordinator, COORDINATOR);
    assert.equal(suite.snapshotRoot, SNAPSHOT_ROOT);
    assert.equal(suite.blockTag, BLOCK_TAG);
    assert.deepEqual(suite.modules, MODULE_ADDRESSES);
  });
  const calls = fixture.requests.filter((request) => request.method === "eth_call");
  assert.ok(calls.length >= 7 * 6);
  assert.ok(calls.every((request) => request.params[1] === BLOCK_TAG));
});

test("Suite verification rejects code hash and binding mismatches", async () => {
  const badHash = suiteFixture({ badCodeHashFor: "badge" });
  await withFetch(badHash.fetch, () =>
    assert.rejects(verifySuite(configured(), true), /suite module badge code hash mismatch/));

  const badBinding = suiteFixture({ badDirectoryBindingFor: "release" });
  await withFetch(badBinding.fetch, () =>
    assert.rejects(verifySuite(configured(), true), /suite module release directory binding mismatch/));
});

function activityLog(overrides = {}) {
  return {
    address: MODULE_ADDRESSES[MODULE_KEYS[0]],
    topics: [`0x${"11".repeat(32)}`],
    data: "0x",
    blockNumber: "0x30d3f",
    transactionHash: TX_HASH,
    logIndex: "0x0",
    ...overrides,
  };
}

function activityTxFixture(extra = {}) {
  return {
    transaction: (hash) => ({
      hash,
      from: OWNER,
      to: MODULE_ADDRESSES[MODULE_KEYS[0]],
      input: "0x",
      value: "0x0",
      type: "0x0",
      blockNumber: "0x30d3f",
    }),
    receipt: () => ({ status: "0x1", gasUsed: "0x5208", logs: [activityLog()] }),
    block: () => ({ timestamp: "0x66c00000" }),
    ...extra,
  };
}

test("activity log reads never exceed the RPC [from, to] block distance cap", async () => {
  const latest = 200_000n;
  const spans = [];
  const fixture = suiteFixture({
    ...activityTxFixture(),
    latestBlock: `0x${latest.toString(16)}`,
    getLogs: (filter, json) => {
      spans.push({ from: BigInt(filter.fromBlock), to: BigInt(filter.toBlock) });
      return json([]);
    },
  });

  const events = await withFetch(fixture.fetch, () => contractActivity(configured(), 50));
  assert.deepEqual(events, []);

  const window = BigInt(EVM_ACTIVITY_BLOCK_WINDOW);
  const cap = BigInt(EVM_LOG_RANGE_LIMIT);
  assert.ok(spans.length > 1, "the window must be split into multiple bounded spans");
  for (const span of spans) {
    assert.ok(span.to - span.from < cap, `span ${span.from}-${span.to} exceeds the ${cap} block cap`);
    assert.ok(span.from <= span.to, `span ${span.from}-${span.to} is inverted`);
  }
  // Newest first, contiguous, and covering exactly the documented window.
  assert.equal(spans[0].to, latest);
  assert.equal(spans[spans.length - 1].from, latest - window);
  for (let index = 1; index < spans.length; index += 1) {
    assert.equal(spans[index].to, spans[index - 1].from - 1n, "spans must be contiguous");
  }
});

test("activity stops requesting older spans once the limit is filled", async () => {
  const spans = [];
  const fixture = suiteFixture({
    ...activityTxFixture(),
    latestBlock: "0x30d40",
    getLogs: (filter, json) => {
      spans.push(filter);
      return json(spans.length === 1 ? [activityLog()] : []);
    },
  });

  const events = await withFetch(fixture.fetch, () => contractActivity(configured(), 1));
  assert.equal(events.length, 1);
  assert.equal(events[0].txhash, TX_HASH);
  assert.equal(events[0].height, "199999");
  assert.equal(spans.length, 1, "a filled limit must not keep scanning older spans");
});

test("activity halves a span when the provider still reports a range cap", async () => {
  const accepted = [];
  const fixture = suiteFixture({
    ...activityTxFixture(),
    latestBlock: "0x2710",
    getLogs: (filter, json) => {
      const from = BigInt(filter.fromBlock);
      const to = BigInt(filter.toBlock);
      if (to - from > 2_500n) {
        return new Response(JSON.stringify({
          jsonrpc: "2.0",
          id: 1,
          error: { code: -32000, message: "maximum [from, to] blocks distance: 10000" },
        }), { status: 200, headers: { "content-type": "application/json" } });
      }
      accepted.push({ from, to });
      return json([]);
    },
  });

  const events = await withFetch(fixture.fetch, () => contractActivity(configured(), 50));
  assert.deepEqual(events, []);
  assert.ok(accepted.length >= 4, "a rejected span must be split and retried");
  for (const span of accepted) {
    assert.ok(span.to - span.from <= 2_500n, `retry span ${span.from}-${span.to} was not narrowed`);
  }
  // The split must still cover the whole window without gaps.
  const ordered = [...accepted].sort((left, right) => (left.from < right.from ? -1 : 1));
  assert.equal(ordered[0].from, 0n);
  assert.equal(ordered[ordered.length - 1].to, 10_000n);
});

test("activity surfaces non-range RPC failures instead of splitting forever", async () => {
  let calls = 0;
  const fixture = suiteFixture({
    ...activityTxFixture(),
    latestBlock: "0x2710",
    getLogs: () => {
      calls += 1;
      return new Response(JSON.stringify({
        jsonrpc: "2.0",
        id: 1,
        error: { code: -32000, message: "rate limit exceeded for this key" },
      }), { status: 200, headers: { "content-type": "application/json" } });
    },
  });

  await withFetch(fixture.fetch, async () => {
    await assert.rejects(contractActivity(configured(), 50), /rate limit exceeded/);
  });
  assert.equal(calls, 1, "an unrelated RPC error must not trigger range splitting");
});

test("registry reads use viem tuple decoding and stable repo IDs", async () => {
  const fixture = suiteFixture({
    moduleCalls(module, decoded) {
      if (module === "core" && decoded.functionName === "resolveRepository") {
        assert.equal(decoded.args[0], OWNER);
        assert.equal(decoded.args[1], "demo");
        return [{
          id: REPO_ID,
          owner: OWNER,
          name: "demo",
          description: "suite repo",
          defaultBranch: "main",
          forkedFrom: `0x${"00".repeat(32)}`,
          createdAt: 10n,
          updatedAt: 20n,
          exists: true,
        }, false];
      }
      if (module === "moderation" && decoded.functionName === "effectiveStatus") {
        assert.equal(decoded.args[0], REPO_ID);
        return 1;
      }
      return undefined;
    },
  });
  await withFetch(fixture.fetch, async () => {
    const result = await resolveRepo(configured(), OWNER, "demo");
    assert.equal(result.repoId, REPO_ID);
    assert.equal(result.isCanonical, false);
    assert.equal(result.info.moderation_status, "frozen");
    assert.equal(result.info.description, "suite repo");
    assert.match(result.info.owner, /^inj1/);
  });
});

function walletFixture({ receiptStatus = "0x1" } = {}) {
  const requests = [];
  return {
    requests,
    async request(request) {
      requests.push(request);
      switch (request.method) {
        case "wallet_switchEthereumChain": return null;
        case "eth_chainId": return "0x59f";
        case "eth_requestAccounts": return [OWNER];
        case "eth_gasPrice": return "0x1";
        case "eth_estimateGas": return "0x5208";
        case "eth_sendTransaction": return TX_HASH;
        case "eth_getTransactionReceipt": return { status: receiptStatus, transactionHash: TX_HASH };
        default: throw new Error(`unexpected wallet method ${request.method}`);
      }
    },
  };
}

test("wallet writes estimate gas and broadcast an explicit legacy transaction at Injective minimum gasPrice", async () => {
  const fixture = suiteFixture();
  const wallet = walletFixture();
  await withFetch(fixture.fetch, async () => {
    const hash = await sponsorWithEconomicModule(wallet, configured(), REPO_ID, "0.1", "thanks");
    assert.equal(hash, TX_HASH);
  });
  const estimate = wallet.requests.find((request) => request.method === "eth_estimateGas").params[0];
  const sent = wallet.requests.find((request) => request.method === "eth_sendTransaction").params[0];
  assert.equal(estimate.to, MODULE_ADDRESSES.economic);
  assert.equal(estimate.gasPrice, "0x9896800");
  assert.equal(estimate.value, "0x16345785d8a0000");
  assert.equal(sent.gas, "0x99e8");
  assert.equal(sent.type, "0x0");
  assert.equal(sent.gasPrice, "0x9896800");
  const decoded = decodeFunctionData({ abi: MODULE_ABIS.economic, data: sent.data });
  assert.equal(decoded.functionName, "sponsor");
  assert.deepEqual(decoded.args, [REPO_ID, "thanks"]);
});

test("revenue split writes accept the V1 twenty-recipient limit and reject twenty-one", async () => {
  const fixture = suiteFixture();
  const wallet = walletFixture();
  const splits = Array.from({ length: 20 }, (_, index) => ({
    address: `0x${(index + 0x9000).toString(16).padStart(40, "0")}`,
    bps: 1,
  }));
  await withFetch(fixture.fetch, async () => {
    await setRevenueSplitsWithEconomicModule(wallet, configured(), REPO_ID, splits);
    await assert.rejects(
      setRevenueSplitsWithEconomicModule(wallet, configured(), REPO_ID, [...splits, {
        address: `0x${"a000".padStart(40, "0")}`,
        bps: 1,
      }]),
      /at most 20 recipients/,
    );
  });
  const sent = wallet.requests.find((request) => request.method === "eth_sendTransaction").params[0];
  const decoded = decodeFunctionData({ abi: MODULE_ABIS.economic, data: sent.data });
  assert.equal(decoded.functionName, "setRevenueSplits");
  assert.equal(decoded.args[1].length, 20);
  assert.equal(decoded.args[2].length, 20);
});

test("metadata writes encode V3 repoId patch flags instead of a locator", async () => {
  const fixture = suiteFixture();
  const wallet = walletFixture();
  await withFetch(fixture.fetch, async () => {
    await updateRepoInfoWithEvm(wallet, configured(), REPO_ID, { description: "changed" });
  });
  const sent = wallet.requests.find((request) => request.method === "eth_sendTransaction").params[0];
  assert.equal(sent.to, MODULE_ADDRESSES.core);
  const decoded = decodeFunctionData({ abi: MODULE_ABIS.core, data: sent.data });
  assert.equal(decoded.functionName, "updateMetadata");
  assert.deepEqual(decoded.args, [REPO_ID, true, "changed", false, ""]);
});

test("ownership-transfer rejection uses RepositoryCore cancellation", async () => {
  const fixture = suiteFixture();
  const wallet = walletFixture();
  await withFetch(fixture.fetch, async () => {
    await cancelOwnershipTransferWithEvm(wallet, configured(), REPO_ID);
    await rejectOwnershipTransferWithEvm(wallet, configured(), REPO_ID);
  });
  const sent = wallet.requests
    .filter((request) => request.method === "eth_sendTransaction")
    .map((request) => request.params[0]);
  assert.equal(sent.length, 2);
  assert.equal(sent[0].to, MODULE_ADDRESSES.core);
  assert.equal(sent[0].data, sent[1].data);
  const decoded = decodeFunctionData({ abi: MODULE_ABIS.core, data: sent[1].data });
  assert.equal(decoded.functionName, "cancelOwnershipTransfer");
  assert.deepEqual(decoded.args, [REPO_ID]);
});

test("a mined status zero receipt is a failure and never falls back", async () => {
  const fixture = suiteFixture();
  const wallet = walletFixture({ receiptStatus: "0x0" });
  await withFetch(fixture.fetch, async () => {
    await assert.rejects(
      sponsorWithEconomicModule(wallet, configured(), REPO_ID, "0.1", ""),
      /EVM transaction reverted/,
    );
  });
  assert.equal(wallet.requests.filter((request) => request.method === "eth_sendTransaction").length, 1);
});

test("receipt transport failures return a typed uncertain error carrying the tx hash", async () => {
  const fixture = suiteFixture();
  const wallet = walletFixture();
  wallet.request = async function request(value) {
    this.requests.push(value);
    if (value.method === "eth_getTransactionReceipt") throw Object.assign(new Error("offline"), { code: -32000 });
    switch (value.method) {
      case "wallet_switchEthereumChain": return null;
      case "eth_chainId": return "0x59f";
      case "eth_requestAccounts": return [OWNER];
      case "eth_gasPrice": return "0x1";
      case "eth_estimateGas": return "0x5208";
      case "eth_sendTransaction": return TX_HASH;
      default: throw new Error(`unexpected wallet method ${value.method}`);
    }
  };
  await withFetch(fixture.fetch, async () => {
    await assert.rejects(
      sponsorWithEconomicModule(wallet, configured(), REPO_ID, "0.1", ""),
      (error) => error instanceof EVMReceiptUnconfirmedError && error.txHash === TX_HASH,
    );
  });
});

test("ordinary Web runtime contains no V1 fallback or handwritten ABI selector table", async () => {
  const paths = [
    "../src/lib/chain.ts",
    "../src/lib/profile.ts",
    "../src/lib/transport.ts",
    "../src/lib/registry.ts",
    "../src/lib/modules.ts",
    "../src/lib/wallet.ts",
  ];
  const source = (await Promise.all(paths.map((path) => readFile(new URL(path, import.meta.url), "utf8")))).join("\n");
  assert.doesNotMatch(source, /contractVersion|legacyClient|smartQuery|wasm\/v1|MsgExecuteContract/);
  assert.doesNotMatch(source, /SELECTORS\s*=|abiWord\(|dynamicOffset\(/);
  assert.match(source, /encodeFunctionData/);
  assert.match(source, /decodeFunctionResult/);
});

test("wallet connect is independent from Suite verification and keeps failures visible", async () => {
  const walletContext = await readFile(new URL("../src/lib/WalletContext.tsx", import.meta.url), "utf8");
  const walletModal = await readFile(new URL("../src/components/WalletModal.tsx", import.meta.url), "utf8");
  const requestAccounts = walletContext.indexOf('await provider.request({ method: "eth_requestAccounts" })');
  const switchChain = walletContext.indexOf("await ensureWalletChain(provider, cfg)");
  assert.ok(requestAccounts >= 0 && requestAccounts < switchChain);
  assert.match(walletContext, /await ensureWalletChain\(provider, cfg\)/);
  assert.doesNotMatch(walletContext, /await verifySuite\(cfg\)/);
  assert.match(walletContext, /method: "eth_accounts"/);
  assert.match(walletModal, /if \(await connect\(w\.id\)\) close\(\)/);
  assert.match(walletModal, /const close = \(\) => \{[\s\S]*?onClose\(\);[\s\S]*?\};/);
  assert.match(walletModal, /Scan with a supported EVM wallet/);
  assert.match(walletModal, /supports Injective EVM testnet \(chain 1439\)/);
  assert.match(walletModal, /Keplr Mobile does not currently list chain 1439 for EVM WalletConnect/);
  assert.doesNotMatch(walletModal, /WalletConnect-compatible Android wallet/);
  assert.match(
    walletModal,
    /<QrCode size=\{17\} \/>\s*<IconifyIcon className="walletconnect-trigger-brand" icon="thesvg-color:walletconnect"[\s\S]*?<span>Scan with WalletConnect<\/span>/,
  );
  assert.doesNotMatch(walletModal, /walletconnect-brand/);
  assert.match(walletModal, /QRCodeSVG/);
  assert.match(walletModal, /walletconnect-entry/);
  assert.match(walletContext, /connectWalletConnect/);
  assert.match(walletContext, /restoreWalletConnect/);
  assert.match(walletContext, /WalletConnect pairing failed/);
});

test("WalletConnect uses a custom URI surface and preserves the EIP-1193 session boundary", async () => {
  const source = await readFile(new URL("../src/lib/walletconnect.ts", import.meta.url), "utf8");
  assert.match(source, /showQrModal: false/);
  assert.match(source, /chains: \[cfg\.evmChainId\]/);
  assert.doesNotMatch(source, /optionalChains/);
  assert.match(source, /display_uri/);
  assert.match(source, /provider\.session/);
  assert.match(source, /pairingPromise/);
  assert.match(source, /cleanupPendingPairings/);
  assert.doesNotMatch(source, /localStorage/);
});

test("WalletConnect chain incompatibility remains actionable and does not expose raw logger objects", async () => {
  const walletContext = await readFile(new URL("../src/lib/WalletContext.tsx", import.meta.url), "utf8");
  assert.match(walletContext, /wallet may not support Injective EVM testnet \(chain 1439\)/);
  assert.doesNotMatch(walletContext, /JSON\.stringify\(cause\)/);
});

test("wallet chain IDs accept numeric WalletConnect responses and hex injected responses", async () => {
  assert.equal(await walletChainId({ request: async () => 1439 }), 1439n);
  assert.equal(await walletChainId({ request: async () => "0x59f" }), 1439n);
});

test("WalletConnect QR entry is hidden at the existing mobile breakpoint", async () => {
  const styles = await readFile(new URL("../src/styles.css", import.meta.url), "utf8");
  assert.match(styles, /@media \(max-width: 600px\)[\s\S]*?\.walletconnect-entry\s*\{\s*display: none;/);
});

test("WalletConnect brand icon is bundled for offline rendering", async () => {
  const icons = await readFile(new URL("../src/lib/wallet-icons.ts", import.meta.url), "utf8");
  assert.match(icons, /prefix: "thesvg-color"/);
  assert.match(icons, /walletconnect:\s*\{[\s\S]*?fill=\\?"#3b99fc\\?"/);
});

test("EIP-6963 announcements are matched by wallet RDNS", async () => {
  const previous = globalThis.window;
  const browserWindow = new EventTarget();
  const provider = { request: async () => [] };
  browserWindow.addEventListener("eip6963:requestProvider", () => {
    const announcement = new Event("eip6963:announceProvider");
    Object.defineProperty(announcement, "detail", { value: {
      info: { uuid: "rabby-test", name: "Rabby", icon: "data:image/svg+xml,", rdns: "io.rabby" },
      provider,
    } });
    browserWindow.dispatchEvent(announcement);
  });
  globalThis.window = browserWindow;
  try {
    const wallet = await import("../src/lib/wallet.ts");
    let updates = 0;
    const unsubscribe = wallet.subscribeWalletProviders(() => { updates += 1; });
    assert.equal(wallet.getEvmProvider("rabby"), provider);
    assert.equal(wallet.isWalletInstalled("rabby"), true);
    assert.ok(updates >= 1);
    unsubscribe();
  } finally {
    globalThis.window = previous;
  }
});

test("late EIP-6963 announcements update availability without replacing a selected family", () => {
  const previous = globalThis.window;
  const browserWindow = new EventTarget();
  globalThis.window = browserWindow;
  try {
    requestWalletProviders();
    assert.equal(isWalletInstalled("okxevm"), false);
    let updates = 0;
    const unsubscribe = subscribeWalletProviders(() => { updates += 1; });
    const provider = { request: async () => [] };
    const announcement = new Event("eip6963:announceProvider");
    Object.defineProperty(announcement, "detail", { value: {
      info: { uuid: "okx-late", name: "OKX Wallet", icon: "data:image/svg+xml,", rdns: "com.okex.wallet" },
      provider,
    } });
    browserWindow.dispatchEvent(announcement);
    assert.equal(getEvmProvider("okxevm"), provider);
    assert.ok(updates >= 1);
    unsubscribe();
    clearDiscoveredWalletProviders();
  } finally {
    globalThis.window = previous;
  }
});

test("provider error codes normalize numeric, string, and nested EIP-1193 shapes", () => {
  assert.equal(providerErrorCode({ code: 4001 }), 4001);
  assert.equal(providerErrorCode({ code: "4100" }), 4100);
  assert.equal(providerErrorCode({ error: { data: { code: "4200" } } }), 4200);
  assert.equal(providerErrorCode({ cause: { originalError: { code: "-32002" } } }), -32002);
  assert.equal(providerErrorCode({ data: { message: "no code" } }), undefined);
  assert.match(formatWalletError({ error: { code: "4001" } }), /rejected/i);
  assert.match(formatWalletError({ data: { code: "-32002" } }), /already pending/i);
});

test("wallet chain setup only adds a chain for normalized unknown-chain errors", async () => {
  const requests = [];
  const wallet = {
    async request(request) {
      requests.push(request);
      if (request.method === "wallet_switchEthereumChain") {
        throw { error: { code: "4001" } };
      }
      throw new Error(`unexpected wallet method ${request.method}`);
    },
  };
  await assert.rejects(ensureWalletChain(wallet, configForProfile()), (error) => providerErrorCode(error) === 4001);
  assert.deepEqual(requests.map((request) => request.method), ["wallet_switchEthereumChain"]);
});

test("EIP-6963 resolution covers every announced EVM wallet and pins provider identity", async () => {
  const previous = globalThis.window;
  const browserWindow = new EventTarget();
  const eipWallets = SUPPORTED_WALLETS.filter((wallet) => !["keplr", "compass"].includes(wallet.id));
  const providers = new Map();
  for (const wallet of eipWallets) {
    const provider = { request: async () => [] };
    providers.set(wallet.id, provider);
  }
  browserWindow.addEventListener("eip6963:requestProvider", () => {
    const rdns = {
      metamask: "io.metamask",
      rabby: "io.rabby",
      okxevm: "com.okex.wallet",
      bitget: "com.bitget.web3",
      trust: "com.trustwallet.app",
      coinbase: "com.coinbase.wallet",
      gate: "io.gate.wallet",
      brave: "com.brave.wallet",
    };
    for (const wallet of eipWallets) {
      const announcement = new Event("eip6963:announceProvider");
      Object.defineProperty(announcement, "detail", { value: {
        info: { uuid: `${wallet.id}-uuid`, name: wallet.label, icon: "data:image/svg+xml,", rdns: rdns[wallet.id] },
        provider: providers.get(wallet.id),
      } });
      browserWindow.dispatchEvent(announcement);
    }
  });
  globalThis.window = browserWindow;
  try {
    let updates = 0;
    const unsubscribe = (await import("../src/lib/wallet.ts")).subscribeWalletProviders(() => { updates += 1; });
    for (const wallet of eipWallets) {
      const resolution = resolveEvmProvider(wallet.id, { requireUnique: true });
      assert.equal(resolution?.provider, providers.get(wallet.id));
      assert.equal(isWalletInstalled(wallet.id), true);
    }
    assert.ok(updates >= eipWallets.length);

    const selected = resolveEvmProvider("rabby");
    assert.ok(selected);
    const replacement = { request: async () => [] };
    const changed = new Event("eip6963:announceProvider");
    Object.defineProperty(changed, "detail", { value: {
      info: { uuid: "rabby-uuid", name: "Rabby", icon: "data:image/svg+xml,", rdns: "io.rabby" },
      provider: replacement,
    } });
    browserWindow.dispatchEvent(changed);
    assert.equal(resolveEvmProvider("rabby", { pinned: selected }), undefined);
    assert.equal(getEvmProvider("rabby"), replacement);
    unsubscribe();
    clearDiscoveredWalletProviders();
  } finally {
    globalThis.window = previous;
  }
});

test("same-brand provider ambiguity is deterministic for connect but blocked for silent restore", () => {
  const previous = globalThis.window;
  const browserWindow = new EventTarget();
  globalThis.window = browserWindow;
  try {
    requestWalletProviders();
    const first = { request: async () => [] };
    const second = { request: async () => [] };
    for (const [uuid, provider] of [["rabby-z", second], ["rabby-a", first]]) {
      const announcement = new Event("eip6963:announceProvider");
      Object.defineProperty(announcement, "detail", { value: {
        info: { uuid, name: "Rabby", icon: "data:image/svg+xml,", rdns: "io.rabby" },
        provider,
      } });
      browserWindow.dispatchEvent(announcement);
    }
    assert.equal(resolveEvmProvider("rabby")?.provider, first);
    assert.equal(resolveEvmProvider("rabby", { requireUnique: true }), undefined);
  } finally {
    clearDiscoveredWalletProviders();
    globalThis.window = previous;
  }
});

test("legacy fallback never chooses an arbitrary provider when a vendor flag is ambiguous", () => {
  const previous = globalThis.window;
  const first = { request: async () => [] , isMetaMask: true };
  const second = { request: async () => [] , isMetaMask: true };
  globalThis.window = Object.assign(new EventTarget(), { ethereum: { providers: [first, second] } });
  try {
    assert.equal(getEvmProvider("metamask"), undefined);
    globalThis.window = Object.assign(new EventTarget(), { ethereum: first });
    assert.equal(getEvmProvider("metamask"), first);
    const keplrProvider = { request: async () => [], on() {}, off() {} };
    globalThis.window = Object.assign(new EventTarget(), { keplr: { ethereum: keplrProvider } });
    assert.equal(getEvmProvider("keplr"), keplrProvider);
    const compassProvider = { request: async () => [] };
    globalThis.window = Object.assign(new EventTarget(), { compassEvm: compassProvider });
    assert.equal(getEvmProvider("compass"), compassProvider);
    const gateProvider = { request: async () => [], isGateWallet: true, isMetaMask: true };
    globalThis.window = Object.assign(new EventTarget(), { ethereum: gateProvider, gatewallet: gateProvider });
    assert.equal(getEvmProvider("gate"), gateProvider);
    assert.equal(getEvmProvider("metamask"), undefined);
  } finally {
    clearDiscoveredWalletProviders();
    globalThis.window = previous;
  }
});

test("wallet metadata uses the requested brand icons and keeps Compass unchanged", () => {
  assert.deepEqual(Object.fromEntries(SUPPORTED_WALLETS.map(({ id, icon }) => [id, icon])), {
    metamask: "token-branded:metamask",
    rabby: "token-branded:rabby",
    okxevm: "token-branded:okx",
    bitget: "token-branded:bitget",
    trust: "token-branded:trust",
    coinbase: "token-branded:coinbase",
    gate: "token-branded:gate-io",
    brave: "thesvg-color:brave",
    keplr: "token-branded:keplr",
    compass: "CP",
  });
});

test("Keplr EVM preparation uses only the documented provider enable hook", async () => {
  const calls = [];
  const provider = {
    async enable() { calls.push("enable"); },
    async request() { calls.push("request"); return []; },
  };
  await prepareEvmProvider({ walletId: "keplr", provider, identity: { source: "legacy" } });
  assert.deepEqual(calls, ["enable"]);
  await prepareEvmProvider({ walletId: "metamask", provider, identity: { source: "legacy" } });
  assert.deepEqual(calls, ["enable"]);
});

test("wallet context pins events to the selected provider and marks wrong chains unwritable", async () => {
  const walletContext = await readFile(new URL("../src/lib/WalletContext.tsx", import.meta.url), "utf8");
  const walletSource = await readFile(new URL("../src/lib/wallet.ts", import.meta.url), "utf8");
  assert.match(walletContext, /resolveEvmProvider\(walletId, \{ pinned: resolution \}\)/);
  assert.match(walletContext, /accountsChanged/);
  assert.match(walletContext, /chainChanged/);
  assert.match(walletContext, /disconnect/);
  assert.match(walletContext, /writable:/);
  assert.match(walletContext, /removeListener/);
  assert.match(walletContext, /provider\.off/);
  assert.doesNotMatch(walletContext, /getEvmProvider\(/);
  assert.match(walletSource, /keplr\?\.ethereum/);
  assert.match(walletSource, /compassEvm/);
  assert.doesNotMatch(walletSource, /keplr\.enable\(/);
});
