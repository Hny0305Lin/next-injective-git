import assert from "node:assert/strict";
import test from "node:test";
import { encodeAbiParameters, encodeEventTopics, parseAbiParameters } from "viem";

import { coreAbi, moderationAbi } from "../src/lib/abis.ts";
import { toInjectiveAddress } from "../src/lib/address.ts";
import {
  absorbRepos,
  applyRepoEvent,
  decodeRepoEvent,
  parseIndex,
  repoIndexStorageKey,
  searchRepoEntries,
  serializeIndex,
} from "../src/lib/repo-index.ts";

const OWNER_A = "0x1111111111111111111111111111111111111111";
const OWNER_B = "0x2222222222222222222222222222222222222222";
const ZERO_BYTES32 = "0x0000000000000000000000000000000000000000000000000000000000000000";
const REPO_ID = "0x" + "ab".repeat(32);
const OTHER_REPO_ID = "0x" + "cd".repeat(32);

function makeLog({ abi, eventName, args, blockNumber = 100, logIndex = 0, removed = false }) {
  const topics = encodeEventTopics({ abi, eventName, args });
  let data = "0x";
  if (eventName === "RepositoryCreated") {
    data = encodeAbiParameters(parseAbiParameters("string"), [args.name]);
  } else if (eventName === "OwnershipTransferred") {
    data = encodeAbiParameters(parseAbiParameters("string, bool"), [args.name, args.recovered]);
  } else if (eventName === "RepositoryStatusSet") {
    data = encodeAbiParameters(parseAbiParameters("uint8, string"), [args.status, args.reasonHash]);
  }
  return {
    address: "0x" + "33".repeat(20),
    topics,
    data,
    blockNumber: `0x${blockNumber.toString(16)}`,
    transactionHash: "0x" + "c1".repeat(32),
    logIndex: `0x${logIndex.toString(16)}`,
    ...(removed ? { removed } : {}),
  };
}

function shard() {
  return { suiteDirectory: "0x" + "aa".repeat(20), suiteVersion: 4, entries: new Map() };
}

function entry(repoId, name, createdBlock, status = 0) {
  return {
    repoId,
    suiteDirectory: "0x" + "aa".repeat(20),
    suiteVersion: 4,
    owner: "inj1owner",
    name,
    status,
    blockNumber: createdBlock,
    logIndex: 0,
    createdBlock,
  };
}

test("decodeRepoEvent decodes creation, transfer, and moderation logs", () => {
  const created = decodeRepoEvent(makeLog({
    abi: coreAbi,
    eventName: "RepositoryCreated",
    args: { repoId: REPO_ID, owner: OWNER_A, name: "demo-repo", forkedFrom: ZERO_BYTES32 },
  }));
  assert.equal(created.kind, "created");
  assert.equal(created.repoId, REPO_ID);
  assert.equal(created.owner, toInjectiveAddress(OWNER_A));
  assert.equal(created.name, "demo-repo");
  assert.equal(created.blockNumber, 100);
  assert.equal(created.logIndex, 0);

  const transferred = decodeRepoEvent(makeLog({
    abi: coreAbi,
    eventName: "OwnershipTransferred",
    args: { repoId: REPO_ID, oldOwner: OWNER_A, newOwner: OWNER_B, name: "demo-repo", recovered: false },
    blockNumber: 150,
  }));
  assert.equal(transferred.kind, "owner");
  assert.equal(transferred.owner, toInjectiveAddress(OWNER_B));
  assert.equal(transferred.name, "demo-repo");

  const status = decodeRepoEvent(makeLog({
    abi: moderationAbi,
    eventName: "RepositoryStatusSet",
    args: { repoId: REPO_ID, status: 1, updatedBy: OWNER_A, reasonHash: "" },
    blockNumber: 160,
  }));
  assert.equal(status.kind, "status");
  assert.equal(status.status, 1);
});

test("decodeRepoEvent ignores unrelated suite events and empty topics", () => {
  const unrelated = decodeRepoEvent(makeLog({
    abi: coreAbi,
    eventName: "RepositoryMetadataUpdated",
    args: { repoId: REPO_ID, owner: OWNER_A, fieldMask: 1, description: "d", defaultBranch: "main" },
  }));
  assert.equal(unrelated, null);
  const emptyTopics = makeLog({
    abi: coreAbi,
    eventName: "RepositoryCreated",
    args: { repoId: REPO_ID, owner: OWNER_A, name: "x", forkedFrom: ZERO_BYTES32 },
  });
  assert.equal(decodeRepoEvent({ ...emptyTopics, topics: [] }), null);
});

