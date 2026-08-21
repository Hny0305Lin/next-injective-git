import assert from "node:assert/strict";
import test from "node:test";

import {
  COSMWASM_V1_ARCHIVE,
  cosmWasmV1RepoInfo,
  listCosmWasmV1Refs,
  listCosmWasmV1Repos,
  queryCosmWasmV1,
  resetCosmWasmV1Snapshot,
} from "../src/lib/cosmwasm-v1.ts";
import { isCosmWasmV1ArchivePath, repoNameFromBase } from "../src/features/repo/useRepoViews.ts";

const OWNER = "inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5";
const UPDATED_BY = OWNER;

function jsonResponse(payload, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    async json() { return payload; },
  };
}

function parseSmartQuery(url) {
  const encoded = url.split("/smart/")[1];
  return JSON.parse(Buffer.from(decodeURIComponent(encoded), "base64").toString("utf8"));
}

function repo(name) {
  return {
    owner: OWNER,
    name,
    description: `${name} description`,
    default_branch: "main",
    created_at: 1_700_000_000,
    updated_at: 1_700_000_010,
    moderation_status: "active",
    forked_from: null,
  };
}

function ref(name) {
  return {
    ref_name: name,
    commit_sha: "0123456789abcdef0123456789abcdef01234567",
    pack_uris: ["ipfs://bafkreigdummy"],
    updated_at: 1_700_000_010,
    updated_by: UPDATED_BY,
  };
}

test.beforeEach(() => {
  resetCosmWasmV1Snapshot();
});

test("repository breadcrumbs support both EVM and V1 archive route bases", () => {
  assert.equal(repoNameFromBase(`/${OWNER}/demo-showcase`), "demo-showcase");
  assert.equal(
    repoNameFromBase(`/archive/cosmwasm-v1/${OWNER}/demo%20showcase`),
    "demo showcase",
  );
  assert.equal(isCosmWasmV1ArchivePath("/archive/cosmwasm-v1"), true);
  assert.equal(isCosmWasmV1ArchivePath(`/archive/cosmwasm-v1/${OWNER}/demo-showcase`), true);
  assert.equal(isCosmWasmV1ArchivePath("/archive/cosmwasm-v1evil/foo/bar"), false);
});

test("V1 smart queries use a frozen latest height and never a write method", async () => {
  const calls = [];
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url: String(url), init });
    if (String(url).endsWith("/blocks/latest")) {
      return jsonResponse({ block: { header: { height: "12345" } } });
    }
    assert.equal(init.method, "GET");
    assert.equal(init.headers["x-cosmos-block-height"], "12345");
    return jsonResponse({ data: { ok: true } });
  };
  try {
    assert.deepEqual(await queryCosmWasmV1({ config: {} }), { ok: true });
  } finally {
    globalThis.fetch = previousFetch;
  }
  assert.equal(calls.length, 2);
  assert.match(calls[1].url, new RegExp(`/contract/${COSMWASM_V1_ARCHIVE.contract}/smart/`));
  assert.equal(parseSmartQuery(calls[1].url).config !== undefined, true);
});

test("V1 smart queries retry one transient network failure", async () => {
  let queryCalls = 0;
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    if (String(url).endsWith("/blocks/latest")) {
      return jsonResponse({ block: { header: { height: "12349" } } });
    }
    queryCalls += 1;
    if (queryCalls === 1) throw new TypeError("Failed to fetch");
    assert.equal(init.method, "GET");
    return jsonResponse({ data: { retried: true } });
  };
  try {
    assert.deepEqual(await queryCosmWasmV1({ config: {} }), { retried: true });
  } finally {
    globalThis.fetch = previousFetch;
  }
  assert.equal(queryCalls, 2);
});

test("V1 repository and ref queries map protocol fields and paginate", async () => {
  const firstPage = Array.from({ length: 100 }, (_, index) => repo(`repo-${String(index).padStart(3, "0")}`));
  const calls = [];
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url: String(url), init });
    if (String(url).endsWith("/blocks/latest")) {
      return jsonResponse({ block: { header: { height: "12346" } } });
    }
    const query = parseSmartQuery(String(url));
    if (query.list_repos) {
      return query.list_repos.start_after == null
        ? jsonResponse({ data: { repos: firstPage } })
        : jsonResponse({ data: { repos: [repo("repo-100")] } });
    }
    if (query.repo_info) return jsonResponse({ data: repo(query.repo_info.repo) });
    if (query.list_refs) return jsonResponse({ data: { refs: [ref("refs/heads/main")] } });
    throw new Error(`unexpected query ${JSON.stringify(query)}`);
  };
  try {
    const repos = await listCosmWasmV1Repos(OWNER);
    assert.equal(repos.length, 101);
    assert.equal(repos[100].name, "repo-100");
    const info = await cosmWasmV1RepoInfo(OWNER, "demo");
    assert.equal(info.owner, OWNER);
    assert.equal(info.default_branch, "main");
    const refs = await listCosmWasmV1Refs(OWNER, "demo");
    assert.equal(refs[0].ref_name, "refs/heads/main");
    assert.equal(refs[0].updated_by, OWNER);
  } finally {
    globalThis.fetch = previousFetch;
  }
  const smartCalls = calls.filter((call) => call.url.includes("/smart/"));
  assert.equal(smartCalls.every((call) => call.init.method === "GET"), true);
  assert.equal(smartCalls.every((call) => call.init.headers["x-cosmos-block-height"] === "12346"), true);
});

test("V1 pagination fails closed when a full page does not advance", async () => {
  const page = Array.from({ length: 100 }, (_, index) => repo(`repo-${String(index).padStart(3, "0")}`));
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url) => {
    if (String(url).endsWith("/blocks/latest")) return jsonResponse({ block: { header: { height: "12347" } } });
    return jsonResponse({ data: { repos: page } });
  };
  try {
    await assert.rejects(listCosmWasmV1Repos(OWNER), /cursor did not advance/);
  } finally {
    globalThis.fetch = previousFetch;
  }
});

test("V1 archive rejects oversized LCD responses before parsing", async () => {
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url) => {
    if (String(url).endsWith("/blocks/latest")) return jsonResponse({ block: { header: { height: "12348" } } });
    return {
      ok: true,
      status: 200,
      headers: { get: (name) => name === "content-length" ? String(16 * 1024 * 1024 + 1) : null },
      async json() { throw new Error("oversized response must not be parsed"); },
    };
  };
  try {
    await assert.rejects(queryCosmWasmV1({ config: {} }), /response is too large/);
  } finally {
    globalThis.fetch = previousFetch;
  }
});
