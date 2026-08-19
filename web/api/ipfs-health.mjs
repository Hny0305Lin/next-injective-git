/**
 * Server-side read-only IPFS gateway probe.
 *
 * The gateway map is deliberately static.  Do not accept a gateway URL from a
 * request: this endpoint is public and an arbitrary URL parameter would turn
 * it into an SSRF primitive.
 */

export const IPFS_GATEWAYS = Object.freeze({
  "injective-testnet": "https://igit-hk.haohanyh.ovh",
});

export const DEFAULT_PROFILE = "injective-testnet";
export const PROBE_TIMEOUT_MS = 8_000;

const ALLOWED_PROFILE_PATTERN = /^[a-z0-9-]+$/;

function gatewayEndpoint(gateway) {
  // The values in IPFS_GATEWAYS are checked once more at the use site.  Keep
  // this helper separate so path construction cannot accidentally concatenate
  // an untrusted request value.
  const parsed = new URL(gateway);
  if (parsed.protocol !== "https:" || parsed.username || parsed.password || parsed.search || parsed.hash) {
    throw new Error("invalid configured gateway");
  }
  parsed.pathname = `${parsed.pathname.replace(/\/+$/, "")}/ipfs/`;
  return parsed.toString();
}

function queryParameter(query, name) {
  if (!query || typeof query !== "object" || !Object.hasOwn(query, name)) {
    return { present: false, value: null };
  }
  const raw = query[name];
  if (Array.isArray(raw)) {
    return { present: true, value: raw.length === 1 ? String(raw[0]) : null };
  }
  return { present: true, value: raw == null ? "" : String(raw) };
}

function requestQuery(request) {
  const query = request?.query;
  if (query && typeof query === "object") {
    const profile = queryParameter(query, "profile");
    const gateway = queryParameter(query, "gateway");
    if (profile.present || gateway.present) {
      return {
        profile: profile.value,
        profilePresent: profile.present,
        gateway: gateway.value,
        gatewayPresent: gateway.present,
      };
    }
  }

  const rawUrl = typeof request?.url === "string" ? request.url : "";
  if (!rawUrl) return { profile: null, profilePresent: false, gateway: null, gatewayPresent: false };
  try {
    // Vercel supplies an absolute URL in some runtimes and a path in others.
    const base = request?.headers?.host ? `https://${request.headers.host}` : "https://localhost";
    const parsed = new URL(rawUrl, base);
    const profiles = parsed.searchParams.getAll("profile");
    const gateways = parsed.searchParams.getAll("gateway");
    return {
      profile: profiles.length === 1 ? profiles[0] : null,
      profilePresent: profiles.length > 0,
      gateway: gateways.length === 1 ? gateways[0] : null,
      gatewayPresent: gateways.length > 0,
    };
  } catch {
    return { profile: null, profilePresent: false, gateway: null, gatewayPresent: false };
  }
}

/** Resolve a request to one of the statically configured gateways. */
export function resolveProfile(request) {
  const query = requestQuery(request);
  // A gateway query parameter is never accepted, even when it happens to
  // equal the configured value.  This makes the no-SSRF contract explicit.
  if (query.gatewayPresent) {
    return { error: "gateway selection is not supported" };
  }

  if (query.profilePresent && !query.profile) {
    return { error: "invalid profile", profile: null };
  }
  const requested = query.profilePresent ? query.profile : DEFAULT_PROFILE;
  if (!ALLOWED_PROFILE_PATTERN.test(requested) || !Object.hasOwn(IPFS_GATEWAYS, requested)) {
    return { error: "unsupported profile", profile: requested };
  }
  return { profile: requested, gateway: IPFS_GATEWAYS[requested] };
}

function elapsedMs(started, now) {
  return Math.max(0, Math.round(now() - started));
}

function isAbortError(error) {
  return error?.name === "AbortError" || error?.code === "ABORT_ERR";
}

async function cancelBody(response) {
  try {
    await response?.body?.cancel?.();
  } catch {
    // A probe only needs the status line.  Failure to drain/cancel a body must
    // not turn an otherwise valid gateway result into an API error.
  }
}

/**
 * Probe a configured gateway with HEAD and a bounded GET fallback.
 *
 * `options` is intentionally internal/test-friendly; callers should use the
 * static gateway map through the HTTP handler.
 */
export async function probeGateway(
  gateway,
  {
    fetchImpl = globalThis.fetch,
    timeoutMs = PROBE_TIMEOUT_MS,
    now = () => performance.now(),
    wallClock = () => Date.now(),
  } = {},
) {
  if (typeof fetchImpl !== "function") throw new Error("fetch is unavailable");
  const endpoint = gatewayEndpoint(gateway);
  const controller = new AbortController();
  const started = now();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  let method = "HEAD";

  try {
    let response = await fetchImpl(endpoint, {
      method,
      headers: { Accept: "text/plain" },
      signal: controller.signal,
    });
    if (response.status === 405 || response.status === 501) {
      await cancelBody(response);
      method = "GET";
      response = await fetchImpl(endpoint, {
        method,
        headers: { Accept: "text/plain", Range: "bytes=0-0" },
        signal: controller.signal,
      });
    }
    await cancelBody(response);
    const status = Number.isInteger(response.status) && response.status >= 100 && response.status <= 599
      ? response.status
      : null;
    return {
      ok: status !== null && status < 500,
      source: "server",
      gateway,
      status,
      method,
      latencyMs: elapsedMs(started, now),
      checkedAt: wallClock(),
    };
  } catch (error) {
    const timedOut = isAbortError(error);
    return {
      ok: false,
      source: "server",
      gateway,
      status: null,
      method,
      latencyMs: elapsedMs(started, now),
      checkedAt: wallClock(),
      error: timedOut ? "gateway probe timed out" : "gateway probe failed",
      errorCode: timedOut ? "timeout" : "unavailable",
    };
  } finally {
    clearTimeout(timer);
  }
}

function setHeaders(response) {
  response.setHeader("Access-Control-Allow-Origin", "*");
  response.setHeader("Access-Control-Allow-Methods", "GET, OPTIONS");
  response.setHeader("Access-Control-Allow-Headers", "Accept, Content-Type");
  response.setHeader("Cache-Control", "no-store");
  response.setHeader("Content-Type", "application/json; charset=utf-8");
  response.setHeader("Vary", "Origin");
  response.setHeader("X-Content-Type-Options", "nosniff");
}

function sendJson(response, status, payload) {
  response.status(status).json(payload);
}

export default async function handler(request, response) {
  setHeaders(response);
  const method = String(request?.method || "GET").toUpperCase();
  if (method === "OPTIONS") {
    response.status(204).end();
    return;
  }
  if (method !== "GET") {
    sendJson(response, 405, { ok: false, error: "method not allowed" });
    return;
  }

  const selection = resolveProfile(request);
  if (selection.error) {
    sendJson(response, 400, {
      ok: false,
      profile: selection.profile ?? null,
      error: selection.error,
    });
    return;
  }

  try {
    const result = await probeGateway(selection.gateway);
    sendJson(response, 200, { ...result, profile: selection.profile });
  } catch {
    // Keep configuration failures and unexpected runtime errors opaque to a
    // public caller.  The status is represented in JSON for the monitor UI.
    sendJson(response, 500, {
      ok: false,
      profile: selection.profile,
      gateway: selection.gateway,
      source: "server",
      status: null,
      method: null,
      latencyMs: null,
      checkedAt: Date.now(),
      error: "gateway probe is unavailable",
      errorCode: "internal",
    });
  }
}
