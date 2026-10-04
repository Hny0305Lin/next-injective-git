import { test } from "node:test";
import assert from "node:assert/strict";
import { encodeManifest, digest } from "../src/lib/packmanifest.ts";
import {
  fetchVerifiedManifest,
  fetchVerifiedPack,
  ReaderAccessError,
  ReaderIntegrityError,
  ReaderLimitError,
  MAX_WEB_PACK_BYTES,
} from "../src/lib/successorReader.ts";

const base = {
  chainId: "1439",
  suiteDirectory: "0x1111111111111111111111111111111111111111",
  repoId: "0x" + "44".repeat(32),
  refName: "refs/heads/main",
};

async function buildManifest() {
  const manifest = {
    schema: "igit.pack-manifest",
    schemaVersion: 1,
    ...base,
    commit: { algorithm: "sha1", oid: "1d299f0c944e5611d382ce86fced6a7d7516b407" },
    packs: [{
      sequence: 0,
      sha256: "ab".repeat(32),
      size: "32",
      format: "git-pack",
      packVersion: 2,
      thin: false,
      dependsOn: [],
      locations: [{ provider: "cloudflare-r2", url: "https://public.example.com/packs/x.pack", reader: "" }],
    }],
  };
  const bytes = encodeManifest(manifest);
  const sha = await digest(bytes);
  return { manifest, bytes, sha };
}

function stubFetch(handler) {
  const original = globalThis.fetch;
  globalThis.fetch = (input, init) => handler(String(input));
  return () => { globalThis.fetch = original; };
}

test("verified manifest passes the commitment check", async () => {
  const { bytes, sha } = await buildManifest();
  const restore = stubFetch(async () => new Response(bytes));
  try {
    const manifest = await fetchVerifiedManifest(base, {
      sha256: sha, size: String(bytes.length), bootstrapLocator: "https://public.example.com/m.json",
    });
    assert.equal(manifest.commit.oid, "1d299f0c944e5611d382ce86fced6a7d7516b407");
  } finally { restore(); }
});

test("manifest digest mismatch is rejected", async () => {
  const { bytes, sha } = await buildManifest();
  const tampered = new Uint8Array(bytes); tampered[3] ^= 0xff;
  const restore = stubFetch(async () => new Response(tampered));
  try {
    await assert.rejects(
      () => fetchVerifiedManifest(base, { sha256: sha, size: String(bytes.length), bootstrapLocator: "https://public.example.com/m.json" }),
      ReaderIntegrityError,
    );
  } finally { restore(); }
});

test("context substitution across repos is rejected", async () => {
  const { bytes, sha } = await buildManifest();
  const restore = stubFetch(async () => new Response(bytes));
  try {
    const otherRepo = { ...base, repoId: "0x" + "99".repeat(32) };
    await assert.rejects(
      () => fetchVerifiedManifest(otherRepo, { sha256: sha, size: String(bytes.length), bootstrapLocator: "https://public.example.com/m.json" }),
    );
  } finally { restore(); }
});

test("CORS/network failure yields an actionable access hint, not a secret prompt", async () => {
  const { bytes, sha } = await buildManifest();
  const restore = stubFetch(async () => { throw new TypeError("Failed to fetch"); });
  try {
    await assert.rejects(
      () => fetchVerifiedManifest(base, { sha256: sha, size: String(bytes.length), bootstrapLocator: "https://public.example.com/m.json" }),
      (error) => error instanceof ReaderAccessError && error.message.includes("CORS") && error.message.includes("CLI reader"),
    );
  } finally { restore(); }
});

test("oversized manifest is rejected before parsing", async () => {
  const restore = stubFetch(async () => new Response(new Uint8Array(70 * 1024)));
  try {
    await assert.rejects(
      () => fetchVerifiedManifest(base, { sha256: "0".repeat(64), size: String(70 * 1024), bootstrapLocator: "https://public.example.com/m.json" }),
      ReaderLimitError,
    );
  } finally { restore(); }
});

test("pack download verifies digest and size and enforces budgets", async () => {
  const { manifest } = await buildManifest();
  const entry = manifest.packs[0];
  const crypto = await import("node:crypto");
  const packBytes = new TextEncoder().encode("pack-bytes-fixture");
  const packSha = crypto.createHash("sha256").update(packBytes).digest("hex");
  const sized = { ...entry, sha256: packSha, size: String(packBytes.length) };

  const ok = stubFetch(async () => new Response(packBytes));
  try {
    const bytes = await fetchVerifiedPack(sized, { total: 0 });
    assert.equal(bytes.length, packBytes.length);
  } finally { ok(); }

  // wrong bytes fail the digest check
  const bad = stubFetch(async () => new Response(new Uint8Array(packBytes.length).fill(7)));
  try {
    await assert.rejects(() => fetchVerifiedPack(sized, { total: 0 }), ReaderIntegrityError);
  } finally { bad(); }

  // over-budget entries are rejected before any fetch
  await assert.rejects(
    () => fetchVerifiedPack({ ...sized, size: String(MAX_WEB_PACK_BYTES + 1) }, { total: 0 }),
    ReaderLimitError,
  );
  await assert.rejects(
    () => fetchVerifiedPack(sized, { total: 256 * 1024 * 1024 }),
    ReaderLimitError,
  );
});