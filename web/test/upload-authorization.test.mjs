import assert from "node:assert/strict";
import {
  generateKeyPairSync,
  verify,
} from "node:crypto";
import test from "node:test";

import { issueIdentityToken } from "../api/upload-authorization.mjs";

test("issues a short-lived Ed25519 identity token", () => {
  const { privateKey, publicKey } = generateKeyPairSync("ed25519");
  process.env.IGIT_IDENTITY_ED25519_PRIVATE_KEY = privateKey
    .export({ format: "der", type: "pkcs8" })
    .toString("base64");
  const issued = issueIdentityToken(
    { headers: { "x-forwarded-for": "203.0.113.7" } },
    1_000,
  );
  const [header, payload, signature] = issued.authorization.split(".");
  assert.equal(issued.expires_at, 1_600);
  assert.equal(
    verify(
      null,
      Buffer.from(`${header}.${payload}`),
      publicKey,
      Buffer.from(signature, "base64url"),
    ),
    true,
  );
  const claims = JSON.parse(Buffer.from(payload, "base64url").toString());
  assert.equal(claims.kind, "identity");
  assert.equal(claims.exp, 1_600);
  assert.match(claims.sub, /^ip:[0-9a-f]{32}$/);
});
