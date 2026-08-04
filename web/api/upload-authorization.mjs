import {
  createHash,
  createPrivateKey,
  randomUUID,
  sign,
} from "node:crypto";

const TOKEN_TTL_SECONDS = 10 * 60;

function base64url(value) {
  const bytes = Buffer.isBuffer(value) ? value : Buffer.from(value);
  return bytes.toString("base64url");
}

function clientSubject(request) {
  const forwarded = request.headers?.["x-forwarded-for"];
  const raw = Array.isArray(forwarded)
    ? forwarded[0]
    : forwarded || request.socket?.remoteAddress || "unknown";
  const ip = String(raw).split(",")[0].trim();
  const digest = createHash("sha256").update(ip).digest("hex").slice(0, 32);
  return `ip:${digest}`;
}

export function issueIdentityToken(request, now = Math.floor(Date.now() / 1000)) {
  const encodedKey = process.env.IGIT_IDENTITY_ED25519_PRIVATE_KEY;
  if (!encodedKey) {
    throw new Error("identity issuer is not configured");
  }
  const privateKey = createPrivateKey({
    key: Buffer.from(encodedKey, "base64"),
    format: "der",
    type: "pkcs8",
  });
  const header = base64url(JSON.stringify({ alg: "EdDSA", typ: "JWT" }));
  const expiresAt = now + TOKEN_TTL_SECONDS;
  const payload = base64url(
    JSON.stringify({
      kind: "identity",
      sub: clientSubject(request),
      iat: now,
      exp: expiresAt,
      jti: randomUUID(),
    }),
  );
  const message = `${header}.${payload}`;
  const signature = sign(null, Buffer.from(message), privateKey);
  return {
    authorization: `${message}.${base64url(signature)}`,
    expires_at: expiresAt,
  };
}

export default function handler(request, response) {
  response.setHeader("Access-Control-Allow-Origin", "*");
  response.setHeader("Access-Control-Allow-Methods", "POST, OPTIONS");
  response.setHeader("Cache-Control", "no-store");
  if (request.method === "OPTIONS") {
    response.status(204).end();
    return;
  }
  if (request.method !== "POST") {
    response.status(405).json({ error: "method not allowed" });
    return;
  }
  try {
    response.status(200).json(issueIdentityToken(request));
  } catch {
    response.status(503).json({ error: "upload authorization is unavailable" });
  }
}
