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
import { configForProfile, loadConfig, saveConfig } from "../src/lib/profile.ts";
import {
  EVMReceiptUnconfirmedError,
  SuiteConfigurationError,
  SuiteVerificationError,
} from "../src/lib/errors.ts";
import {
  clearSuiteCache,
  verifySuite,
} from "../src/lib/transport.ts";
import {
  resolveRepo,
  updateRepoInfoWithEvm,
} from "../src/lib/registry.ts";
import { sponsorWithEconomicModule } from "../src/lib/modules.ts";

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
        case "eth_blockNumber": return json(BLOCK_TAG);
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
          assert.equal(tag, BLOCK_TAG);
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

test("Web runtime contains no V1/CosmWasm fallback or handwritten ABI selector table", async () => {
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
