import assert from "node:assert/strict";
import test from "node:test";

import handler, {
  DEFAULT_PROFILE,
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
  assert.equal(response.payload.gateway, IPFS_GATEWAYS[DEFAULT_PROFILE]);
  assert.equal(response.payload.source, "server");
  assert.equal(response.payload.status, 400);
  assert.equal(response.payload.method, "HEAD");
  assert.equal(Number.isInteger(response.payload.latencyMs), true);
  assert.equal("latency_ms" in response.payload, false);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, `${IPFS_GATEWAYS[DEFAULT_PROFILE]}/ipfs/`);
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
    await handler({ method: "GET", query: { profile: DEFAULT_PROFILE } }, response);
  } finally {
    globalThis.fetch = previousFetch;
  }

  assert.equal(response.statusCode, 200);
  assert.equal(response.payload.ok, true);
  assert.equal(response.payload.status, 206);
  assert.equal(response.payload.method, "GET");
  assert.equal(response.payload.source, "server");
  assert.equal(calls.length, 2);
  assert.equal(calls[1].init.method, "GET");
  assert.equal(calls[1].init.headers.Range, "bytes=0-0");
});

test("profile and gateway inputs cannot select an arbitrary upstream", async () => {
  assert.deepEqual(resolveProfile({ method: "GET", query: { profile: "unknown" } }), {
    error: "unsupported profile",
    profile: "unknown",
  });
  assert.deepEqual(resolveProfile({
    method: "GET",
    query: { gateway: "https://127.0.0.1:8080" },
  }), { error: "gateway selection is not supported" });

  const previousFetch = globalThis.fetch;
  let called = false;
  globalThis.fetch = async () => {
    called = true;
    throw new Error("an arbitrary gateway must never be contacted");
  };
  const response = responseStub();
  try {
    await handler({ method: "GET", url: "/api/ipfs-health?profile=unknown" }, response);
  } finally {
    globalThis.fetch = previousFetch;
  }
  assert.equal(response.statusCode, 400);
  assert.equal(response.payload.ok, false);
  assert.equal(called, false);
});

test("probe aborts and reports a timeout without leaking upstream details", async () => {
  const result = await probeGateway(IPFS_GATEWAYS[DEFAULT_PROFILE], {
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
