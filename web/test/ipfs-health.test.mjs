import assert from "node:assert/strict";
import test from "node:test";

import handler, {
  DEFAULT_PROFILE,
  DEFAULT_TARGET,
  IPFS_GATEWAYS,
  probeGateway,
  resolveProfile,
} from "../api/ipfs-health.mjs";

function responseStub() {
  const headers = new Map();
  return {
    headers,
    statusCode: null,
    payload: null,
    ended: false,
    setHeader(name, value) {
      headers.set(name.toLowerCase(), value);
    },
    status(value) {
      this.statusCode = value;
      return this;
    },
    json(value) {
      this.payload = value;
      return this;
    },
    end() {
      this.ended = true;
    },
  };
}

test("handler probes the default fixed gateway and returns JSON status", async () => {
  const calls = [];
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init });
    return { status: 400, body: { cancel: async () => {} } };
  };
  const response = responseStub();
  try {
    await handler({ method: "GET", url: "/api/ipfs-health" }, response);
  } finally {
    globalThis.fetch = previousFetch;
  }

  assert.equal(response.statusCode, 200);
  assert.equal(response.payload.ok, true, "HTTP 400 still proves the gateway answered");
  assert.equal(response.payload.profile, DEFAULT_PROFILE);
  assert.equal(response.payload.target, DEFAULT_TARGET);
  assert.equal(response.payload.gateway, IPFS_GATEWAYS[DEFAULT_PROFILE][DEFAULT_TARGET]);
  assert.equal(response.payload.source, "server");
  assert.equal(response.payload.status, 400);
  assert.equal(response.payload.method, "HEAD");
  assert.equal(Number.isInteger(response.payload.latencyMs), true);
  assert.equal("latency_ms" in response.payload, false);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, `${IPFS_GATEWAYS[DEFAULT_PROFILE][DEFAULT_TARGET]}/ipfs/`);
  assert.equal(calls[0].init.method, "HEAD");
  assert.equal(calls[0].init.headers.Accept, "text/plain");
  assert.equal(response.headers.get("access-control-allow-origin"), "*");
  assert.equal(response.headers.get("cache-control"), "no-store");
});

test("handler falls back from HEAD 405 to a bounded GET", async () => {
  const calls = [];
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init });
    return calls.length === 1
      ? { status: 405, body: { cancel: async () => {} } }
      : { status: 206, body: { cancel: async () => {} } };
  };
  const response = responseStub();
  try {
    await handler({ method: "GET", query: { profile: DEFAULT_PROFILE, target: "us" } }, response);
  } finally {
    globalThis.fetch = previousFetch;
  }

  assert.equal(response.statusCode, 200);
  assert.equal(response.payload.ok, true);
  assert.equal(response.payload.status, 206);
  assert.equal(response.payload.method, "GET");
  assert.equal(response.payload.source, "server");
  assert.equal(response.payload.target, "us");
  assert.equal(response.payload.gateway, IPFS_GATEWAYS[DEFAULT_PROFILE].us);
  assert.equal(calls.length, 2);
  assert.equal(calls[1].init.method, "GET");
  assert.equal(calls[1].init.headers.Range, "bytes=0-0");
});

test("profile and target inputs cannot select an arbitrary upstream", async () => {
  assert.deepEqual(resolveProfile({ method: "GET", query: { profile: "unknown" } }), {
    error: "unsupported profile",
    profile: "unknown",
  });
  assert.deepEqual(resolveProfile({
    method: "GET",
    query: { gateway: "https://127.0.0.1:8080" },
  }), { error: "gateway selection is not supported" });
  assert.deepEqual(resolveProfile({
    method: "GET",
    query: { gateway: "unknown" },
  }), { error: "gateway selection is not supported" });
  assert.deepEqual(resolveProfile({
    method: "GET",
    query: { target: "https://127.0.0.1:8080" },
  }), { error: "unsupported target", profile: DEFAULT_PROFILE, target: "https://127.0.0.1:8080" });
  assert.deepEqual(resolveProfile({
    method: "GET",
    query: { target: "unknown" },
  }), { error: "unsupported target", profile: DEFAULT_PROFILE, target: "unknown" });
  assert.deepEqual(resolveProfile({
    method: "GET",
    query: { target: ["hk", "us"] },
  }), { error: "invalid target", profile: DEFAULT_PROFILE, target: null });

  const previousFetch = globalThis.fetch;
  let called = false;
  globalThis.fetch = async () => {
    called = true;
    throw new Error("an arbitrary gateway must never be contacted");
  };
  try {
    for (const url of [
      "/api/ipfs-health?profile=unknown",
      "/api/ipfs-health?target=unknown",
      "/api/ipfs-health?target=hk&target=us",
      "/api/ipfs-health?gateway=us",
      "/api/ipfs-health?gateway=https%3A%2F%2F127.0.0.1%3A8080",
    ]) {
      const response = responseStub();
      await handler({ method: "GET", url }, response);
      assert.equal(response.statusCode, 400, url);
      assert.equal(response.payload.ok, false, url);
    }
  } finally {
    globalThis.fetch = previousFetch;
  }
  assert.equal(called, false);
});

test("the fixed US gateway is selected without accepting a URL", () => {
  const selection = resolveProfile({ method: "GET", query: { target: "us" } });
  assert.deepEqual(selection, {
    profile: DEFAULT_PROFILE,
    target: "us",
    gateway: IPFS_GATEWAYS[DEFAULT_PROFILE].us,
  });
  assert.deepEqual(resolveProfile({
    method: "GET",
    url: `/api/ipfs-health?profile=${DEFAULT_PROFILE}&target=us`,
  }), selection);
});

test("probe aborts and reports a timeout without leaking upstream details", async () => {
  const result = await probeGateway(IPFS_GATEWAYS[DEFAULT_PROFILE][DEFAULT_TARGET], {
    timeoutMs: 5,
    fetchImpl: async (_url, init) => await new Promise((_resolve, reject) => {
      init.signal.addEventListener("abort", () => {
        const error = new Error("aborted");
        error.name = "AbortError";
        reject(error);
      }, { once: true });
    }),
  });
  assert.equal(result.ok, false);
  assert.equal(result.status, null);
  assert.equal(result.source, "server");
  assert.equal(Number.isInteger(result.latencyMs), true);
  assert.equal(result.errorCode, "timeout");
  assert.equal(result.error, "gateway probe timed out");
});

test("handler answers CORS preflight and rejects writes", async () => {
  const optionsResponse = responseStub();
  await handler({ method: "OPTIONS" }, optionsResponse);
  assert.equal(optionsResponse.statusCode, 204);
  assert.equal(optionsResponse.ended, true);
  assert.match(optionsResponse.headers.get("access-control-allow-methods"), /GET/);

  const postResponse = responseStub();
  await handler({ method: "POST" }, postResponse);
  assert.equal(postResponse.statusCode, 405);
  assert.equal(postResponse.payload.ok, false);
});
