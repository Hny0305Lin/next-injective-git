import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { filterDiscoverRepositories, pageDiscoverRepositories } from "../src/lib/discover.ts";

const ownerA = "inj1alpha";
const ownerB = "inj1beta";

function repo(name, overrides = {}) {
  return {
    owner: ownerA,
    name,
    description: `${name} description`,
    default_branch: "main",
    created_at: 1_700_000_000,
    updated_at: 1_700_000_000,
    moderation_status: "active",
    forked_from: null,
    suite_version: 4n,
    source: "evm-suite-events",
    repo_id: `0x${name.padEnd(64, "0").slice(0, 64)}`,
    suite_directory: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    ...overrides,
  };
}

test("discover filtering searches metadata, owner, status, and suite version", () => {
  const rows = [
    repo("r2", { owner: ownerB, updated_at: 30, suite_version: 3n, moderation_status: "frozen" }),
    repo("r1", { updated_at: 20 }),
    repo("delisted", { updated_at: 40, moderation_status: "delisted" }),
  ];
  assert.deepEqual(filterDiscoverRepositories(rows, { query: "description", owner: ownerB, status: "frozen", suiteVersion: 3 }).map((row) => row.name), ["r2"]);
  assert.deepEqual(filterDiscoverRepositories(rows).map((row) => row.name), ["delisted", "r2", "r1"]);
});

test("discover orders by updated time and paginates deterministically", () => {
  const rows = [1, 2, 3, 4, 5].map((number) => repo(`repo-${number}`, { updated_at: number }));
  const ordered = filterDiscoverRepositories(rows);
  assert.deepEqual(ordered.map((row) => row.name), ["repo-5", "repo-4", "repo-3", "repo-2", "repo-1"]);
  assert.deepEqual(pageDiscoverRepositories(ordered, 2, 2).map((row) => row.name), ["repo-3", "repo-2"]);
  assert.deepEqual(pageDiscoverRepositories(ordered, 99, 2), []);
});

test("discover excludes non EVM sources such as the isolated V1 Archive", () => {
  const archiveRow = repo("legacy", { source: "cosmwasm-v1-archive" });
  assert.deepEqual(filterDiscoverRepositories([archiveRow]), []);
});

test("discover is a public route and does not require wallet context", async () => {
  const [page, app] = await Promise.all([
    readFile(new URL("../src/pages/Discover.tsx", import.meta.url), "utf8"),
    readFile(new URL("../src/App.tsx", import.meta.url), "utf8"),
  ]);
  assert.match(app, /Route path=\"\/discover\"/);
  assert.doesNotMatch(page, /WalletContext|useWallet/);
});
