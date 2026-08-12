import assert from "node:assert/strict";
import { readFile, unlink, writeFile } from "node:fs/promises";
import test, { after } from "node:test";
import ts from "typescript";

const generated = new URL("../src/.chain-evm-v2.test.generated.mjs", import.meta.url);
const snapshotBlockTag = "0x1234";

after(async () => {
  await unlink(generated).catch(() => undefined);
});

async function loadChainModule() {
  const sourceUrl = new URL("../src/lib/chain.ts", import.meta.url);
  const source = await readFile(sourceUrl, "utf8");
  const output = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
    fileName: sourceUrl.pathname,
  });
  await writeFile(generated, output.outputText, "utf8");
  return import(`${generated.href}?test=${Date.now()}`);
}

function storageStub() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
    clear: () => values.clear(),
    key: () => null,
    length: 0,
  };
}

function calldataArgs(calldata) {
  assert.match(calldata, /^0x[0-9a-f]+$/i);
  return calldata.slice(10);
}

function calldataWord(args, index) {
  const word = args.slice(index * 64, (index + 1) * 64);
  assert.equal(word.length, 64);
  return BigInt(`0x${word}`);
}

function calldataString(args, offsetWord) {
  const offset = Number(calldataWord(args, offsetWord));
  const length = Number(BigInt(`0x${args.slice(offset * 2, offset * 2 + 64)}`));
  const start = (offset + 32) * 2;
  return Buffer.from(args.slice(start, start + length * 2), "hex").toString("utf8");
}

function abiHexWord(value) {
  return value.replace(/^0x/i, "").padStart(64, "0");
}

function abiErrorString(value) {
  const raw = Buffer.from(value, "utf8").toString("hex");
  return abiHexWord(Buffer.byteLength(value, "utf8").toString(16)) +
    raw.padEnd(Math.ceil(raw.length / 64) * 64, "0");
}

function locatorNotFoundData(owner, repo) {
  return `0x6de3a11d${abiHexWord(owner)}${abiHexWord("40")}${abiErrorString(repo)}`;
}

function repoMovedData(repoId, owner, repo) {
  return `0x58629d7f${abiHexWord(repoId)}${abiHexWord(owner)}${abiHexWord("60")}${abiErrorString(repo)}`;
}

function abiString(value) {
  const raw = Buffer.from(value, "utf8").toString("hex");
  return abiHexWord(Buffer.byteLength(value, "utf8").toString(16)) +
    raw.padEnd(Math.ceil(raw.length / 64) * 64, "0");
}

function abiStringArray(values) {
  let tail = "";
  const offsets = values.map((value) => {
    const offset = values.length * 32 + tail.length / 2;
    tail += abiString(value);
    return abiHexWord(offset.toString(16));
  });
  return abiHexWord(values.length.toString(16)) + offsets.join("") + tail;
}

function abiAddressArray(values) {
  return abiHexWord(values.length.toString(16)) + values.map(abiHexWord).join("");
}

function abiUintArray(values) {
  return abiHexWord(values.length.toString(16)) +
    values.map((value) => abiHexWord(value.toString(16))).join("");
}

function revenueSplitsResult(splits) {
  return `0x${abiHexWord("20")}${abiHexWord(splits.length.toString(16))}${splits.map((split) =>
    `${abiHexWord(split.address)}${abiHexWord(split.bps.toString(16))}`
  ).join("")}`;
}

function abiRepoTuple(owner, name, description = "description", branch = "main", moderationStatus = 0) {
  const nameTail = abiString(name);
  const descriptionTail = abiString(description);
  const branchTail = abiString(branch);
  const repoHeadBytes = 8 * 32;
  return [
    abiHexWord(owner),
    abiHexWord(repoHeadBytes.toString(16)),
    abiHexWord((repoHeadBytes + nameTail.length / 2).toString(16)),
    abiHexWord((repoHeadBytes + nameTail.length / 2 + descriptionTail.length / 2).toString(16)),
    abiHexWord("1"),
    abiHexWord("2"),
    abiHexWord(moderationStatus.toString(16)),
    abiHexWord("1"),
    nameTail,
    descriptionTail,
    branchTail,
  ].join("");
}

function resolvedRepoResult(repoId, canonical, owner, name) {
  const repo = abiRepoTuple(owner, name);
  return `0x${abiHexWord(repoId)}${abiHexWord(canonical ? "1" : "0")}${abiHexWord("60")}${repo}`;
}

function repoPageResult(nextCursor, hasMore, repos) {
  const idTail = abiHexWord(repos.length.toString(16)) +
    repos.map((repo) => abiHexWord(repo.id)).join("");
  let repoTail = "";
  const repoOffsets = repos.map((repo) => {
    const offset = repos.length * 32 + repoTail.length / 2;
    repoTail += abiRepoTuple(
      repo.owner,
      repo.name,
      repo.description,
      repo.branch,
      repo.status ?? (repo.frozen ? 1 : 0),
    );
    return abiHexWord(offset.toString(16));
  });
  const repoArray = abiHexWord(repos.length.toString(16)) + repoOffsets.join("") + repoTail;
  const headBytes = 4 * 32;
  return `0x${[
    abiHexWord(nextCursor.toString(16)),
    abiHexWord(hasMore ? "1" : "0"),
    abiHexWord(headBytes.toString(16)),
    abiHexWord((headBytes + idTail.length / 2).toString(16)),
    idTail,
    repoArray,
  ].join("")}`;
}

function abiRefTuple({ commit, packs, updatedAt, updatedBy }) {
  const commitTail = abiString(commit);
  const packsTail = abiStringArray(packs);
  const headBytes = 5 * 32;
  return [
    abiHexWord(headBytes.toString(16)),
    abiHexWord((headBytes + commitTail.length / 2).toString(16)),
    abiHexWord(updatedAt.toString(16)),
    abiHexWord(updatedBy),
    abiHexWord("1"),
    commitTail,
    packsTail,
  ].join("");
}

function refPageResult(nextCursor, hasMore, names, refs) {
  const namesTail = abiStringArray(names);
  let refsTail = "";
  const refsOffsets = refs.map((ref) => {
    const offset = refs.length * 32 + refsTail.length / 2;
    refsTail += abiRefTuple(ref);
    return abiHexWord(offset.toString(16));
  });
  const refsArray = abiHexWord(refs.length.toString(16)) + refsOffsets.join("") + refsTail;
  const headBytes = 4 * 32;
  return `0x${[
    abiHexWord(nextCursor.toString(16)),
    abiHexWord(hasMore ? "1" : "0"),
    abiHexWord(headBytes.toString(16)),
    abiHexWord((headBytes + namesTail.length / 2).toString(16)),
    namesTail,
    refsArray,
  ].join("")}`;
}

