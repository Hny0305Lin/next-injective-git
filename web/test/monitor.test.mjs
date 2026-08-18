import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { EVM_ACTIVITY_BLOCK_WINDOW } from "../src/lib/activity.ts";
import { COSMWASM_V1_ARCHIVE } from "../src/lib/cosmwasm-v1.ts";

async function source(path) {
  return readFile(new URL(path, import.meta.url), "utf8");
}

test("public Monitor is routed and suppresses the duplicate global Suite alert", async () => {
  const app = await source("../src/App.tsx");
  assert.match(app, /to:\s*"\/monitor",\s*label:\s*"Monitor"/);
  assert.match(app, /<Route path="\/monitor"/);
  assert.match(app, /const isMonitorRoute = location\.pathname === "\/monitor"/);
  assert.match(app, /!isMonitorRoute && suiteReadiness !== "ready"/);
});

test("Monitor composes shadcn primitives and remains wallet independent", async () => {
  const monitor = await source("../src/pages/Monitor.tsx");
  for (const primitive of ["Card", "Badge", "Tabs", "Alert", "Skeleton", "Table", "Tooltip", "Separator", "Button"]) {
    assert.match(monitor, new RegExp(`<${primitive}(?:[\\s>])`), `${primitive} is not used by Monitor`);
  }
  assert.doesNotMatch(monitor, /useWallet|openWalletModal|connectWallet/);
  assert.match(monitor, /Public monitor/);
  assert.match(monitor, /Read-only/);
});

test("Monitor exposes V1 status without enumerating or ranking V1 repositories", async () => {
  const monitor = await source("../src/pages/Monitor.tsx");
  assert.match(monitor, /prepareCosmWasmV1Snapshot/);
  assert.match(monitor, /COSMWASM_V1_ARCHIVE\.explorer/);
  assert.doesNotMatch(monitor, /listCosmWasmV1Repos|listCosmWasmV1Refs|leaderboard|ranking/i);
  assert.match(monitor, /Migration to EVM V2 is required/);
});

test("Monitor activity retains an explicit bounded observation window", async () => {
  assert.equal(EVM_ACTIVITY_BLOCK_WINDOW, 100_000);
  const monitor = await source("../src/pages/Monitor.tsx");
  const activity = await source("../src/lib/activity.ts");
  assert.match(monitor, /EVM_ACTIVITY_BLOCK_WINDOW\.toLocaleString/);
  assert.match(activity, /BigInt\(EVM_ACTIVITY_BLOCK_WINDOW\)/);
  assert.doesNotMatch(monitor, /Total repositories/i);
});

test("V1 archive source includes the public Injective contract explorer", () => {
  assert.equal(COSMWASM_V1_ARCHIVE.network, "Injective Testnet");
  assert.match(COSMWASM_V1_ARCHIVE.explorer, /^https:\/\/testnet\.explorer\.injective\.network\/contract\/inj1/);
  assert.match(COSMWASM_V1_ARCHIVE.explorer, new RegExp(COSMWASM_V1_ARCHIVE.contract));
});