test("applyRepoEvent folds events in order and ignores stale or orphan updates", () => {
  const state = shard();
  applyRepoEvent(state, { kind: "created", repoId: REPO_ID, owner: "inj-a", name: "demo-repo", blockNumber: 100, logIndex: 0 });
  applyRepoEvent(state, { kind: "owner", repoId: REPO_ID, owner: "inj-b", name: "renamed", blockNumber: 90, logIndex: 5 });
  assert.equal(state.entries.get(REPO_ID).owner, "inj-a");
  assert.equal(state.entries.get(REPO_ID).name, "demo-repo");

  applyRepoEvent(state, { kind: "owner", repoId: REPO_ID, owner: "inj-b", name: "", blockNumber: 110, logIndex: 0 });
  assert.equal(state.entries.get(REPO_ID).owner, "inj-b");
  assert.equal(state.entries.get(REPO_ID).name, "demo-repo");

  applyRepoEvent(state, { kind: "status", repoId: OTHER_REPO_ID, status: 1, blockNumber: 120, logIndex: 0 });
  assert.equal(state.entries.size, 1);

  applyRepoEvent(state, { kind: "status", repoId: REPO_ID, status: 1, blockNumber: 120, logIndex: 0 });
  assert.equal(state.entries.get(REPO_ID).status, 1);
});

test("searchRepoEntries ranks exact over prefix over substring and hides delisted", () => {
  const entries = [
    entry("0x" + "01".repeat(32), "demo", 100),
    entry("0x" + "02".repeat(32), "demo-extra", 90),
    entry("0x" + "03".repeat(32), "my-demo", 80),
    entry("0x" + "04".repeat(32), "hidden-demo", 70, 2),
  ];
  assert.deepEqual(searchRepoEntries(entries, "demo").map((item) => item.name), ["demo", "demo-extra", "my-demo"]);
  assert.deepEqual(searchRepoEntries(entries, "DEMO ").map((item) => item.name), ["demo", "demo-extra", "my-demo"]);
  assert.equal(searchRepoEntries(entries, "d").length, 0);
  assert.equal(searchRepoEntries(entries, "  ").length, 0);
  assert.equal(searchRepoEntries(entries, "nothing").length, 0);
  assert.equal(searchRepoEntries(entries, "demo", 1).length, 1);
});

test("searchRepoEntries supports owner/name queries against inj addresses", () => {
  const entries = [entry(REPO_ID, "demo", 100)];
  assert.equal(searchRepoEntries(entries, "inj1own/demo").length, 1);
  assert.equal(searchRepoEntries(entries, "inj1zzz/demo").length, 0);
  assert.equal(searchRepoEntries(entries, "inj1own/missing").length, 0);
});

test("index storage roundtrips through serializeIndex and parseIndex", () => {
  const state = shard();
  applyRepoEvent(state, { kind: "created", repoId: REPO_ID, owner: "inj-a", name: "demo-repo", blockNumber: 10, logIndex: 0 });
  const absorbed = new Map();
  absorbRepos(absorbed, [{ owner: "inj-b", name: "absorbed-repo", moderation: "active" }], 3);
  const raw = serializeIndex({
    shards: [{
      directory: state.suiteDirectory,
      suiteVersion: state.suiteVersion,
      cursor: 50,
      walkNext: 49,
      walkFloor: 0,
      entries: state.entries,
    }],
    absorbed,
  });
  const parsed = parseIndex(raw);
  assert.equal(parsed.shards.length, 1);
  assert.equal(parsed.shards[0].directory, state.suiteDirectory);
  assert.equal(parsed.shards[0].cursor, 50);
  assert.equal(parsed.shards[0].entries.length, 1);
  assert.equal(parsed.shards[0].entries[0].name, "demo-repo");
  assert.equal(parsed.shards[0].entries[0].suiteDirectory, state.suiteDirectory);
  assert.equal(parsed.absorbed.length, 1);
  assert.equal(parsed.absorbed[0].name, "absorbed-repo");
  assert.equal(parsed.absorbed[0].suiteDirectory, "");

  assert.equal(parseIndex("not json"), null);
  assert.equal(parseIndex(null), null);
  assert.equal(parseIndex(JSON.stringify({ version: 999, shards: [] })), null);
  assert.equal(repoIndexStorageKey(1439), "igit.repo-index.v1.1439");
});
test("absorbRepos maps moderation status and refreshes existing entries", () => {
  const absorbed = new Map();
  assert.equal(absorbRepos(absorbed, [
    { owner: "inj-a", name: "alpha", moderation: "active" },
    { owner: "inj-a", name: "beta", moderation: "frozen" },
  ], 4), 2);
  assert.equal(absorbed.get("inj-a/alpha/v4").repoId, null);
  assert.equal(absorbed.get("inj-a/alpha/v4").status, 0);
  assert.equal(absorbed.get("inj-a/beta/v4").status, 1);
  assert.equal(absorbRepos(absorbed, [{ owner: "inj-a", name: "alpha", moderation: "delisted" }], 4), 0);
  assert.equal(absorbed.get("inj-a/alpha/v4").status, 2);
  assert.equal(absorbed.size, 2);
});