function collaboratorPageResult(nextCursor, hasMore, addresses, roles) {
  const addressTail = abiAddressArray(addresses);
  const roleTail = abiUintArray(roles);
  const headBytes = 4 * 32;
  return `0x${[
    abiHexWord(nextCursor.toString(16)),
    abiHexWord(hasMore ? "1" : "0"),
    abiHexWord(headBytes.toString(16)),
    abiHexWord((headBytes + addressTail.length / 2).toString(16)),
    addressTail,
    roleTail,
  ].join("")}`;
}

function abiBadgeTuple({ id, repoId, recipient, reason, awardedBy, awardedAt }) {
  const reasonTail = abiString(reason);
  const headBytes = 6 * 32;
  return [
    abiHexWord(id.toString(16)),
    abiHexWord(repoId),
    abiHexWord(recipient),
    abiHexWord(headBytes.toString(16)),
    abiHexWord(awardedBy),
    abiHexWord(awardedAt.toString(16)),
    reasonTail,
  ].join("");
}

function badgePageResult(nextCursor, hasMore, badges) {
  let tail = "";
  const offsets = badges.map((badge) => {
    const offset = badges.length * 32 + tail.length / 2;
    tail += abiBadgeTuple(badge);
    return abiHexWord(offset.toString(16));
  });
  const values = abiHexWord(badges.length.toString(16)) + offsets.join("") + tail;
  return `0x${[
    abiHexWord(nextCursor.toString(16)),
    abiHexWord(hasMore ? "1" : "0"),
    abiHexWord((3 * 32).toString(16)),
    values,
  ].join("")}`;
}

