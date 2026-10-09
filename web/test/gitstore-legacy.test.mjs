// Legacy (v3 ipfs-pack-uris) pack fetch hardening: strict CID validation,
// transport discipline (redirect/credentials), byte budgets, raw-CID content
// verification and the packfile magic check. Covers the unverified-loadRef
// finding: on-chain pack_uris must never steer the request path or feed
// unverified bytes into the git object store.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  PackUriError,
  parsePackCid,
  fetchLegacyPack,
  inspectCid,
  verifyLegacyCidContent,
} from "../src/lib/gitstore.ts";

const cfg = { ipfsGateway: "https://gw.example.com" };

function packBytes(fill = 0x61, length = 64) {
  const bytes = new Uint8Array(length);
  bytes.set([0x50, 0x41, 0x43, 0x4b, 0, 0, 0, 2, 0, 0, 0, 1]); // "PACK" v2, 1 object
  for (let i = 12; i < length; i++) bytes[i] = fill;
  return bytes;
}

function sha256Hex(bytes) {
  return crypto.subtle.digest("SHA-256", bytes).then((h) =>
    [...new Uint8Array(h)].map((b) => b.toString(16).padStart(2, "0")).join(""));
}

const BASE32_LOWER = "abcdefghijklmnopqrstuvwxyz234567";
function encodeBase32Lower(bytes) {
  let bits = 0, value = 0, out = "";
  for (const b of bytes) {
    value = (value << 8) | b;
    bits += 8;
    while (bits >= 5) {
      out += BASE32_LOWER[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += BASE32_LOWER[(value << (5 - bits)) & 31];
  return out;
}

/** Build a CIDv1-raw(b58 not needed) string committing to the given bytes. */
async function rawCidFor(bytes) {
  const hashHex = await sha256Hex(bytes);
  const hash = Uint8Array.from(hashHex.match(/../g).map((h) => parseInt(h, 16)));
  const encoded = new Uint8Array(2 + 2 + 32);
  encoded[0] = 0x01; // cid version 1
  encoded[1] = 0x55; // raw codec
  encoded[2] = 0x12; // sha2-256
  encoded[3] = 0x20; // 32 bytes
  encoded.set(hash, 4);
  return "b" + encodeBase32Lower(encoded);
}

// A syntactically valid CIDv1 base32 string whose payload is NOT a valid
// CID (version varint != 1) — used to probe decoder robustness.
const malformedCid = "b" + "a".repeat(24);

function stubFetch(handler) {
  const original = globalThis.fetch;
  globalThis.fetch = (input, init) => handler(String(input), init);
  return () => { globalThis.fetch = original; };
}

// ---- parsePackCid: only exact CID forms pass ----

test("parsePackCid accepts ipfs:// and bare CIDv1/CIDv0 forms", () => {
  const cidv1 = "bafkreig7fdeswhfb2jg7tyuz4tb3bxsrldgsl4v5i3nyd7d7j5hgr4g4qu";
  const cidv0 = "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG";
  assert.equal(parsePackCid(`ipfs://${cidv1}`), cidv1);
  assert.equal(parsePackCid(cidv1), cidv1);
  assert.equal(parsePackCid(`ipfs://${cidv0}`), cidv0);
  assert.equal(parsePackCid(cidv0), cidv0);
});

test("parsePackCid rejects traversal, query, fragment, other schemes and junk", () => {
  const bad = [
    "../../../etc/passwd",
    "/ipfs/../../etc/passwd",
    "bafy/../..",
    "bafy?query=1",
    "bafy#fragment",
    "..\\..\\windows",
    "http://evil.example/pack",
    "https://evil.example/pack",
    "ipfs://../../../etc/passwd",
    "ipfs://bafy?x=1",
    "ipfs://",
    "",
    " ",
    "bafy with spaces",
    "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbd",   // 43 chars after Qm
    "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdGx",  // 45 chars after Qm
    "0",
    "bafy\0x",
  ];
  for (const uri of bad) {
    assert.throws(() => parsePackCid(uri), PackUriError, `expected rejection for ${JSON.stringify(uri)}`);
  }
});

// ---- verifyLegacyCidContent: raw CIDs are content-bound ----

test("raw CIDv1 content verification accepts matching bytes and rejects substituted bytes", async () => {
  const good = packBytes(0x61);
  const cid = await rawCidFor(good);
  assert.equal(await verifyLegacyCidContent(cid, good), true);
  assert.equal(await verifyLegacyCidContent(cid, packBytes(0x62)), false);
});

test("dag-pb CIDs (CIDv0) skip byte-level verification instead of failing", async () => {
  const cidv0 = "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG";
  assert.equal(await verifyLegacyCidContent(cidv0, packBytes(0x63)), true);
});

test("malformed CIDv1 payload fails closed", async () => {
  assert.equal(await verifyLegacyCidContent(malformedCid, packBytes()), false);
});

// ---- fetchLegacyPack: transport discipline and integrity gates ----

test("fetchLegacyPack fetches the encoded CID with redirect error and no credentials", async () => {
  const bytes = packBytes();
  const cid = await rawCidFor(bytes);
  let seenUrl = "";
  let seenInit = null;
  const restore = stubFetch(async (url, init) => {
    seenUrl = url;
    seenInit = init;
    return new Response(bytes, { headers: { "content-length": String(bytes.length) } });
  });
  try {
    const out = await fetchLegacyPack(cfg, `ipfs://${cid}`, { total: 0 });
    assert.equal(seenUrl, `https://gw.example.com/ipfs/${cid}`);
    assert.deepEqual(seenInit, { credentials: "omit", redirect: "error" });
    assert.equal(out.length, bytes.length);
  } finally { restore(); }
});

test("fetchLegacyPack rejects a declared length over the per-pack budget", async () => {
  const bytes = packBytes();
  const cid = await rawCidFor(bytes);
  const restore = stubFetch(async () =>
    new Response(bytes, { headers: { "content-length": String(64 * 1024 * 1024) } }));
  try {
    await assert.rejects(
      () => fetchLegacyPack(cfg, cid, { total: 0 }),
      /web budget/,
    );
  } finally { restore(); }
});

test("fetchLegacyPack enforces the cumulative total budget", async () => {
  const bytes = packBytes();
  const cid = await rawCidFor(bytes);
  const restore = stubFetch(async () => new Response(bytes));
  try {
    await assert.rejects(
      () => fetchLegacyPack(cfg, cid, { total: 256 * 1024 * 1024 }),
      /total web download budget/,
    );
  } finally { restore(); }
});

test("fetchLegacyPack rejects substituted bytes for a raw CID", async () => {
  const cid = await rawCidFor(packBytes(0x61));
  const restore = stubFetch(async () => new Response(packBytes(0x62)));
  try {
    await assert.rejects(
      () => fetchLegacyPack(cfg, cid, { total: 0 }),
      /do not match the requested CID/,
    );
  } finally { restore(); }
});

test("fetchLegacyPack rejects non-packfile content even for unverifiable CIDs", async () => {
  const cidv0 = "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG";
  const junk = new TextEncoder().encode("<html>not a pack</html>");
  const restore = stubFetch(async () => new Response(junk));
  try {
    await assert.rejects(
      () => fetchLegacyPack(cfg, cidv0, { total: 0 }),
      /not a git packfile/,
    );
  } finally { restore(); }
});

test("fetchLegacyPack never issues a request for a hostile pack uri", async () => {
  let called = false;
  const restore = stubFetch(async () => { called = true; return new Response(); });
  try {
    await assert.rejects(
      () => fetchLegacyPack(cfg, "../../../etc/passwd", { total: 0 }),
      PackUriError,
    );
    await assert.rejects(
      () => fetchLegacyPack(cfg, "http://evil.example/x", { total: 0 }),
      PackUriError,
    );
    assert.equal(called, false, "no network request may leave for an invalid uri");
  } finally { restore(); }
});

// ---- inspectCid: same validation on the diagnostic path ----

test("inspectCid reports invalid uris without hitting the gateway", async () => {
  let called = false;
  const restore = stubFetch(async () => { called = true; return new Response(); });
  try {
    const info = await inspectCid(cfg, "../../secret");
    assert.equal(info.ok, false);
    assert.match(info.error, /unsupported pack uri/);
    assert.equal(called, false);
  } finally { restore(); }
});

test("inspectCid still parses valid packfiles", async () => {
  const bytes = packBytes();
  const cid = await rawCidFor(bytes);
  const restore = stubFetch(async () => new Response(bytes));
  try {
    const info = await inspectCid(cfg, `ipfs://${cid}`);
    assert.equal(info.ok, true);
    assert.equal(info.cid, cid);
    assert.equal(info.isPack, true);
    assert.equal(info.version, 2);
    assert.equal(info.objectCount, 1);
  } finally { restore(); }
});
