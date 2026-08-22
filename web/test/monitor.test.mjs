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
  const monitor = await source("../src/features/monitor/Monitor.tsx");
  for (const primitive of ["Card", "Badge", "Tabs", "Alert", "Skeleton", "Table", "Tooltip", "Separator", "Button"]) {
    assert.match(monitor, new RegExp(`<${primitive}(?:[\\s>])`), `${primitive} is not used by Monitor`);
  }
  assert.doesNotMatch(monitor, /useWallet|openWalletModal|connectWallet/);
  assert.match(monitor, /Public monitor/);
  assert.match(monitor, /Read-only/);
});

test("Monitor exposes V1 status without enumerating or ranking V1 repositories", async () => {
  const monitor = await source("../src/features/monitor/Monitor.tsx");
  assert.match(monitor, /prepareCosmWasmV1Snapshot/);
  assert.match(monitor, /COSMWASM_V1_ARCHIVE\.explorer/);
  assert.doesNotMatch(monitor, /listCosmWasmV1Repos|listCosmWasmV1Refs|leaderboard|ranking/i);
  assert.match(monitor, /Migration to EVM V2 is required/);
});

test("Monitor activity retains an explicit bounded observation window", async () => {
  assert.equal(EVM_ACTIVITY_BLOCK_WINDOW, 100_000);
  const monitor = await source("../src/features/monitor/Monitor.tsx");
  const activity = await source("../src/lib/activity.ts");
  assert.match(monitor, /EVM_ACTIVITY_BLOCK_WINDOW\.toLocaleString/);
  assert.match(activity, /BigInt\(EVM_ACTIVITY_BLOCK_WINDOW\)/);
  assert.doesNotMatch(monitor, /Total repositories/i);
});

test("Monitor keeps HK and US in one IPFS gateway surface", async () => {
  const monitor = await source("../src/features/monitor/Monitor.tsx");
  assert.match(monitor, /IPFS gateways/);
  assert.match(monitor, /Hong Kong gateway/);
  assert.match(monitor, /US gateway/);
  assert.match(monitor, /United States/);
  assert.match(monitor, /https:\/\/igit-us\.haohanyh\.ovh/);
  assert.match(monitor, /https:\/\/igit-hk\.haohanyh\.ovh/);
  assert.match(monitor, /PUBLIC_IPFS_GATEWAYS\.map/);
  assert.doesNotMatch(monitor, /healthz/);
});

test("Monitor keeps Filebase and Fil.one as provider metadata", async () => {
  const monitor = await source("../src/features/monitor/Monitor.tsx");
  assert.match(monitor, /External archive providers/);
  assert.match(monitor, /Filebase/);
  assert.match(monitor, /https:\/\/s3\.filebase\.com/);
  assert.match(monitor, /Fil\.one/);
  assert.match(monitor, /https:\/\/us-east-1\.s3\.fil\.one/);
  assert.match(monitor, /Provider badges describe the configured role/);
  assert.doesNotMatch(monitor, /us101010|a10101|162\.35\.187\.224|12D3KooW/);
});

test("Monitor measures each gateway directly from the browser using three-sample medians", async () => {
  const monitor = await source("../src/features/monitor/Monitor.tsx");
  const probe = await source("../src/lib/ipfs-probe.ts");
  assert.match(probe, /BROWSER_GATEWAY_PROBE_SAMPLE_COUNT = 3/);
  assert.match(probe, /samples\.push\(await probeGatewayOnce/);
  assert.match(probe, /medianLatency\(responses\.map/);
  assert.match(monitor, /probeGatewayFromBrowser\(gateway\.endpoint\)/);
  assert.match(monitor, /Independent status, latency, and freshness/);
  assert.match(monitor, /Median latency/);
  assert.match(monitor, /This browser/);
  assert.doesNotMatch(monitor, /\/api\/ipfs-health|Website server|Browser fallback/);
});

test("Monitor automatically refreshes all probes every 90 seconds", async () => {
  const monitor = await source("../src/features/monitor/Monitor.tsx");
  assert.match(monitor, /const REFRESH_INTERVAL_MS = 90_000;/);
  assert.match(monitor, /window\.setInterval\(\(\) => void refresh\(\), REFRESH_INTERVAL_MS\)/);
  assert.match(monitor, /detail=\{`auto refresh · \$\{REFRESH_INTERVAL_MS \/ 1000\}s`\}/);
});

test("V1 archive source includes the public Injective contract explorer", () => {
  assert.equal(COSMWASM_V1_ARCHIVE.network, "Injective Testnet");
  assert.match(COSMWASM_V1_ARCHIVE.explorer, /^https:\/\/testnet\.explorer\.injective\.network\/contract\/inj1/);
  assert.match(COSMWASM_V1_ARCHIVE.explorer, new RegExp(COSMWASM_V1_ARCHIVE.contract));
});