test("auto mode tries EVM eth_call and falls back to V1 LCD reads", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  const storage = new Map();
  globalThis.localStorage = {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, String(value)),
    removeItem: (key) => storage.delete(key),
    clear: () => storage.clear(),
    key: () => null,
    length: 0,
  };
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url: String(url), init });
    if (String(url) === cfg.evmRpc) {
      return new Response(JSON.stringify({
        jsonrpc: "2.0",
        id: 1,
        error: {
          code: 3,
          message: "execution reverted",
          data: { originalError: { data: locatorNotFoundData("11".repeat(20), "demo") } },
        },
      }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    return new Response(JSON.stringify({
      data: {
        owner: "inj1owner",
        name: "demo",
        description: "from v1",
        default_branch: "main",
        created_at: 1,
        updated_at: 2,
        moderation_status: "active",
        forked_from: null,
      },
    }), { status: 200, headers: { "content-type": "application/json" } });
  };

  try {
    const result = await module.repoInfo(cfg, "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d", "demo");
    assert.equal(result.description, "from v1");
    assert.equal(calls.length, 2);
    assert.equal(calls[0].init.method, "POST");
    const evmBody = JSON.parse(calls[0].init.body);
    assert.equal(evmBody.method, "eth_call");
    assert.equal(evmBody.params[0].to, cfg.evmContract);
    assert.match(evmBody.params[0].data, /^0x2c1ce86c[0-9a-f]+$/);
    assert.match(calls[1].url, /\/cosmwasm\/wasm\/v1\/contract\//);

    module.saveConfig(cfg);
    const stored = JSON.parse(storage.get("igit-web-config"));
    assert.deepEqual(stored, { profile: "injective-testnet", contractVersion: "auto" });
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("auto mode does not hide configured V2 transport failures behind V1", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {}, clear: () => {}, key: () => null, length: 0 };
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  globalThis.fetch = async () => { throw new Error("network unavailable"); };
  try {
    await assert.rejects(
      module.repoInfo(cfg, "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d", "demo"),
      /network unavailable/,
    );
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("auto mode does not treat a stable RepoNotFound error as a V1 locator miss", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {}, clear: () => {}, key: () => null, length: 0 };
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  let calls = 0;
  globalThis.fetch = async (url) => {
    calls += 1;
    if (String(url) === cfg.evmRpc) {
      return new Response(JSON.stringify({
        jsonrpc: "2.0",
        id: 1,
        error: {
          code: 3,
          message: "execution reverted",
          data: { originalError: { data: `0xf6ca4ec1${"00".repeat(32)}` } },
        },
      }), { status: 200, headers: { "content-type": "application/json" } });
    }
    return new Response(JSON.stringify({
      data: {
        owner: "inj1owner",
        name: "legacy",
        description: "legacy fallback",
        default_branch: "main",
        created_at: 1,
        updated_at: 2,
        moderation_status: "active",
        forked_from: null,
      },
    }), { status: 200, headers: { "content-type": "application/json" } });
  };
  try {
    await assert.rejects(
      module.repoInfo(
        cfg,
        "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d",
        "legacy",
      ),
      (error) => error instanceof module.EVMRepoNotFoundError && error.repoId === `0x${"00".repeat(32)}`,
    );
    assert.equal(calls, 1);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM owner repository listing drains bounded pages without V1 fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";

  const requestedOwner = "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d";
  const ownerHex = "7519165dc1b7987c43a03328110922623a099c09";
  const pages = new Map([
    [0, repoPageResult(64, true, [
      { id: "11".repeat(32), owner: ownerHex, name: "alpha", description: "first", branch: "main" },
      { id: "22".repeat(32), owner: ownerHex, name: "beta", description: "second", branch: "develop" },
    ])],
    [64, repoPageResult(65, false, [
      { id: "33".repeat(32), owner: ownerHex, name: "gamma", description: "third", branch: "main", frozen: true },
      { id: "44".repeat(32), owner: ownerHex, name: "delta", description: "fourth", branch: "main", status: 2 },
    ])],
  ]);
  const cursors = [];
  let blockCalls = 0;
  let lcdCalls = 0;
  globalThis.fetch = async (url, init = {}) => {
    if (String(url) !== cfg.evmRpc) {
      lcdCalls += 1;
      throw new Error(`unexpected V1 LCD fallback: ${url}`);
    }
    const request = JSON.parse(init.body);
    if (request.method === "eth_blockNumber") {
      blockCalls += 1;
      return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result: snapshotBlockTag }));
    }
    assert.equal(request.method, "eth_call");
    assert.equal(request.params[1], snapshotBlockTag);
    const calldata = request.params[0].data;
    assert.equal(calldata.slice(0, 10), "0x502a5e93");
    const args = calldataArgs(calldata);
    assert.equal(calldataWord(args, 0), BigInt(`0x${ownerHex}`));
    const cursor = Number(calldataWord(args, 1));
    assert.equal(calldataWord(args, 2), 64n);
    cursors.push(cursor);
    const result = pages.get(cursor);
    assert.ok(result, `missing repository page fixture for cursor ${cursor}`);
    return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  };

  try {
    const active = await module.listRepos(cfg, requestedOwner);
    assert.deepEqual(active.map((repo) => repo.name), ["alpha", "beta"]);
    const all = await module.listRepos(cfg, requestedOwner, true);
    assert.deepEqual(all.map((repo) => repo.name), ["alpha", "beta", "gamma", "delta"]);
    assert.equal(all[1].default_branch, "develop");
    assert.equal(all[2].moderation_status, "frozen");
    assert.equal(all[3].moderation_status, "delisted");
    assert.deepEqual(cursors, [0, 64, 0, 64]);
    assert.equal(blockCalls, 2);
    assert.equal(lcdCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM owner repository pagination rejects a stalled cursor without V1 fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  let rpcCalls = 0;
  let blockCalls = 0;
  let lcdCalls = 0;
  globalThis.fetch = async (url, init = {}) => {
    if (String(url) !== cfg.evmRpc) {
      lcdCalls += 1;
      throw new Error(`unexpected V1 LCD fallback: ${url}`);
    }
    const request = JSON.parse(init.body);
    if (request.method === "eth_blockNumber") {
      blockCalls += 1;
      return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result: snapshotBlockTag }));
    }
    rpcCalls += 1;
    assert.equal(request.method, "eth_call");
    assert.equal(request.params[1], snapshotBlockTag);
    assert.equal(request.params[0].data.slice(0, 10), "0x502a5e93");
    return new Response(JSON.stringify({
      jsonrpc: "2.0",
      id: request.id,
      result: repoPageResult(0, true, []),
    }), { status: 200, headers: { "content-type": "application/json" } });
  };

  try {
    await assert.rejects(
      module.listRepos(cfg, "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d"),
      /EVM repository page cursor did not advance: 0/,
    );
    assert.equal(rpcCalls, 1);
    assert.equal(blockCalls, 1);
    assert.equal(lcdCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM stable repo ID reads drain two ref and collaborator pages without V1 fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";

  const requestedOwner = "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d";
  const canonicalOwner = "22".repeat(20);
  const repoId = `0x${"ab".repeat(32)}`;
  const resolved = resolvedRepoResult(repoId, false, canonicalOwner, "demo");
  const refPages = new Map([
    [0, refPageResult(64, true, ["refs/heads/main", "refs/heads/release"], [
      {
        commit: "a".repeat(40),
        packs: ["ipfs://pack-one"],
        updatedAt: 3,
        updatedBy: "33".repeat(20),
      },
      {
        commit: "b".repeat(40),
        packs: ["ipfs://pack-two"],
        updatedAt: 4,
        updatedBy: "44".repeat(20),
      },
    ])],
    [64, refPageResult(65, false, ["refs/tags/v1"], [{
      commit: "c".repeat(40),
      packs: ["ipfs://pack-three"],
      updatedAt: 5,
      updatedBy: "55".repeat(20),
    }])],
  ]);
  const collaboratorPages = new Map([
    [0, collaboratorPageResult(64, true, ["66".repeat(20)], [1])],
    [64, collaboratorPageResult(65, false, ["77".repeat(20)], [2])],
  ]);
  const refCursors = [];
  const collaboratorCursors = [];
  let resolveCalls = 0;
  let blockCalls = 0;
  let lcdCalls = 0;

  globalThis.fetch = async (url, init = {}) => {
    if (String(url) !== cfg.evmRpc) {
      lcdCalls += 1;
      throw new Error(`unexpected V1 LCD fallback: ${url}`);
    }
    const request = JSON.parse(init.body);
    if (request.method === "eth_blockNumber") {
      blockCalls += 1;
      return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result: snapshotBlockTag }));
    }
    assert.equal(request.method, "eth_call");
    assert.equal(request.params[1], snapshotBlockTag);
    const calldata = request.params[0].data;
    const selector = calldata.slice(0, 10);
    let result;
    if (selector === "0x2c1ce86c") {
      resolveCalls += 1;
      result = resolved;
    } else {
      const args = calldataArgs(calldata);
      assert.equal(calldataWord(args, 0), BigInt(repoId));
      const cursor = Number(calldataWord(args, 1));
      assert.equal(calldataWord(args, 2), 64n);
      if (selector === "0x084871ef") {
        refCursors.push(cursor);
        result = refPages.get(cursor);
      } else if (selector === "0x1758c17d") {
        collaboratorCursors.push(cursor);
        result = collaboratorPages.get(cursor);
      } else {
        assert.fail(`unexpected EVM selector ${selector}`);
      }
    }
    assert.ok(result, `missing page fixture for ${selector}`);
    return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  };

  try {
    const refs = await module.listRefs(cfg, requestedOwner, "demo");
    assert.deepEqual(refs.map((ref) => ref.ref_name), [
      "refs/heads/main",
      "refs/heads/release",
      "refs/tags/v1",
    ]);
    assert.deepEqual(refs.map((ref) => ref.commit_sha), [
      "a".repeat(40),
      "b".repeat(40),
      "c".repeat(40),
    ]);

    const collaborators = await module.listCollaborators(cfg, requestedOwner, "demo");
    assert.deepEqual(collaborators.map((entry) => entry.role), ["maintainer", "reader"]);
    assert.ok(collaborators.every((entry) => entry.address.startsWith("inj1")));

    assert.deepEqual(refCursors, [0, 64]);
    assert.deepEqual(collaboratorCursors, [0, 64]);
    assert.equal(resolveCalls, 2);
    assert.equal(blockCalls, 2);
    assert.equal(lcdCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM stable repo ID pagination rejects stalled cursors without V1 fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const owner = "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d";
  const repoId = `0x${"ab".repeat(32)}`;
  const resolved = resolvedRepoResult(repoId, true, "22".repeat(20), "demo");
  const cases = [
    {
      call: () => module.listRefs(cfg, owner, "demo"),
      selector: "0x084871ef",
      result: refPageResult(0, true, [], []),
      error: /EVM ref page cursor did not advance: 0/,
    },
    {
      call: () => module.listCollaborators(cfg, owner, "demo"),
      selector: "0x1758c17d",
      result: collaboratorPageResult(0, true, [], []),
      error: /EVM collaborator page cursor did not advance: 0/,
    },
  ];
  let lcdCalls = 0;

  try {
    for (const fixture of cases) {
      let rpcCalls = 0;
      let blockCalls = 0;
      globalThis.fetch = async (url, init = {}) => {
        if (String(url) !== cfg.evmRpc) {
          lcdCalls += 1;
          throw new Error(`unexpected V1 LCD fallback: ${url}`);
        }
        const request = JSON.parse(init.body);
        if (request.method === "eth_blockNumber") {
          blockCalls += 1;
          return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result: snapshotBlockTag }));
        }
        rpcCalls += 1;
        assert.equal(request.method, "eth_call");
        assert.equal(request.params[1], snapshotBlockTag);
        const calldata = request.params[0].data;
        const selector = calldata.slice(0, 10);
        if (selector === "0x2c1ce86c") {
          return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result: resolved }));
        }
        assert.equal(selector, fixture.selector);
        const args = calldataArgs(calldata);
        assert.equal(calldataWord(args, 0), BigInt(repoId));
        assert.equal(calldataWord(args, 1), 0n);
        assert.equal(calldataWord(args, 2), 64n);
        return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result: fixture.result }));
      };

      await assert.rejects(fixture.call(), fixture.error);
      assert.equal(rpcCalls, 2);
      assert.equal(blockCalls, 1);
    }
    assert.equal(lcdCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM ownership transfer query decodes pending state and an empty state", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "v2");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const repoId = `0x${"ab".repeat(32)}`;
  const target = "22".repeat(20);
  const results = [
    `0x${abiHexWord(target)}${abiHexWord("64")}${abiHexWord("c8")}${abiHexWord("12c")}`,
    `0x${"0".repeat(64 * 4)}`,
  ];
  let calls = 0;
  globalThis.fetch = async (_url, init) => {
    const request = JSON.parse(init.body);
    assert.equal(request.method, "eth_call");
    assert.equal(request.params[0].to, cfg.evmContract);
    assert.equal(request.params[0].data.slice(0, 10), "0x01a17250");
    assert.equal(calldataWord(calldataArgs(request.params[0].data), 0), BigInt(repoId));
    assert.equal(request.params[1], "latest");
    const result = results[calls];
    calls += 1;
    return new Response(JSON.stringify({ jsonrpc: "2.0", id: request.id, result }));
  };

  try {
    const pending = await module.pendingOwnershipTransferWithEvm(cfg, repoId);
    assert.match(pending.newOwner, /^inj1/);
    assert.equal(pending.proposedAt, 100);
    assert.equal(pending.executeAfter, 200);
    assert.equal(pending.expiresAt, 300);
    assert.equal(await module.pendingOwnershipTransferWithEvm(cfg, repoId), null);
    assert.equal(calls, 2);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM ownership transfer capabilities are role-based and never browser-time gated", async () => {
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const owner = "inj1owner";
  const target = "inj1target";
  const pending = {
    newOwner: target,
    proposedAt: 100,
    executeAfter: Number.MAX_SAFE_INTEGER - 1,
    expiresAt: Number.MAX_SAFE_INTEGER,
  };

  try {
    assert.deepEqual(module.ownershipTransferCapabilities(pending, owner, owner), {
      canCancel: true,
      canAccept: false,
      canReject: false,
      canExpire: true,
    });
    assert.deepEqual(module.ownershipTransferCapabilities(pending, target, owner), {
      canCancel: false,
      canAccept: true,
      canReject: true,
      canExpire: true,
    });
    assert.deepEqual(module.ownershipTransferCapabilities(pending, "inj1observer", owner), {
      canCancel: false,
      canAccept: false,
      canReject: false,
      canExpire: true,
    });
    assert.deepEqual(module.ownershipTransferCapabilities(null, target, owner), {
      canCancel: false,
      canAccept: false,
      canReject: false,
      canExpire: false,
    });
  } finally {
    globalThis.localStorage = previousStorage;
  }
});