test("absorbed repositories are searchable but delisted ones stay hidden", () => {
  const absorbed = new Map();
  absorbRepos(absorbed, [
    { owner: "inj-a", name: "alpha-repo", moderation: "active" },
    { owner: "inj-a", name: "ghost-repo", moderation: "delisted" },
  ], 4);
  const results = searchRepoEntries([...absorbed.values()], "repo");
  assert.deepEqual(results.map((item) => item.name), ["alpha-repo"]);
});
test("explorer log items parse into index events", async () => {
  const { toExplorerRpcLogForTest } = await import("../src/lib/repo-index.ts");
  const encoded = makeLog({
    abi: coreAbi,
    eventName: "RepositoryCreated",
    args: { repoId: REPO_ID, owner: OWNER_A, name: "demo-repo", forkedFrom: ZERO_BYTES32 },
    blockNumber: 142557286,
    logIndex: 3,
  });
  const item = {
    address: "0x" + "33".repeat(20),
    topics: encoded.topics,
    data: encoded.data,
    blockNumber: `0x${(142557286).toString(16)}`,
    transactionHash: "0x" + "9f".repeat(32),
    logIndex: "0x3",
  };
  const log = toExplorerRpcLogForTest(item);
  assert.ok(log);
  const event = decodeRepoEvent(log);
  assert.equal(event.kind, "created");
  assert.equal(event.repoId, REPO_ID);
  assert.equal(event.name, "demo-repo");
  assert.equal(event.owner, toInjectiveAddress(OWNER_A));
  assert.equal(event.blockNumber, 142557286);
  assert.equal(event.logIndex, 3);
  assert.equal(toExplorerRpcLogForTest({ ...item, topics: [] }), null);
  assert.equal(toExplorerRpcLogForTest({ ...item, blockNumber: "abc" }), null);
});

test("explorerLogsUrl builds an etherscan-style logs query", async () => {
  const { explorerLogsUrl } = await import("../src/lib/repo-index.ts");
  const url = explorerLogsUrl("https://testnet.blockscout.injective.network/", "0xabc", "0xtopic");
  assert.equal(url, "https://testnet.blockscout.injective.network/api?module=logs&action=getLogs&address=0xabc&fromBlock=0&toBlock=latest&topic0=0xtopic");
});
test("searchRepoEntries matches a bare address query by exact owner", () => {
  const owner = toInjectiveAddress(OWNER_A);
  const entries = [
    entry("0x" + "01".repeat(32), "alpha", 100),
    entry("0x" + "02".repeat(32), "beta", 90),
  ].map((item, index) => ({ ...item, owner: index === 0 ? owner : toInjectiveAddress(OWNER_B) }));
  const hits = searchRepoEntries(entries, owner);
  assert.equal(hits.length, 1);
  assert.equal(hits[0].name, "alpha");
  const evmForm = toInjectiveAddress(OWNER_B);
  assert.equal(searchRepoEntries(entries, evmForm)[0].name, "beta");
  const stranger = toInjectiveAddress("0x9999999999999999999999999999999999999999");
  assert.equal(searchRepoEntries(entries, stranger).length, 0);
});