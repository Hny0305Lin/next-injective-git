import assert from "node:assert/strict";
import test from "node:test";

import {
  BROWSER_GATEWAY_PROBE_SAMPLE_COUNT,
  medianLatency,
  probeGatewayFromBrowser,
} from "../src/lib/ipfs-probe.ts";

test("browser gateway probe uses the median of three sequential responses", async () => {
  const calls = [];
  const clock = [0, 90, 100, 120, 130, 180];
  let active = 0;
  let maxActive = 0;
  const result = await probeGatewayFromBrowser("https://gateway.example/", {
    now: () => clock.shift(),
    fetchImpl: async (url, init) => {
      active += 1;
      maxActive = Math.max(maxActive, active);
      await Promise.resolve();
      calls.push({ url: String(url), init });
      active -= 1;
      return { status: 400, body: { cancel: async () => {} } };
    },
  });

  assert.equal(BROWSER_GATEWAY_PROBE_SAMPLE_COUNT, 3);
  assert.equal(calls.length, 3);
  assert.equal(maxActive, 1, "samples for one gateway must not overlap");
  assert.equal(result.ok, true);
  assert.equal(result.status, 400);
  assert.equal(result.latencyMs, 50);
  assert.equal(result.sampleCount, 3);
  assert.equal(result.responseSamples, 3);
  assert.equal(result.reachableSamples, 3);
  assert.equal(result.source, "browser");
  assert.equal(result.error, undefined);
  assert.deepEqual(calls.map((call) => call.url), Array(3).fill("https://gateway.example/ipfs/"));
  assert.equal(calls.every((call) => call.init.method === "HEAD" && call.init.cache === "no-store"), true);
});

test("browser gateway probe degrades partial results without hiding the median", async () => {
  const clock = [0, 10, 20, 50, 60, 100];
  const outcomes = [400, 502, new Error("offline")];
  const result = await probeGatewayFromBrowser("https://gateway.example", {
    now: () => clock.shift(),
    fetchImpl: async () => {
      const outcome = outcomes.shift();
      if (outcome instanceof Error) throw outcome;
      return { status: outcome, body: { cancel: async () => {} } };
    },
  });

  assert.equal(result.ok, false);
  assert.equal(result.status, 400);
  assert.equal(result.latencyMs, 20);
  assert.equal(result.responseSamples, 2);
  assert.equal(result.reachableSamples, 1);
  assert.match(result.error, /1\/3 browser probes reached/);
});

test("browser gateway probe does not invent latency when every request fails", async () => {
  const clock = [0, 10, 20, 30, 40, 50];
  const result = await probeGatewayFromBrowser("https://gateway.example", {
    now: () => clock.shift(),
    fetchImpl: async () => {
      throw new Error("offline");
    },
  });

  assert.equal(result.ok, false);
  assert.equal(result.status, null);
  assert.equal(result.latencyMs, null);
  assert.equal(result.responseSamples, 0);
  assert.equal(result.reachableSamples, 0);
  assert.equal(result.error, "gateway probes failed");
});

test("median latency handles odd, even, and empty samples", () => {
  assert.equal(medianLatency([90, 20, 50]), 50);
  assert.equal(medianLatency([10, 30]), 20);
  assert.equal(medianLatency([]), null);
});