test("EVM ownership transfer writes encode every action and wait for receipts", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "v2");
  cfg.evmChainId = 31337;
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const repoId = `0x${"ab".repeat(32)}`;
  const target = "0x3333333333333333333333333333333333333333";
  const txHashes = Array.from({ length: 5 }, (_, index) => `0x${(index + 1).toString(16).padStart(64, "0")}`);
  const requests = [];
  let sendIndex = 0;
  let fetchCalls = 0;
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected CosmWasm fallback");
  };
  const provider = {
    async request(request) {
      requests.push(request);
      if (request.method === "wallet_switchEthereumChain") return null;
      if (request.method === "eth_requestAccounts") {
        return ["0x2222222222222222222222222222222222222222"];
      }
      if (request.method === "eth_sendTransaction") {
        const hash = txHashes[sendIndex];
        sendIndex += 1;
        return hash;
      }
      if (request.method === "eth_getTransactionReceipt") {
        return { transactionHash: request.params[0], status: "0x1" };
      }
      throw new Error(`unexpected provider method ${request.method}`);
    },
  };

  try {
    assert.equal(await module.beginOwnershipTransferWithEvm(provider, cfg, repoId, target), txHashes[0]);
    assert.equal(await module.cancelOwnershipTransferWithEvm(provider, cfg, repoId), txHashes[1]);
    assert.equal(await module.rejectOwnershipTransferWithEvm(provider, cfg, repoId), txHashes[2]);
    assert.equal(await module.expireOwnershipTransferWithEvm(provider, cfg, repoId), txHashes[3]);
    assert.equal(await module.acceptOwnershipWithEvm(provider, cfg, repoId), txHashes[4]);

    const sends = requests.filter((request) => request.method === "eth_sendTransaction");
    assert.deepEqual(sends.map((request) => request.params[0].data.slice(0, 10)), [
      "0x27abb3e4",
      "0x99d02f6d",
      "0xe704076c",
      "0x5cfae861",
      "0x1b4f6c46",
    ]);
    for (const request of sends) {
      const tx = request.params[0];
      assert.equal(tx.from, "0x2222222222222222222222222222222222222222");
      assert.equal(tx.to, cfg.evmContract);
      assert.equal(calldataWord(calldataArgs(tx.data), 0), BigInt(repoId));
    }
    assert.equal(calldataWord(calldataArgs(sends[0].params[0].data), 1), BigInt(target));
    assert.deepEqual(
      requests.filter((request) => request.method === "eth_getTransactionReceipt").map((request) => request.params[0]),
      txHashes,
    );
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM metadata writes preserve patch flags and clear the query cache after a successful receipt", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmChainId = 31337;
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const legacyCfg = { ...cfg, contractVersion: "v1" };

  let lcdCalls = 0;
  globalThis.fetch = async () => {
    lcdCalls += 1;
    return new Response(JSON.stringify({ data: { repos: [] } }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  };

  const txHashes = [`0x${"ab".repeat(32)}`, `0x${"cd".repeat(32)}`];
  const requests = [];
  let sendIndex = 0;
  const provider = {
    async request(request) {
      requests.push(request);
      switch (request.method) {
        case "wallet_switchEthereumChain":
          return null;
        case "eth_requestAccounts":
          return ["0x2222222222222222222222222222222222222222"];
        case "eth_sendTransaction": {
          const hash = txHashes[sendIndex];
          sendIndex += 1;
          return hash;
        }
        case "eth_getTransactionReceipt":
          return { transactionHash: request.params[0], status: "0x1" };
        default:
          throw new Error(`unexpected provider method ${request.method}`);
      }
    },
  };

  try {
    // Populate the LCD query cache and prove the second read is a cache hit.
    await module.listRepos(legacyCfg, "inj1owner");
    await module.listRepos(legacyCfg, "inj1owner");
    assert.equal(lcdCalls, 1);

    const firstHash = await module.updateRepoInfoWithEvm(provider, cfg, "demo", {
      description: "",
      defaultBranch: undefined,
    });
    assert.equal(firstHash, txHashes[0]);

    // A successful receipt invalidates data cached before the transaction.
    await module.listRepos(legacyCfg, "inj1owner");
    assert.equal(lcdCalls, 2);

    const secondHash = await module.updateRepoInfoWithEvm(provider, cfg, "demo", {
      description: undefined,
      defaultBranch: "",
    });
    assert.equal(secondHash, txHashes[1]);

    const switches = requests.filter((request) => request.method === "wallet_switchEthereumChain");
    assert.equal(switches.length, 2);
    for (const request of switches) {
      assert.deepEqual(request.params, [{ chainId: "0x7a69" }]);
    }

    const sends = requests.filter((request) => request.method === "eth_sendTransaction");
    assert.equal(sends.length, 2);
    for (const request of sends) {
      assert.equal(request.params[0].from, "0x2222222222222222222222222222222222222222");
      assert.equal(request.params[0].to, cfg.evmContract);
      assert.equal(request.params.length, 1);
    }

    const firstData = sends[0].params[0].data;
    assert.equal(firstData.slice(0, 10), "0x1a532166");
    const firstArgs = calldataArgs(firstData);
    assert.equal(calldataWord(firstArgs, 1), 1n);
    assert.equal(calldataWord(firstArgs, 3), 0n);
    assert.equal(calldataString(firstArgs, 0), "demo");
    assert.equal(calldataString(firstArgs, 2), "");
    assert.equal(calldataString(firstArgs, 4), "");

    const secondData = sends[1].params[0].data;
    assert.equal(secondData.slice(0, 10), "0x1a532166");
    const secondArgs = calldataArgs(secondData);
    assert.equal(calldataWord(secondArgs, 1), 0n);
    assert.equal(calldataWord(secondArgs, 3), 1n);
    assert.equal(calldataString(secondArgs, 0), "demo");
    assert.equal(calldataString(secondArgs, 2), "");
    assert.equal(calldataString(secondArgs, 4), "");

    const receipts = requests.filter((request) => request.method === "eth_getTransactionReceipt");
    assert.deepEqual(receipts.map((request) => request.params), [[txHashes[0]], [txHashes[1]]]);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM metadata receipt failure is surfaced without a CosmWasm write fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const txHash = `0x${"ef".repeat(32)}`;
  let fetchCalls = 0;
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected LCD fallback");
  };
  const methods = [];
  const provider = {
    async request(request) {
      methods.push(request.method);
      if (request.method === "wallet_switchEthereumChain") return null;
      if (request.method === "eth_requestAccounts") {
        return ["0x2222222222222222222222222222222222222222"];
      }
      if (request.method === "eth_sendTransaction") return txHash;
      if (request.method === "eth_getTransactionReceipt") {
        return { transactionHash: txHash, status: "0x0" };
      }
      throw new Error(`unexpected provider method ${request.method}`);
    },
  };

  try {
    await assert.rejects(
      module.updateRepoInfoWithEvm(provider, cfg, "demo", { description: "failed" }),
      /reverted \(receipt status 0x0\)/,
    );
    assert.deepEqual(methods, [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
      "eth_getTransactionReceipt",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM metadata writes decode RepoMoved and expose the canonical URL", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const repoId = `0x${"ab".repeat(32)}`;
  const currentOwner = "33".repeat(20);
  let fetchCalls = 0;
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected LCD fallback");
  };
  const methods = [];
  const provider = {
    async request(request) {
      methods.push(request.method);
      if (request.method === "wallet_switchEthereumChain") return null;
      if (request.method === "eth_requestAccounts") {
        return ["0x2222222222222222222222222222222222222222"];
      }
      if (request.method === "eth_sendTransaction") {
        throw {
          code: 3,
          message: "execution reverted",
          data: { originalError: { data: repoMovedData(repoId, currentOwner, "demo") } },
        };
      }
      throw new Error(`unexpected provider method ${request.method}`);
    },
  };

  try {
    await assert.rejects(
      module.updateRepoInfoWithEvm(provider, cfg, "demo", { description: "moved" }),
      (error) => {
        assert.ok(error instanceof module.EVMRepoMovedError);
        assert.equal(error.repoId, repoId);
        assert.equal(error.repo, "demo");
        assert.match(error.currentOwner, /^inj1/);
        assert.equal(error.canonicalUrl, `igit://${error.currentOwner}/demo`);
        return true;
      },
    );
    assert.deepEqual(methods, [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM metadata writes fail closed before wallet or LCD access when no V2 contract is configured", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "";
  let providerCalls = 0;
  let fetchCalls = 0;
  const provider = {
    async request() {
      providerCalls += 1;
      throw new Error("wallet should not be called");
    },
  };
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("LCD should not be called");
  };

  try {
    await assert.rejects(
      module.updateRepoInfoWithEvm(provider, cfg, "demo", { defaultBranch: "main" }),
      /EVM V2 registry is not configured/,
    );
    assert.equal(providerCalls, 0);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM badge recipient pages and canonical repo lookups share one block snapshot", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "v2");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmBadgeModule = "0x2222222222222222222222222222222222222222";
  const recipient = "0x3333333333333333333333333333333333333333";
  const awardedBy = "0x4444444444444444444444444444444444444444";
  const repoId = `0x${"ab".repeat(32)}`;
  const requests = [];

  globalThis.fetch = async (url, init = {}) => {
    assert.equal(String(url), cfg.evmRpc);
    const body = JSON.parse(init.body);
    requests.push(body);
    if (body.method === "eth_blockNumber") {
      return new Response(JSON.stringify({ jsonrpc: "2.0", id: body.id, result: snapshotBlockTag }));
    }
    assert.equal(body.method, "eth_call");
    assert.equal(body.params[1], snapshotBlockTag);
    const { to, data } = body.params[0];
    let result;
    if (to.toLowerCase() === cfg.evmBadgeModule.toLowerCase()) {
      assert.equal(data.slice(0, 10), "0x4a7d9d07");
      const cursor = Number(calldataWord(calldataArgs(data), 1));
      assert.equal(calldataWord(calldataArgs(data), 2), 64n);
      result = cursor === 0
        ? badgePageResult(64, true, [{ id: 1, repoId, recipient, reason: "first", awardedBy, awardedAt: 10 }])
        : badgePageResult(65, false, [{ id: 2, repoId, recipient, reason: "second", awardedBy, awardedAt: 11 }]);
    } else {
      assert.equal(to.toLowerCase(), cfg.evmContract.toLowerCase());
      assert.equal(data.slice(0, 10), "0x3334edd1");
      assert.equal(BigInt(`0x${data.slice(10, 74)}`), BigInt(repoId));
      result = `0x${abiHexWord("20")}${abiRepoTuple(awardedBy, "renamed")}`;
    }
    return new Response(JSON.stringify({ jsonrpc: "2.0", id: body.id, result }));
  };

  try {
    const badges = await module.badgesByRecipient(cfg, recipient);
    assert.deepEqual(badges.map((badge) => ({
      id: badge.id,
      repoId: badge.repo_id,
      repoName: badge.repo_name,
      reason: badge.reason,
    })), [
      { id: 1, repoId, repoName: "renamed", reason: "first" },
      { id: 2, repoId, repoName: "renamed", reason: "second" },
    ]);
    assert.equal(requests.filter((request) => request.method === "eth_blockNumber").length, 1);
    assert.equal(requests.filter((request) => request.params?.[0]?.to === cfg.evmBadgeModule).length, 2);
    assert.equal(requests.filter((request) => request.params?.[0]?.to === cfg.evmContract).length, 1);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM badge award targets the module, waits for receipt, and never writes V1", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmChainId = 31337;
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmBadgeModule = "0x2222222222222222222222222222222222222222";
  const repoId = `0x${"ab".repeat(32)}`;
  const recipient = "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d";
  const txHash = `0x${"cd".repeat(32)}`;
  let fetchCalls = 0;
  const requests = [];
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected LCD fallback");
  };
  const provider = {
    async request(request) {
      requests.push(request);
      if (request.method === "wallet_switchEthereumChain") return null;
      if (request.method === "eth_requestAccounts") return ["0x3333333333333333333333333333333333333333"];
      if (request.method === "eth_sendTransaction") return txHash;
      if (request.method === "eth_getTransactionReceipt") {
        return { transactionHash: txHash, status: "0x1" };
      }
      throw new Error(`unexpected provider method ${request.method}`);
    },
  };

  try {
    assert.equal(await module.awardBadgeWithEvm(provider, cfg, repoId, recipient, "  fixed CI  "), txHash);
    const send = requests.find((request) => request.method === "eth_sendTransaction");
    assert.equal(send.params[0].to, cfg.evmBadgeModule);
    assert.equal(send.params[0].data.slice(0, 10), "0x3ad71059");
    const args = calldataArgs(send.params[0].data);
    assert.equal(calldataWord(args, 0), BigInt(repoId));
    assert.equal(calldataWord(args, 1), BigInt("0x7519165dc1b7987c43a03328110922623a099c09"));
    assert.equal(calldataString(args, 2), "fixed CI");
    assert.deepEqual(requests.map((request) => request.method), [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
      "eth_getTransactionReceipt",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM badge award surfaces a reverted receipt without a V1 write fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmChainId = 31337;
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmBadgeModule = "0x2222222222222222222222222222222222222222";
  const txHash = `0x${"cd".repeat(32)}`;
  let fetchCalls = 0;
  const requests = [];
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected LCD fallback");
  };
  const provider = {
    async request(request) {
      requests.push(request);
      if (request.method === "wallet_switchEthereumChain") return null;
      if (request.method === "eth_requestAccounts") return ["0x3333333333333333333333333333333333333333"];
      if (request.method === "eth_sendTransaction") return txHash;
      if (request.method === "eth_getTransactionReceipt") {
        return { transactionHash: txHash, status: "0x0" };
      }
      throw new Error(`unexpected provider method ${request.method}`);
    },
  };

  try {
    await assert.rejects(
      module.awardBadgeWithEvm(
        provider,
        cfg,
        `0x${"ab".repeat(32)}`,
        "0x5555555555555555555555555555555555555555",
        "failed receipt",
      ),
      /reverted \(receipt status 0x0\)/,
    );
    assert.deepEqual(requests.map((request) => request.method), [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
      "eth_getTransactionReceipt",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM badge award decodes module custom errors without a V1 write fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmChainId = 31337;
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmBadgeModule = "0x2222222222222222222222222222222222222222";
  const repoId = `0x${"ab".repeat(32)}`;
  const caller = "3333333333333333333333333333333333333333";
  const recipient = "5555555555555555555555555555555555555555";
  const cases = [
    {
      data: `0x3dc0967d${abiHexWord(repoId)}${abiHexWord("1")}`,
      message: /repository is not active: frozen/,
    },
    {
      data: `0x17858bbe${abiHexWord(recipient)}`,
      message: /invalid badge recipient: inj1/,
    },
    {
      data: `0xfc91889b${abiHexWord("101")}${abiHexWord("100")}`,
      message: /invalid badge reason length 257; maximum 256 UTF-8 bytes/,
    },
    {
      data: `0xdc29602f${abiHexWord("2a")}`,
      message: /badge not found: 42/,
    },
    {
      data: `0xdc78128f${abiHexWord("41")}${abiHexWord("40")}`,
      message: /invalid EVM page size 65; maximum 64/,
    },
    {
      data: `0x2165c8f6${abiHexWord("9")}${abiHexWord("8")}`,
      message: /invalid EVM page cursor 9; total 8/,
    },
    {
      data: "0x11a1e697",
      message: /EVM module registry configuration is invalid/,
    },
    {
      data: `0x8e4a23d6${abiHexWord(caller)}`,
      message: /EVM contract caller is unauthorized: inj1/,
    },
  ];
  let fetchCalls = 0;
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected LCD fallback");
  };

  try {
    for (const fixture of cases) {
      const methods = [];
      const provider = {
        async request(request) {
          methods.push(request.method);
          if (request.method === "wallet_switchEthereumChain") return null;
          if (request.method === "eth_requestAccounts") return [`0x${caller}`];
          if (request.method === "eth_sendTransaction") {
            throw {
              code: 3,
              message: "execution reverted",
              data: { originalError: { data: fixture.data } },
            };
          }
          throw new Error(`unexpected provider method ${request.method}`);
        },
      };
      await assert.rejects(
        module.awardBadgeWithEvm(
          provider,
          cfg,
          repoId,
          `0x${recipient}`,
          "valid reason",
        ),
        fixture.message,
      );
      assert.deepEqual(methods, [
        "wallet_switchEthereumChain",
        "eth_requestAccounts",
        "eth_sendTransaction",
      ]);
    }
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM badge award fails closed before wallet and LCD when module address is missing", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmBadgeModule = "";
  let providerCalls = 0;
  let fetchCalls = 0;
  const provider = { async request() { providerCalls += 1; } };
  globalThis.fetch = async () => { fetchCalls += 1; };

  try {
    await assert.rejects(
      module.awardBadgeWithEvm(
        provider,
        cfg,
        `0x${"ab".repeat(32)}`,
        "0x3333333333333333333333333333333333333333",
        "fixed CI",
      ),
      /badge module is not configured/,
    );
    assert.equal(providerCalls, 0);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM economic reads resolve one stable repo ID and target the module without V1 fallback", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmEconomicModule = "0x2222222222222222222222222222222222222222";
  const owner = "0x3333333333333333333333333333333333333333";
  const recipient = "4444444444444444444444444444444444444444";
  const repoId = `0x${"ab".repeat(32)}`;
  const requests = [];

  globalThis.fetch = async (url, init = {}) => {
    assert.equal(String(url), cfg.evmRpc);
    const body = JSON.parse(init.body);
    requests.push(body);
    if (body.method === "eth_blockNumber") {
      return new Response(JSON.stringify({ jsonrpc: "2.0", id: body.id, result: snapshotBlockTag }));
    }
    assert.equal(body.method, "eth_call");
    const { to, data } = body.params[0];
    if (to.toLowerCase() === cfg.evmContract.toLowerCase()) {
      assert.equal(data.slice(0, 10), "0x2c1ce86c");
      return new Response(JSON.stringify({
        jsonrpc: "2.0", id: body.id, result: resolvedRepoResult(repoId, true, owner, "demo"),
      }));
    }
    assert.equal(to.toLowerCase(), cfg.evmEconomicModule.toLowerCase());
    assert.equal(BigInt(`0x${data.slice(10, 74)}`), BigInt(repoId));
    const result = data.slice(0, 10) === "0xd223931b"
      ? revenueSplitsResult([{ address: recipient, bps: 1250 }])
      : `0x${abiHexWord((5n * 10n ** 17n).toString(16))}`;
    assert.ok(data.slice(0, 10) === "0xd223931b" || data.slice(0, 10) === "0x60d02143");
    return new Response(JSON.stringify({ jsonrpc: "2.0", id: body.id, result }));
  };

  try {
    assert.deepEqual(await module.revenueSplits(cfg, owner, "demo"), [{
      address: "inj1g3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyfc6zmu",
      bps: 1250,
    }]);
    assert.deepEqual(await module.sponsorTotals(cfg, owner, "demo"), [{
      denom: "inj",
      amount: "500000000000000000",
    }]);
    assert.equal(requests.length, 6);
    const calls = requests.filter((request) => request.method === "eth_call");
    assert.equal(calls.length, 4);
    assert.ok(calls.every((request) => request.params[1] === snapshotBlockTag));
    assert.equal(calls.filter((request) => request.params[0].to === cfg.evmEconomicModule).length, 2);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM economic sponsor sends exact INJ value, waits for receipt, and never writes V1", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmChainId = 31337;
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmEconomicModule = "0x2222222222222222222222222222222222222222";
  const repoId = `0x${"ab".repeat(32)}`;
  const txHash = `0x${"cd".repeat(32)}`;
  let fetchCalls = 0;
  const requests = [];
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected LCD fallback");
  };
  const provider = {
    async request(request) {
      requests.push(request);
      if (request.method === "wallet_switchEthereumChain") return null;
      if (request.method === "eth_requestAccounts") return ["0x3333333333333333333333333333333333333333"];
      if (request.method === "eth_sendTransaction") return txHash;
      if (request.method === "eth_getTransactionReceipt") return { transactionHash: txHash, status: "0x1" };
      throw new Error(`unexpected provider method ${request.method}`);
    },
  };

  try {
    assert.equal(await module.sponsorWithEconomicModule(provider, cfg, repoId, "0.125", " great work "), txHash);
    const send = requests.find((request) => request.method === "eth_sendTransaction");
    assert.equal(send.params[0].to, cfg.evmEconomicModule);
    assert.equal(send.params[0].value, `0x${(125n * 10n ** 15n).toString(16)}`);
    assert.equal(send.params[0].data.slice(0, 10), "0x5b480bac");
    const args = calldataArgs(send.params[0].data);
    assert.equal(calldataWord(args, 0), BigInt(repoId));
    assert.equal(calldataString(args, 1), "great work");
    assert.deepEqual(requests.map((request) => request.method), [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
      "eth_getTransactionReceipt",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM economic sponsor fails closed for missing module and reverted receipt", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const repoId = `0x${"ab".repeat(32)}`;
  let fetchCalls = 0;
  let providerCalls = 0;
  globalThis.fetch = async () => { fetchCalls += 1; };
  const unusedProvider = { async request() { providerCalls += 1; } };

  try {
    await assert.rejects(
      module.sponsorWithEconomicModule(unusedProvider, cfg, repoId, "0.1", "message"),
      /economic module is not configured/,
    );
    assert.equal(providerCalls, 0);
    assert.equal(fetchCalls, 0);

    cfg.evmEconomicModule = "0x2222222222222222222222222222222222222222";
    const txHash = `0x${"cd".repeat(32)}`;
    const methods = [];
    const provider = {
      async request(request) {
        methods.push(request.method);
        if (request.method === "wallet_switchEthereumChain") return null;
        if (request.method === "eth_requestAccounts") return ["0x3333333333333333333333333333333333333333"];
        if (request.method === "eth_sendTransaction") return txHash;
        if (request.method === "eth_getTransactionReceipt") return { transactionHash: txHash, status: "0x0" };
        throw new Error(`unexpected provider method ${request.method}`);
      },
    };
    await assert.rejects(
      module.sponsorWithEconomicModule(provider, cfg, repoId, "0.1", "message"),
      /reverted \(receipt status 0x0\)/,
    );
    assert.deepEqual(methods, [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
      "eth_getTransactionReceipt",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM economic split update encodes dynamic arrays, waits for receipt, and never writes V1", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmChainId = 31337;
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  cfg.evmEconomicModule = "0x2222222222222222222222222222222222222222";
  const repoId = `0x${"ab".repeat(32)}`;
  const recipients = [
    "0x4444444444444444444444444444444444444444",
    "inj124242424242424242424242424242424mxdlww",
  ];
  const txHash = `0x${"cd".repeat(32)}`;
  let fetchCalls = 0;
  const requests = [];
  globalThis.fetch = async () => {
    fetchCalls += 1;
    throw new Error("unexpected LCD fallback");
  };
  const provider = {
    async request(request) {
      requests.push(request);
      if (request.method === "wallet_switchEthereumChain") return null;
      if (request.method === "eth_requestAccounts") return ["0x3333333333333333333333333333333333333333"];
      if (request.method === "eth_sendTransaction") return txHash;
      if (request.method === "eth_getTransactionReceipt") return { transactionHash: txHash, status: "0x1" };
      throw new Error(`unexpected provider method ${request.method}`);
    },
  };

  try {
    assert.equal(await module.setRevenueSplitsWithEconomicModule(provider, cfg, repoId, [
      { address: recipients[0], bps: 1250 },
      { address: recipients[1], bps: 750 },
    ]), txHash);
    const send = requests.find((request) => request.method === "eth_sendTransaction");
    assert.equal(send.params[0].to, cfg.evmEconomicModule);
    assert.equal(Object.hasOwn(send.params[0], "value"), false);
    assert.equal(send.params[0].data.slice(0, 10), "0x8c15cea8");
    const args = calldataArgs(send.params[0].data);
    assert.equal(calldataWord(args, 0), BigInt(repoId));
    const recipientsOffset = Number(calldataWord(args, 1));
    const basisPointsOffset = Number(calldataWord(args, 2));
    assert.equal(recipientsOffset, 96);
    assert.equal(basisPointsOffset, 192);
    assert.equal(calldataWord(args, recipientsOffset / 32), 2n);
    assert.equal(calldataWord(args, recipientsOffset / 32 + 1), BigInt(recipients[0]));
    assert.equal(calldataWord(args, recipientsOffset / 32 + 2), BigInt("0x" + "55".repeat(20)));
    assert.equal(calldataWord(args, basisPointsOffset / 32), 2n);
    assert.equal(calldataWord(args, basisPointsOffset / 32 + 1), 1250n);
    assert.equal(calldataWord(args, basisPointsOffset / 32 + 2), 750n);
    assert.deepEqual(requests.map((request) => request.method), [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
      "eth_getTransactionReceipt",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});

test("EVM economic split update fails closed for missing module and reverted receipt", async () => {
  const previousFetch = globalThis.fetch;
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = storageStub();
  const module = await loadChainModule();
  const cfg = module.configForProfile("injective-testnet", "auto");
  cfg.evmContract = "0x1111111111111111111111111111111111111111";
  const repoId = `0x${"ab".repeat(32)}`;
  const splits = [{ address: "0x4444444444444444444444444444444444444444", bps: 1250 }];
  let fetchCalls = 0;
  let providerCalls = 0;
  globalThis.fetch = async () => { fetchCalls += 1; };
  const unusedProvider = { async request() { providerCalls += 1; } };

  try {
    await assert.rejects(
      module.setRevenueSplitsWithEconomicModule(unusedProvider, cfg, repoId, splits),
      /economic module is not configured/,
    );
    assert.equal(providerCalls, 0);
    assert.equal(fetchCalls, 0);

    cfg.evmEconomicModule = "0x2222222222222222222222222222222222222222";
    const txHash = `0x${"cd".repeat(32)}`;
    const methods = [];
    const provider = {
      async request(request) {
        methods.push(request.method);
        if (request.method === "wallet_switchEthereumChain") return null;
        if (request.method === "eth_requestAccounts") return ["0x3333333333333333333333333333333333333333"];
        if (request.method === "eth_sendTransaction") return txHash;
        if (request.method === "eth_getTransactionReceipt") return { transactionHash: txHash, status: "0x0" };
        throw new Error(`unexpected provider method ${request.method}`);
      },
    };
    await assert.rejects(
      module.setRevenueSplitsWithEconomicModule(provider, cfg, repoId, splits),
      /reverted \(receipt status 0x0\)/,
    );
    assert.deepEqual(methods, [
      "wallet_switchEthereumChain",
      "eth_requestAccounts",
      "eth_sendTransaction",
      "eth_getTransactionReceipt",
    ]);
    assert.equal(fetchCalls, 0);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.localStorage = previousStorage;
  }
});
