import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { ownerRepoSample } from "../src/lib/storage-stats.ts";

async function source(path) {
  return readFile(new URL(path, import.meta.url), "utf8");
}

// Two wallets observed by the same board walk. Wallet A stored repos on the
// HK IPFS gateway and Filebase; wallet B has one IPFS repo; wallet C none.
function statsWithTwoOwners() {
  const ownerA = {
    reposByProvider: { ipfs: 2 },
    reposByHost: { "s3.filebase.com": 1 },
    repoNamesByProvider: { ipfs: ["repo-a-1", "repo-a-2"] },
    repoNamesByHost: { "s3.filebase.com": ["repo-a-1"] },
  };
  const ownerB = {
    reposByProvider: { ipfs: 1 },
    reposByHost: {},
    repoNamesByProvider: { ipfs: ["repo-b-1"] },
    repoNamesByHost: {},
  };
  return {
    observedOwners: 2,
    repos: 3,
    reposDiscovered: 3,
    totalPacks: 6,
    packsByProvider: { ipfs: 6, "aws-s3": 0, "cloudflare-r2": 0 },
    reposByProvider: { ipfs: 3, "aws-s3": 0, "cloudflare-r2": 0 },
    packsByHost: { "s3.filebase.com": 2 },
    reposByHost: { "s3.filebase.com": 1 },
    ownerSamples: { "inj1walleta": ownerA, "inj1walletb": ownerB },
    v1: { owners: 0, repos: 0, packs: 0 },
    sampledAt: Date.now(),
    truncated: false,
  };
}

test("ownerRepoSample returns only the requested wallet's repository names", () => {
  const stats = statsWithTwoOwners();
  const sample = ownerRepoSample(stats, "inj1walletb", { provider: "ipfs" });
  assert.deepEqual(sample.names, ["repo-b-1"]);
  assert.equal(sample.total, 1);
  assert.ok(!sample.names.includes("repo-a-1"));
  assert.ok(!sample.names.includes("repo-a-2"));
});

test("ownerRepoSample matches owner keys case-insensitively (bech32 lookups)", () => {
  const stats = statsWithTwoOwners();
  const sample = ownerRepoSample(stats, "INJ1WALLETB", { provider: "ipfs" });
  assert.deepEqual(sample.names, ["repo-b-1"]);
});

test("ownerRepoSample never falls back to another wallet's names", () => {
  const stats = statsWithTwoOwners();
  // A wallet with no repositories sees an empty sample, never A's or B's names.
  const stranger = ownerRepoSample(stats, "inj1walletc", { provider: "ipfs" });
  assert.deepEqual(stranger.names, []);
  // No owner bucket -> total is null; the popup folds null into 0.
  assert.ok(stranger.total == null);
  // Disconnected session / missing stats: no names at all.
  assert.deepEqual(ownerRepoSample(stats, null, { provider: "ipfs" }).names, []);
  assert.deepEqual(ownerRepoSample(null, "inj1walleta", { provider: "ipfs" }).names, []);
  // Unknown keying returns nothing either.
  assert.deepEqual(ownerRepoSample(stats, "inj1walleta", {}).names, []);
});

test("ownerRepoSample scopes host-keyed samples to the owner too", () => {
  const stats = statsWithTwoOwners();
  const own = ownerRepoSample(stats, "inj1walleta", { host: "s3.filebase.com" });
  assert.deepEqual(own.names, ["repo-a-1"]);
  assert.equal(own.total, 1);
  const other = ownerRepoSample(stats, "inj1walletb", { host: "s3.filebase.com" });
  assert.deepEqual(other.names, []);
  assert.equal(other.total, 0);
});

test("storage stats record repo names per owner, never merged across wallets", async () => {
  const lib = await source("../src/lib/storage-stats.ts");
  assert.match(lib, /ownerSamples: Record<string, OwnerRepoSamples>/);
  assert.match(lib, /function ownerBucket\(stats: StorageStats, owner: string\)/);
  // No code path may read merged cross-owner name samples anymore.
  assert.doesNotMatch(lib, /stats\.repoNamesByProvider|stats\.repoNamesByHost/);
  // Cache is versioned and legacy unscoped caches are purged (never read).
  assert.match(lib, /igit-mapmonitor-stats:v2:/);
  assert.match(lib, /localStorage\.removeItem\(STATS_CACHE_LEGACY_PREFIX/);
  assert.match(lib, /parsed\.stats\?\.ownerSamples == null/);
});

test("MapMonitor popup renders only the connected wallet's repositories", async () => {
  const page = await source("../src/pages/MapMonitor.tsx");
  // Popup identity comes from the CURRENT session's wallet address.
  assert.match(page, /const walletAddress = wallet\.connected\?\.address \?\? null/);
  assert.match(page, /livePointRepos\(point\.id, boardData, walletAddress\)/);
  assert.match(page, /ownerRepoSample\(data, owner, \{ provider: "ipfs" \}\)/);
  // Gate requires BOTH a connection and a resolved address.
  assert.match(page, /if \(!walletConnected \|\| walletAddress == null\) return base;/);
  // Links target the connected wallet's Owner page, not a hardcoded owner.
  assert.doesNotMatch(page, /OBSERVED_OWNER_PATH/);
  assert.match(page, /const ownerPath = `\/\$\{encodeURIComponent\(walletAddress\)\}`/);
  assert.doesNotMatch(page, /data\.repoNamesByProvider|data\.repoNamesByHost/);
});
